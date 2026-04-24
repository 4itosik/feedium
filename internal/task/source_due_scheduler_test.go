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

	cfg := testSummaryCfg()
	uc := newTestUsecase(t, d, cfg, nil)
	scheduler := task.NewSourceDueScheduler(uc, cfg, testLogger())
	require.NoError(t, scheduler.Start(ctx))

	require.Eventually(t, func() bool {
		active, _, err := outboxRepo.HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizeSource)
		return err == nil && active
	}, 3*time.Second, 50*time.Millisecond)

	require.NoError(t, scheduler.Stop(ctx))

	// next_summary_at bumped into the future → second claim is empty.
	_, err := sourceRepo.ClaimDueCumulative(ctx)
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

	_, err := outboxRepo.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil))
	require.NoError(t, err)

	cfg := testSummaryCfg()
	uc := newTestUsecase(t, d, cfg, nil)
	scheduler := task.NewSourceDueScheduler(uc, cfg, testLogger())
	require.NoError(t, scheduler.Start(ctx))

	time.Sleep(200 * time.Millisecond)
	require.NoError(t, scheduler.Stop(ctx))

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

	outboxRepo := data.NewSummaryOutboxRepo(d)

	cfg := testSummaryCfg()
	uc := newTestUsecase(t, d, cfg, nil)
	scheduler := task.NewSourceDueScheduler(uc, cfg, testLogger())
	require.NoError(t, scheduler.Start(ctx))

	time.Sleep(200 * time.Millisecond)
	require.NoError(t, scheduler.Stop(ctx))

	active, _, err := outboxRepo.HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizeSource)
	require.NoError(t, err)
	assert.False(t, active, "no event should be created when no new posts")
}
