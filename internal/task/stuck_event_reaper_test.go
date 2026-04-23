package task_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/task"
)

func TestIntegration_StuckEventReaper_TerminatesAtMaxAttempts(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	outbox := data.NewSummaryOutboxRepo(d)
	sourceID := createTestGroupSource(ctx, t, d)

	saved, err := outbox.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil))
	require.NoError(t, err)

	// Bump attempt_count to 3 by claiming and letting the lease expire three times.
	for range 3 {
		_, err = outbox.ClaimOne(ctx, "test-worker", 20*time.Millisecond)
		require.NoError(t, err)
		time.Sleep(30 * time.Millisecond)
	}

	cfg := testSummaryCfg()
	cfg.Worker.MaxAttempts = 3
	cfg.Reaper = &conf.SummaryReaper{
		Interval: durationpb.New(50 * time.Millisecond),
		Grace:    durationpb.New(10 * time.Millisecond),
	}

	reaper := task.NewStuckEventReaper(outbox, cfg, testLogger())
	require.NoError(t, reaper.Start(ctx))

	require.Eventually(t, func() bool {
		ev, getErr := outbox.Get(ctx, saved.ID)
		return getErr == nil && ev.Status == biz.SummaryEventStatusFailed
	}, 2*time.Second, 50*time.Millisecond)

	require.NoError(t, reaper.Stop(ctx))

	fetched, err := outbox.Get(ctx, saved.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched.Error)
	assert.Equal(t, "max attempts exceeded", *fetched.Error)
}

func TestIntegration_StuckEventReaper_BelowMaxAttempts_NoTerminal(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	outbox := data.NewSummaryOutboxRepo(d)
	sourceID := createTestGroupSource(ctx, t, d)

	saved, err := outbox.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil))
	require.NoError(t, err)

	// Single claim — attempt_count = 1 < max.
	_, err = outbox.ClaimOne(ctx, "test-worker", 20*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(30 * time.Millisecond)

	cfg := testSummaryCfg()
	cfg.Worker.MaxAttempts = 5
	cfg.Reaper = &conf.SummaryReaper{
		Interval: durationpb.New(50 * time.Millisecond),
		Grace:    durationpb.New(10 * time.Millisecond),
	}

	reaper := task.NewStuckEventReaper(outbox, cfg, testLogger())
	require.NoError(t, reaper.Start(ctx))
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, reaper.Stop(ctx))

	fetched, err := outbox.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, biz.SummaryEventStatusProcessing, fetched.Status)
}
