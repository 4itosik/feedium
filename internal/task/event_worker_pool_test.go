package task_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/task"
)

func TestIntegration_EventWorkerPool_ProcessesPostEvent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "Some interesting content that should be summarized.")

	outboxRepo := data.NewSummaryOutboxRepo(d)
	summaryRepo := data.NewSummaryRepo(d)

	postIDRef := post.ID
	_, err := outboxRepo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, sourceID, &postIDRef))
	require.NoError(t, err)

	llm := &fakeLLM{reply: "hello world summary"}
	cfg := testSummaryCfg()

	pool := task.NewEventWorkerPool(outboxRepo, newTestUsecase(t, d, cfg, llm), cfg, testLogger())
	require.NoError(t, pool.Start(ctx))

	require.Eventually(t, func() bool {
		summaries, listErr := summaryRepo.ListByPost(ctx, post.ID)
		return listErr == nil && len(summaries) == 1
	}, 5*time.Second, 50*time.Millisecond, "post summary must be created")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, pool.Stop(stopCtx))

	// After completion the event must no longer be an "active" one.
	active, _, err := outboxRepo.HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizePost)
	require.NoError(t, err)
	assert.False(t, active, "event must be terminal after processing")
}

func TestIntegration_EventWorkerPool_ParallelUniqueness(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	outboxRepo := data.NewSummaryOutboxRepo(d)
	summaryRepo := data.NewSummaryRepo(d)

	// N distinct sources, each with one post and one summarize_post event.
	const n = 8
	postIDs := make([]string, 0, n)
	for range n {
		srcID := createTestGroupSource(ctx, t, d)
		p := createTestPost(ctx, t, d, srcID, "payload "+uuid.NewString())
		postIDRef := p.ID
		_, err := outboxRepo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, srcID, &postIDRef))
		require.NoError(t, err)
		postIDs = append(postIDs, p.ID)
	}

	llm := &fakeLLM{reply: "concurrent summary"}
	cfg := testSummaryCfg()
	cfg.Worker.Concurrency = 4

	// Two independent pools simulating two processes racing.
	pool1 := task.NewEventWorkerPool(outboxRepo, newTestUsecase(t, d, cfg, llm), cfg, testLogger())
	pool2 := task.NewEventWorkerPool(outboxRepo, newTestUsecase(t, d, cfg, llm), cfg, testLogger())

	require.NoError(t, pool1.Start(ctx))
	require.NoError(t, pool2.Start(ctx))

	require.Eventually(t, func() bool {
		for _, pid := range postIDs {
			summaries, err := summaryRepo.ListByPost(ctx, pid)
			if err != nil || len(summaries) == 0 {
				return false
			}
		}
		return true
	}, 10*time.Second, 100*time.Millisecond, "every post gets a summary")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, pool1.Stop(stopCtx))
	require.NoError(t, pool2.Stop(stopCtx))

	// No duplicates: exactly one Summary per post.
	for _, pid := range postIDs {
		summaries, err := summaryRepo.ListByPost(ctx, pid)
		require.NoError(t, err)
		assert.Len(t, summaries, 1, "post %s should have exactly one summary", pid)
	}
}

func TestIntegration_EventWorkerPool_GracefulDrainOnStop(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "slow content for drain test")

	outboxRepo := data.NewSummaryOutboxRepo(d)
	summaryRepo := data.NewSummaryRepo(d)

	postIDRef := post.ID
	_, err := outboxRepo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, sourceID, &postIDRef))
	require.NoError(t, err)

	llm := &fakeLLM{reply: "drained", delay: 300 * time.Millisecond}
	cfg := testSummaryCfg()

	pool := task.NewEventWorkerPool(outboxRepo, newTestUsecase(t, d, cfg, llm), cfg, testLogger())
	require.NoError(t, pool.Start(ctx))

	// Give a claimer time to pick the event up.
	time.Sleep(100 * time.Millisecond)

	// Stop with enough graceful window to let the in-flight handler finish.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, pool.Stop(stopCtx))

	summaries, err := summaryRepo.ListByPost(ctx, post.ID)
	require.NoError(t, err)
	assert.Len(t, summaries, 1, "in-flight event drained to completion")
}

