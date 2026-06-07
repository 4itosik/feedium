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

func newInterest(topic string, embedding []float32) biz.Interest {
	now := time.Now()
	return biz.Interest{
		ID:        uuid.Must(uuid.NewV7()).String(),
		Topic:     topic,
		Embedding: embedding,
		Model:     "test-model",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestIntegration_InterestRepo_CRUD(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewInterestRepo(d)

	// Act
	saved, err := repo.Save(ctx, newInterest("go programming", []float32{0.1, 0.2, 0.3}))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "go programming", saved.Topic)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, saved.Embedding)

	got, err := repo.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, got.Embedding)

	got.Topic = "rust"
	got.Embedding = []float32{0.9}
	got.UpdatedAt = time.Now()
	updated, err := repo.Update(ctx, got)
	require.NoError(t, err)
	assert.Equal(t, "rust", updated.Topic)
	assert.Equal(t, []float32{0.9}, updated.Embedding)

	require.NoError(t, repo.Delete(ctx, saved.ID))
	_, err = repo.Get(ctx, saved.ID)
	assert.ErrorIs(t, err, biz.ErrInterestNotFound)
}

func TestIntegration_InterestRepo_NotFound(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewInterestRepo(d)
	missing := newInterest("ghost", []float32{1})

	// Act
	_, updErr := repo.Update(ctx, missing)
	delErr := repo.Delete(ctx, missing.ID)

	// Assert
	require.ErrorIs(t, updErr, biz.ErrInterestNotFound)
	require.ErrorIs(t, delErr, biz.ErrInterestNotFound)
}

func TestIntegration_InterestRepo_ListEmbeddings(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewInterestRepo(d)
	_, err := repo.Save(ctx, newInterest("a", []float32{1, 0}))
	require.NoError(t, err)
	_, err = repo.Save(ctx, newInterest("b", []float32{0, 1}))
	require.NoError(t, err)

	// Act
	embeddings, err := repo.ListEmbeddings(ctx)

	// Assert
	require.NoError(t, err)
	assert.Len(t, embeddings, 2)
}

func TestIntegration_InterestRepo_ListPagination(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewInterestRepo(d)
	for i := range 3 {
		_, err := repo.Save(ctx, newInterest(string(rune('a'+i)), []float32{float32(i)}))
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	// Act
	first, err := repo.List(ctx, biz.ListInterestsFilter{PageSize: 2})

	// Assert
	require.NoError(t, err)
	assert.Len(t, first.Items, 2)
	require.NotEmpty(t, first.NextPageToken)

	second, err := repo.List(ctx, biz.ListInterestsFilter{PageSize: 2, PageToken: first.NextPageToken})
	require.NoError(t, err)
	assert.Len(t, second.Items, 1)
	assert.Empty(t, second.NextPageToken)
}
