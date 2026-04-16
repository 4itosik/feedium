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

func TestIntegration_OutboxRepo_SaveAndGet(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)

	saved, err := repo.Save(ctx, event)
	require.NoError(t, err)
	assert.NotEmpty(t, saved.ID)
	assert.Equal(t, sourceID, saved.SourceID)
	assert.Equal(t, biz.SummaryEventTypeSummarizeSource, saved.EventType)
	assert.Equal(t, biz.SummaryEventStatusPending, saved.Status)
	assert.False(t, saved.CreatedAt.IsZero())
	assert.Nil(t, saved.PostID)
	assert.Nil(t, saved.SummaryID)
	assert.Nil(t, saved.Error)
	assert.Nil(t, saved.ProcessedAt)

	fetched, err := repo.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, fetched.ID)
	assert.Equal(t, saved.SourceID, fetched.SourceID)
	assert.Equal(t, saved.EventType, fetched.EventType)
	assert.Equal(t, saved.Status, fetched.Status)
}

func TestIntegration_OutboxRepo_ListPending(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	event1 := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
	saved1, err := repo.Save(ctx, event1)
	require.NoError(t, err)

	postID := uuid.New().String()
	event2 := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, sourceID, &postID)
	saved2, err := repo.Save(ctx, event2)
	require.NoError(t, err)

	err = repo.UpdateStatus(ctx, saved1.ID, biz.SummaryEventStatusCompleted, nil, nil)
	require.NoError(t, err)

	pending, err := repo.ListPending(ctx, 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, saved2.ID, pending[0].ID)
	assert.Equal(t, biz.SummaryEventStatusPending, pending[0].Status)
}

func TestIntegration_OutboxRepo_ListPending_SortedByCreatedAt(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID1 := createTestSource(ctx, t, client)
	sourceID2 := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	event1 := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID1, nil)
	saved1, err := repo.Save(ctx, event1)
	require.NoError(t, err)

	event2 := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID2, nil)
	saved2, err := repo.Save(ctx, event2)
	require.NoError(t, err)

	pending, err := repo.ListPending(ctx, 10)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, saved1.ID, pending[0].ID)
	assert.Equal(t, saved2.ID, pending[1].ID)
	assert.True(t, pending[0].CreatedAt.Before(pending[1].CreatedAt) || pending[0].CreatedAt.Equal(pending[1].CreatedAt))
}

func TestIntegration_OutboxRepo_UpdateStatus_Completed(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	summaryRepo := data.NewSummaryRepo(&data.Data{Ent: client})
	outboxRepo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
	saved, err := outboxRepo.Save(ctx, event)
	require.NoError(t, err)

	// Create a summary first before referencing it
	summary := biz.Summary{
		ID:        uuid.New().String(),
		SourceID:  sourceID,
		Text:      "Test summary",
		WordCount: 2,
		CreatedAt: time.Now(),
	}
	savedSummary, err := summaryRepo.Save(ctx, summary)
	require.NoError(t, err)

	err = outboxRepo.UpdateStatus(ctx, saved.ID, biz.SummaryEventStatusCompleted, &savedSummary.ID, nil)
	require.NoError(t, err)

	updated, err := outboxRepo.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, biz.SummaryEventStatusCompleted, updated.Status)
	require.NotNil(t, updated.SummaryID)
	assert.Equal(t, savedSummary.ID, *updated.SummaryID)
	assert.NotNil(t, updated.ProcessedAt)
	assert.False(t, updated.ProcessedAt.IsZero())
}

func TestIntegration_OutboxRepo_UpdateStatus_Failed(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
	saved, err := repo.Save(ctx, event)
	require.NoError(t, err)

	errText := "LLM rate limit exceeded"
	err = repo.UpdateStatus(ctx, saved.ID, biz.SummaryEventStatusFailed, nil, &errText)
	require.NoError(t, err)

	updated, err := repo.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, biz.SummaryEventStatusFailed, updated.Status)
	require.NotNil(t, updated.Error)
	assert.Equal(t, errText, *updated.Error)
	assert.NotNil(t, updated.ProcessedAt)
	assert.False(t, updated.ProcessedAt.IsZero())
}

func TestIntegration_OutboxRepo_UpdateStatus_Expired(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
	saved, err := repo.Save(ctx, event)
	require.NoError(t, err)

	err = repo.UpdateStatus(ctx, saved.ID, biz.SummaryEventStatusExpired, nil, nil)
	require.NoError(t, err)

	updated, err := repo.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, biz.SummaryEventStatusExpired, updated.Status)
	assert.Nil(t, updated.SummaryID)
	assert.Nil(t, updated.Error)
	assert.NotNil(t, updated.ProcessedAt)
	assert.False(t, updated.ProcessedAt.IsZero())
}

func TestIntegration_OutboxRepo_HasActiveEvent_True(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
	saved, err := repo.Save(ctx, event)
	require.NoError(t, err)

	found, activeEvent, err := repo.HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizeSource)
	require.NoError(t, err)
	assert.True(t, found)
	require.NotNil(t, activeEvent)
	assert.Equal(t, saved.ID, activeEvent.ID)
	assert.Equal(t, biz.SummaryEventStatusPending, activeEvent.Status)
}

func TestIntegration_OutboxRepo_HasActiveEvent_False(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	found, activeEvent, err := repo.HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizeSource)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, activeEvent)
}

func TestIntegration_OutboxRepo_HasActiveEvent_Failed_NotBlocked(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
	saved, err := repo.Save(ctx, event)
	require.NoError(t, err)

	err = repo.UpdateStatus(ctx, saved.ID, biz.SummaryEventStatusFailed, nil, nil)
	require.NoError(t, err)

	found, activeEvent, err := repo.HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizeSource)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, activeEvent)
}

func TestIntegration_OutboxRepo_UniqueActivePost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	postID := uuid.New().String()

	event1 := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, sourceID, &postID)
	_, err := repo.Save(ctx, event1)
	require.NoError(t, err)

	event2 := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizePost, sourceID, &postID)
	_, err = repo.Save(ctx, event2)
	assert.ErrorIs(t, err, biz.ErrSummaryAlreadyProcessing)
}

func TestIntegration_OutboxRepo_UniqueActiveSource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	sourceID := createTestSource(ctx, t, client)
	repo := data.NewSummaryOutboxRepo(&data.Data{Ent: client})

	event1 := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
	_, err := repo.Save(ctx, event1)
	require.NoError(t, err)

	event2 := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, sourceID, nil)
	_, err = repo.Save(ctx, event2)
	assert.ErrorIs(t, err, biz.ErrSummaryAlreadyProcessing)
}
