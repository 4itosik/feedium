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

func countScoreEvents(ctx context.Context, t *testing.T, d *data.Data, postID string) int {
	t.Helper()
	var n int
	err := d.DB.QueryRowContext(ctx, `SELECT count(*) FROM score_events WHERE post_id = $1`, postID).Scan(&n)
	require.NoError(t, err)
	return n
}

func TestIntegration_ScoreDueScheduler_EnqueuesSummarizedUnscored(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()
	sourceID := createTestGroupSource(ctx, t, d)
	post := createTestPost(ctx, t, d, sourceID, "post body text")
	createTestSummary(ctx, t, d, sourceID, post.ID)
	cfg := testScoringCfg()
	uc := newTestScoreUsecase(t, d, cfg, &fakeScorer{vec: []float32{1, 0}})
	scheduler := task.NewScoreDueScheduler(uc, cfg, testLogger())

	// Act
	require.NoError(t, scheduler.Start(ctx))
	defer func() { require.NoError(t, scheduler.Stop(context.Background())) }()

	// Assert
	require.Eventually(t, func() bool {
		return countScoreEvents(ctx, t, d, post.ID) == 1
	}, 15*time.Second, 50*time.Millisecond)

	// A second tick must not create a duplicate active event (INV-04).
	time.Sleep(150 * time.Millisecond)
	require.Equal(t, 1, countScoreEvents(ctx, t, d, post.ID))
}
