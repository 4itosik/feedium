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

// seedSource creates an RSS source and returns its id.
func seedSource(t *testing.T, d *data.Data) string {
	t.Helper()
	repo := data.NewSourceRepo(d)
	now := time.Now()
	src, err := repo.Save(context.Background(), biz.Source{
		Type:      biz.SourceTypeRSS,
		Config:    &biz.RSSConfig{FeedURL: "https://example.com/feed"},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	return src.ID
}

// seedPost creates a post for the source and returns its id.
func seedPost(t *testing.T, d *data.Data, sourceID, externalID string, publishedAt time.Time) string {
	t.Helper()
	repo := data.NewPostRepo(d)
	saved, _, err := repo.Save(context.Background(), biz.Post{
		ID:          uuid.Must(uuid.NewV7()).String(),
		SourceID:    sourceID,
		ExternalID:  externalID,
		PublishedAt: publishedAt,
		Text:        "post body " + externalID,
		Metadata:    map[string]string{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
	require.NoError(t, err)
	return saved.ID
}

func newScore(postID string, value float64) biz.Score {
	now := time.Now()
	return biz.Score{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    postID,
		Value:     value,
		Model:     "test-model",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestIntegration_ScoreRepo_UpsertIdempotent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreRepo(d)
	sourceID := seedSource(t, d)
	postID := seedPost(t, d, sourceID, "ext-1", time.Now())

	// Act
	_, err := repo.Upsert(ctx, newScore(postID, 0.3))
	require.NoError(t, err)
	second, err := repo.Upsert(ctx, newScore(postID, 0.7))

	// Assert
	require.NoError(t, err)
	assert.InDelta(t, 0.7, second.Value, 1e-9)

	feed, err := repo.ListRanked(ctx, biz.FeedQuery{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, feed.Items, 1)
	assert.InDelta(t, 0.7, feed.Items[0].Score, 1e-9)
}

func TestIntegration_ScoreRepo_ListRankedOrderAndFilters(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreRepo(d)
	sourceID := seedSource(t, d)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	high := seedPost(t, d, sourceID, "ext-high", base.Add(48*time.Hour))
	mid := seedPost(t, d, sourceID, "ext-mid", base.Add(24*time.Hour))
	low := seedPost(t, d, sourceID, "ext-low", base)
	_, err := repo.Upsert(ctx, newScore(high, 0.9))
	require.NoError(t, err)
	_, err = repo.Upsert(ctx, newScore(mid, 0.5))
	require.NoError(t, err)
	_, err = repo.Upsert(ctx, newScore(low, 0.1))
	require.NoError(t, err)

	// Act
	feed, err := repo.ListRanked(ctx, biz.FeedQuery{MinScore: 0.4, PageSize: 10})

	// Assert
	require.NoError(t, err)
	require.Len(t, feed.Items, 2)
	assert.InDelta(t, 0.9, feed.Items[0].Score, 1e-9)
	assert.InDelta(t, 0.5, feed.Items[1].Score, 1e-9)

	// time filter excludes the oldest two when from is set past them
	from := base.Add(36 * time.Hour)
	filtered, err := repo.ListRanked(ctx, biz.FeedQuery{From: &from, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, high, filtered.Items[0].Post.ID)
}

func TestIntegration_ScoreRepo_ListRankedPagination(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreRepo(d)
	sourceID := seedSource(t, d)
	for i, v := range []float64{0.9, 0.6, 0.3} {
		postID := seedPost(t, d, sourceID, "ext-"+string(rune('a'+i)), time.Now())
		_, err := repo.Upsert(ctx, newScore(postID, v))
		require.NoError(t, err)
	}

	// Act
	first, err := repo.ListRanked(ctx, biz.FeedQuery{PageSize: 2})

	// Assert
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	require.NotEmpty(t, first.NextPageToken)
	assert.InDelta(t, 0.9, first.Items[0].Score, 1e-9)

	second, err := repo.ListRanked(ctx, biz.FeedQuery{PageSize: 2, PageToken: first.NextPageToken})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.InDelta(t, 0.3, second.Items[0].Score, 1e-9)
	assert.Empty(t, second.NextPageToken)
}
