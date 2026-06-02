package task_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/task"
)

func TestIntegration_StuckScoreEventReaper_TerminatesExhausted(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()
	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "post body text")
	createTestSummary(ctx, t, d, sourceID, post.ID)

	outbox := data.NewScoreOutboxRepo(d)
	_, err := outbox.EnqueueDuePosts(ctx)
	require.NoError(t, err)
	// Claim with a tiny lease so it becomes a stuck processing row past grace.
	_, err = outbox.ClaimOne(ctx, "dead-worker", time.Millisecond)
	require.NoError(t, err)
	time.Sleep(60 * time.Millisecond)

	// MaxAttempts=1 so the single claim already exhausted the budget.
	cfg := testScoringCfg()
	cfg.Worker.MaxAttempts = 1
	uc := newTestScoreUsecase(t, d, cfg, &fakeScorer{vec: []float32{1, 0}})
	reaper := task.NewStuckScoreEventReaper(uc, cfg, testLogger())

	// Act
	require.NoError(t, reaper.Start(ctx))
	defer func() { require.NoError(t, reaper.Stop(context.Background())) }()

	// Assert
	require.Eventually(t, func() bool {
		return scoreEventStatus(ctx, t, d, post.ID) == "failed"
	}, 15*time.Second, 50*time.Millisecond)
}
