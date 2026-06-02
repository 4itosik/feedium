package data_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
)

// seedSummary persists a summary for the given post so the post becomes
// eligible for scoring enqueue.
func seedSummary(t *testing.T, d *data.Data, sourceID, postID string) {
	t.Helper()
	repo := data.NewSummaryRepo(d)
	pid := postID
	_, err := repo.Save(context.Background(), biz.Summary{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    &pid,
		SourceID:  sourceID,
		Text:      "summary text",
		WordCount: 2,
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)
}

func TestIntegration_ScoreOutbox_EnqueueDuePostsIdempotent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreOutboxRepo(d)
	sourceID := seedSource(t, d)
	postID := seedPost(t, d, sourceID, "ext-1", time.Now())
	seedSummary(t, d, sourceID, postID)

	// Act
	first, err := repo.EnqueueDuePosts(ctx)
	require.NoError(t, err)
	second, err := repo.EnqueueDuePosts(ctx)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, first)
	assert.Equal(t, 0, second, "an active event blocks re-enqueue (INV-04)")
}

func TestIntegration_ScoreOutbox_ClaimAndFinalize(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreOutboxRepo(d)
	sourceID := seedSource(t, d)
	postID := seedPost(t, d, sourceID, "ext-1", time.Now())
	seedSummary(t, d, sourceID, postID)
	_, err := repo.EnqueueDuePosts(ctx)
	require.NoError(t, err)

	// Act
	claimed, err := repo.ClaimOne(ctx, "worker-1", 5*time.Minute)
	require.NoError(t, err)
	require.Equal(t, postID, claimed.PostID)
	require.Equal(t, 1, claimed.AttemptCount)
	finErr := repo.FinalizeWithLease(ctx, claimed.ID, "worker-1", biz.ScoreEventStatusCompleted, nil)

	// Assert
	require.NoError(t, finErr)
	_, claimErr := repo.ClaimOne(ctx, "worker-1", 5*time.Minute)
	assert.ErrorIs(t, claimErr, biz.ErrNoScoreEventAvailable)
}

func TestIntegration_ScoreOutbox_FinalizeWithWrongWorkerLosesLease(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreOutboxRepo(d)
	sourceID := seedSource(t, d)
	postID := seedPost(t, d, sourceID, "ext-1", time.Now())
	seedSummary(t, d, sourceID, postID)
	_, err := repo.EnqueueDuePosts(ctx)
	require.NoError(t, err)
	claimed, err := repo.ClaimOne(ctx, "worker-1", 5*time.Minute)
	require.NoError(t, err)

	// Act
	err = repo.FinalizeWithLease(ctx, claimed.ID, "intruder", biz.ScoreEventStatusCompleted, nil)

	// Assert
	assert.ErrorIs(t, err, biz.ErrLeaseLost)
}

func TestIntegration_ScoreOutbox_MarkForRetryReclaims(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreOutboxRepo(d)
	sourceID := seedSource(t, d)
	postID := seedPost(t, d, sourceID, "ext-1", time.Now())
	seedSummary(t, d, sourceID, postID)
	_, err := repo.EnqueueDuePosts(ctx)
	require.NoError(t, err)
	claimed, err := repo.ClaimOne(ctx, "worker-1", 5*time.Minute)
	require.NoError(t, err)

	// Act
	err = repo.MarkForRetry(ctx, claimed.ID, "worker-1", time.Now().Add(-time.Second), "transient")
	require.NoError(t, err)
	reclaimed, err := repo.ClaimOne(ctx, "worker-2", 5*time.Minute)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, claimed.ID, reclaimed.ID)
	assert.Equal(t, 2, reclaimed.AttemptCount)
}

func TestIntegration_ScoreOutbox_ExtendLease(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreOutboxRepo(d)
	sourceID := seedSource(t, d)
	postID := seedPost(t, d, sourceID, "ext-1", time.Now())
	seedSummary(t, d, sourceID, postID)
	_, err := repo.EnqueueDuePosts(ctx)
	require.NoError(t, err)
	claimed, err := repo.ClaimOne(ctx, "worker-1", 5*time.Minute)
	require.NoError(t, err)

	// Act
	ownErr := repo.ExtendLease(ctx, claimed.ID, "worker-1", 5*time.Minute)
	lostErr := repo.ExtendLease(ctx, claimed.ID, "intruder", 5*time.Minute)

	// Assert
	require.NoError(t, ownErr)
	assert.ErrorIs(t, lostErr, biz.ErrLeaseLost)
}

func TestIntegration_ScoreOutbox_FailExpired(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreOutboxRepo(d)
	sourceID := seedSource(t, d)
	postID := seedPost(t, d, sourceID, "ext-1", time.Now())
	seedSummary(t, d, sourceID, postID)
	_, err := repo.EnqueueDuePosts(ctx)
	require.NoError(t, err)
	claimed, err := repo.ClaimOne(ctx, "worker-1", time.Millisecond)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Act
	expired, err := repo.ListLeaseExpired(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	ok, err := repo.FailExpired(ctx, claimed.ID, 1, 0, "max attempts exceeded")

	// Assert
	require.NoError(t, err)
	assert.True(t, ok)
}
