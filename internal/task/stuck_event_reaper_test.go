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

// TestIntegration_StuckEventReaper_DoesNotStompFreshClaim asserts that FailExpired
// refuses to terminate when the enumerated row has been re-claimed with a live lease
// between ListLeaseExpired and the terminal write. Regression guard for H2.
func TestIntegration_StuckEventReaper_DoesNotStompFreshClaim(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	outbox := data.NewSummaryOutboxRepo(d)
	sourceID := createTestGroupSource(ctx, t, d)

	saved, err := outbox.Save(ctx, biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil))
	require.NoError(t, err)

	// Push attempt_count to max via 3 expired claims.
	for range 3 {
		_, err = outbox.ClaimOne(ctx, "reaper-race-loser", 20*time.Millisecond)
		require.NoError(t, err)
		time.Sleep(30 * time.Millisecond)
	}

	// Fresh claim by a live worker — new lease (2s), attempt_count now 4.
	claimed, err := outbox.ClaimOne(ctx, "fresh-worker", 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, 4, claimed.AttemptCount)

	// Reaper tries to terminate directly via FailExpired: guard must refuse because
	// locked_until > now() - grace (lease is fresh).
	terminated, err := outbox.FailExpired(ctx, saved.ID, 3, 10*time.Millisecond, "max attempts exceeded")
	require.NoError(t, err)
	assert.False(t, terminated, "fresh claim lease must survive — reaper termination should be rejected by guard")

	// Event must still be 'processing' with the fresh lease intact.
	fetched, err := outbox.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, biz.SummaryEventStatusProcessing, fetched.Status)
	assert.Nil(t, fetched.Error, "no stale 'max attempts exceeded' written over live lease")
}
