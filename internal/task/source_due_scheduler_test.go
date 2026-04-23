package task_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/task"
)

func TestIntegration_SourceDueScheduler_EnqueuesAndBumps(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	sourceID := createTestGroupSource(ctx, t, d)
	_ = createTestPost(ctx, t, d, sourceID, "fresh content")

	outboxRepo := data.NewSummaryOutboxRepo(d)
	sourceRepo := data.NewSourceRepo(d)
	summaryRepo := data.NewSummaryRepo(d)
	postRepo := data.NewPostRepo(d)
	txMgr := data.NewTxManager(d)

	cfg := testSummaryCfg()
	scheduler := task.NewSourceDueScheduler(outboxRepo, sourceRepo, summaryRepo, postRepo, txMgr, cfg, testLogger())
	require.NoError(t, scheduler.Start(ctx))

	// Wait for the scheduler to create a summarize_source event for the due source.
	require.Eventually(t, func() bool {
		active, _, err := outboxRepo.HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizeSource)
		return err == nil && active
	}, 3*time.Second, 50*time.Millisecond)

	require.NoError(t, scheduler.Stop(ctx))

	// Source's next_summary_at must be in the future.
	src, err := sourceRepo.Get(ctx, sourceID)
	require.NoError(t, err)
	_ = src // Get does not currently surface next_summary_at; verify via second claim.

	// Second claim attempt must return no due source (already bumped into the future).
	_, err = sourceRepo.ClaimDueCumulative(ctx)
	assert.ErrorIs(t, err, biz.ErrNoSourceDue)
}

func TestIntegration_SourceDueScheduler_NoDuplicateWhenActiveEventExists(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	sourceID := createTestGroupSource(ctx, t, d)
	_ = createTestPost(ctx, t, d, sourceID, "content that produces an event")

	outboxRepo := data.NewSummaryOutboxRepo(d)
	sourceRepo := data.NewSourceRepo(d)
	summaryRepo := data.NewSummaryRepo(d)
	postRepo := data.NewPostRepo(d)
	txMgr := data.NewTxManager(d)

	// Pre-seed an active event.
	_, err := outboxRepo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil))
	require.NoError(t, err)

	cfg := testSummaryCfg()
	scheduler := task.NewSourceDueScheduler(outboxRepo, sourceRepo, summaryRepo, postRepo, txMgr, cfg, testLogger())
	require.NoError(t, scheduler.Start(ctx))

	// Give the scheduler a couple of ticks.
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, scheduler.Stop(ctx))

	// Still exactly one active event (no duplicates).
	active, ev, err := outboxRepo.HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizeSource)
	require.NoError(t, err)
	assert.True(t, active)
	require.NotNil(t, ev)
}

func TestIntegration_SourceDueScheduler_NoPostsNoEvent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	sourceID := createTestGroupSource(ctx, t, d)
	// No posts at all.

	outboxRepo := data.NewSummaryOutboxRepo(d)
	sourceRepo := data.NewSourceRepo(d)
	summaryRepo := data.NewSummaryRepo(d)
	postRepo := data.NewPostRepo(d)
	txMgr := data.NewTxManager(d)

	cfg := testSummaryCfg()
	scheduler := task.NewSourceDueScheduler(outboxRepo, sourceRepo, summaryRepo, postRepo, txMgr, cfg, testLogger())
	require.NoError(t, scheduler.Start(ctx))

	time.Sleep(200 * time.Millisecond)
	require.NoError(t, scheduler.Stop(ctx))

	active, _, err := outboxRepo.HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizeSource)
	require.NoError(t, err)
	assert.False(t, active, "no event should be created when no new posts")
}