func TestIntegration_EventWorkerPool_CrashRecoveryViaLeaseExpiry(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "content")

	outboxRepo := data.NewSummaryOutboxRepo(d)

	postIDRef := post.ID
	saved, err := outboxRepo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, sourceID, &postIDRef))
	require.NoError(t, err)

	// Simulate first process claiming and crashing (short lease).
	_, err = outboxRepo.ClaimOne(ctx, "crashed-worker", 150*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(250 * time.Millisecond) // lease expires

	// Second process takes over.
	cfg := testSummaryCfg()
	cfg.Worker.Concurrency = 1
	llm := &fakeLLM{reply: "recovered"}

	pool := task.NewEventWorkerPool(outboxRepo, newTestUsecase(t, d, cfg, llm), cfg, testLogger())
	require.NoError(t, pool.Start(ctx))

	require.Eventually(t, func() bool {
		fetched, getErr := outboxRepo.Get(ctx, saved.ID)
		return getErr == nil && fetched.Status == biz.SummaryEventStatusCompleted
	}, 5*time.Second, 50*time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, pool.Stop(stopCtx))

	fetched, err := outboxRepo.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, fetched.AttemptCount, 2, "attempt_count must reflect at least 2 claims (crash + recovery)")
}

func TestIntegration_EventWorkerPool_IdempotentOnExistingSummary(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "already-summarized post")

	outboxRepo := data.NewSummaryOutboxRepo(d)
	summaryRepo := data.NewSummaryRepo(d)

	// Pre-seed a summary for the post.
	postIDRef := post.ID
	preSum, err := summaryRepo.Save(ctx, biz.Summary{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    &postIDRef,
		SourceID:  sourceID,
		Text:      "pre-existing summary",
		WordCount: 3,
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	_, err = outboxRepo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, sourceID, &postIDRef))
	require.NoError(t, err)

	llm := &fakeLLM{err: errors.New("LLM MUST NOT be called")}
	cfg := testSummaryCfg()

	pool := task.NewEventWorkerPool(outboxRepo, newTestUsecase(t, d, cfg, llm), cfg, testLogger())
	require.NoError(t, pool.Start(ctx))

	require.Eventually(t, func() bool {
		summaries, listErr := summaryRepo.ListByPost(ctx, post.ID)
		return listErr == nil && len(summaries) == 1
	}, 5*time.Second, 50*time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, pool.Stop(stopCtx))

	assert.Zero(t, llm.CallCount(), "LLM must not be called for an already-summarized post")
	_ = preSum
}

// TestIntegration_EventWorkerPool_GracefulTimeoutDoesNotFailEvent asserts that when
// Stop exceeds graceful_timeout, the in-flight event is NOT written as permanent
// Failed via FinalizeWithLease — instead the worker abandons, lease eventually expires
// and the event is eligible for re-claim. Regression guard for H1.
func TestIntegration_EventWorkerPool_GracefulTimeoutDoesNotFailEvent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "content that takes long to summarize")

	outboxRepo := data.NewSummaryOutboxRepo(d)
	summaryRepo := data.NewSummaryRepo(d)

	postIDRef := post.ID
	saved, err := outboxRepo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, sourceID, &postIDRef))
	require.NoError(t, err)

	// LLM delay >> graceful_timeout so Stop is guaranteed to cancel the handler mid-call.
	llm := &fakeLLM{reply: "should-never-be-saved", delay: 3 * time.Second}
	cfg := testSummaryCfg()
	cfg.Worker.Concurrency = 1
	cfg.Worker.GracefulTimeout = durationpb.New(150 * time.Millisecond)

	pool := task.NewEventWorkerPool(outboxRepo, newTestUsecase(t, d, cfg, llm), cfg, testLogger())
	require.NoError(t, pool.Start(ctx))

	// Give the worker time to claim and enter LLM call.
	time.Sleep(200 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	require.NoError(t, pool.Stop(stopCtx))

	fetched, err := outboxRepo.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.NotEqual(t, biz.SummaryEventStatusFailed, fetched.Status,
		"graceful-timeout cancel must not mark event as Failed; lease should be left to expire")
	summaries, err := summaryRepo.ListByPost(ctx, post.ID)
	require.NoError(t, err)
	assert.Empty(t, summaries, "handler was cancelled mid-LLM; no Summary should be saved")
}
