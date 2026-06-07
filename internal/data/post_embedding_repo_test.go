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

func TestIntegration_PostEmbeddingRepo_UpsertGetRoundTrip(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewPostEmbeddingRepo(d)
	sourceID := seedSource(t, d)
	postID := seedPost(t, d, sourceID, "ext-emb-1", time.Now())

	pe := biz.PostEmbedding{
		PostID:    postID,
		Vector:    []float32{0.1, 0.2, 0.3, -0.4},
		Model:     "test-embed-model",
		InputHash: "hash-1",
	}

	// Act
	_, err := repo.Upsert(ctx, pe)
	require.NoError(t, err)
	got, err := repo.Get(ctx, postID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, postID, got.PostID)
	assert.Equal(t, pe.Vector, got.Vector)
	assert.Equal(t, "test-embed-model", got.Model)
	assert.Equal(t, "hash-1", got.InputHash)
	assert.False(t, got.CreatedAt.IsZero())
	assert.False(t, got.UpdatedAt.IsZero())
}

func TestIntegration_PostEmbeddingRepo_UpsertOverwritesSingleRow(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewPostEmbeddingRepo(d)
	sourceID := seedSource(t, d)
	postID := seedPost(t, d, sourceID, "ext-emb-2", time.Now())

	first, err := repo.Upsert(ctx, biz.PostEmbedding{
		PostID:    postID,
		Vector:    []float32{1, 2, 3},
		Model:     "model-a",
		InputHash: "hash-a",
	})
	require.NoError(t, err)

	// Act: second upsert with different vector/model/input_hash for the same post
	_, err = repo.Upsert(ctx, biz.PostEmbedding{
		PostID:    postID,
		Vector:    []float32{9, 8, 7, 6},
		Model:     "model-b",
		InputHash: "hash-b",
	})
	require.NoError(t, err)

	// Assert: Get returns the new values
	got, err := repo.Get(ctx, postID)
	require.NoError(t, err)
	assert.Equal(t, []float32{9, 8, 7, 6}, got.Vector)
	assert.Equal(t, "model-b", got.Model)
	assert.Equal(t, "hash-b", got.InputHash)

	// Assert: created_at is immutable across upsert, updated_at advances
	assert.Equal(t, first.CreatedAt.Unix(), got.CreatedAt.Unix(), "created_at must be immutable on upsert")
	assert.False(t, got.UpdatedAt.Before(got.CreatedAt))

	// Assert: exactly ONE row exists for that post_id
	var count int
	err = d.DB.QueryRowContext(ctx,
		"SELECT count(*) FROM post_embeddings WHERE post_id = $1", postID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestIntegration_PostEmbeddingRepo_GetNotFound(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewPostEmbeddingRepo(d)

	// Act
	_, err := repo.Get(ctx, uuid.Must(uuid.NewV7()).String())

	// Assert
	require.ErrorIs(t, err, biz.ErrPostEmbeddingNotFound)
}
