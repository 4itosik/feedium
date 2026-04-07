package connect_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	summaryv1 "feedium/api/summary/v1"
	"feedium/internal/app/summary"
	summaryconnect "feedium/internal/app/summary/adapters/connect"
)

// mockRepository is a mock implementation of summary.Repository for testing.
type mockRepository struct {
	getByPostIDFunc   func(postID uuid.UUID) (*summary.Summary, []uuid.UUID, error)
	listSummariesFunc func(sourceID *uuid.UUID, limit int) ([]summary.WithPostIDs, error)
}

func (m *mockRepository) Create(_ context.Context, _ *summary.Summary, _ []uuid.UUID) error {
	return nil
}

func (m *mockRepository) GetByPostID(_ context.Context, postID uuid.UUID) (*summary.Summary, []uuid.UUID, error) {
	if m.getByPostIDFunc != nil {
		return m.getByPostIDFunc(postID)
	}
	return nil, nil, nil
}

func (m *mockRepository) ListSummaries(
	_ context.Context,
	sourceID *uuid.UUID,
	limit int,
) ([]summary.WithPostIDs, error) {
	if m.listSummariesFunc != nil {
		return m.listSummariesFunc(sourceID, limit)
	}
	return nil, nil
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHandler_GetSummaryByPost_Success(t *testing.T) {
	summaryID := uuid.New()
	sourceID := uuid.New()
	eventID := uuid.New()
	postID := uuid.New()
	postID2 := uuid.New()

	repo := &mockRepository{
		getByPostIDFunc: func(_ uuid.UUID) (*summary.Summary, []uuid.UUID, error) {
			return &summary.Summary{
				ID:        summaryID,
				SourceID:  sourceID,
				EventID:   eventID,
				Content:   "Test summary content",
				CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			}, []uuid.UUID{postID, postID2}, nil
		},
	}

	handler := summaryconnect.New(repo, newTestLogger())
	req := connect.NewRequest(&summaryv1.GetSummaryByPostRequest{PostId: postID.String()})

	resp, err := handler.GetSummaryByPost(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, summaryID.String(), resp.Msg.GetSummary().GetId())
	assert.Equal(t, "Test summary content", resp.Msg.GetSummary().GetContent())
	assert.Len(t, resp.Msg.GetSummary().GetPostIds(), 2)
}

func TestHandler_GetSummaryByPost_NotFound(t *testing.T) {
	repo := &mockRepository{
		getByPostIDFunc: func(_ uuid.UUID) (*summary.Summary, []uuid.UUID, error) {
			return nil, nil, nil
		},
	}

	handler := summaryconnect.New(repo, newTestLogger())
	req := connect.NewRequest(&summaryv1.GetSummaryByPostRequest{PostId: uuid.New().String()})

	_, err := handler.GetSummaryByPost(context.Background(), req)

	require.Error(t, err)
	connErr := new(connect.Error)
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeNotFound, connErr.Code())
}

func TestHandler_GetSummaryByPost_InvalidUUID(t *testing.T) {
	repo := &mockRepository{}
	handler := summaryconnect.New(repo, newTestLogger())
	req := connect.NewRequest(&summaryv1.GetSummaryByPostRequest{PostId: "invalid-uuid"})

	_, err := handler.GetSummaryByPost(context.Background(), req)

	require.Error(t, err)
	connErr := new(connect.Error)
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestHandler_GetSummaryByPost_EmptyUUID(t *testing.T) {
	repo := &mockRepository{}
	handler := summaryconnect.New(repo, newTestLogger())
	req := connect.NewRequest(&summaryv1.GetSummaryByPostRequest{PostId: "  "})

	_, err := handler.GetSummaryByPost(context.Background(), req)

	require.Error(t, err)
	connErr := new(connect.Error)
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestHandler_GetSummaryByPost_RepositoryError(t *testing.T) {
	repo := &mockRepository{
		getByPostIDFunc: func(_ uuid.UUID) (*summary.Summary, []uuid.UUID, error) {
			return nil, nil, errors.New("database error")
		},
	}

	handler := summaryconnect.New(repo, newTestLogger())
	req := connect.NewRequest(&summaryv1.GetSummaryByPostRequest{PostId: uuid.New().String()})

	_, err := handler.GetSummaryByPost(context.Background(), req)

	require.Error(t, err)
	connErr := new(connect.Error)
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInternal, connErr.Code())
}

func TestHandler_ListSummariesBySource_Success(t *testing.T) {
	summaryID1 := uuid.New()
	summaryID2 := uuid.New()
	sourceID := uuid.New()

	repo := &mockRepository{
		listSummariesFunc: func(_ *uuid.UUID, _ int) ([]summary.WithPostIDs, error) {
			return []summary.WithPostIDs{
				{
					Summary: summary.Summary{
						ID:        summaryID1,
						SourceID:  sourceID,
						EventID:   uuid.New(),
						Content:   "Summary 1",
						CreatedAt: time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
					},
					PostIDs: []uuid.UUID{uuid.New()},
				},
				{
					Summary: summary.Summary{
						ID:        summaryID2,
						SourceID:  sourceID,
						EventID:   uuid.New(),
						Content:   "Summary 2",
						CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
					},
					PostIDs: []uuid.UUID{uuid.New(), uuid.New()},
				},
			}, nil
		},
	}

	handler := summaryconnect.New(repo, newTestLogger())
	req := connect.NewRequest(&summaryv1.ListSummariesBySourceRequest{
		SourceId: sourceID.String(),
		Limit:    10,
	})

	resp, err := handler.ListSummariesBySource(context.Background(), req)

	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetSummaries(), 2)
	assert.Equal(t, "Summary 1", resp.Msg.GetSummaries()[0].GetContent())
	assert.Equal(t, "Summary 2", resp.Msg.GetSummaries()[1].GetContent())
}

func TestHandler_ListSummariesBySource_AllSources(t *testing.T) {
	repo := &mockRepository{
		listSummariesFunc: func(sourceID *uuid.UUID, _ int) ([]summary.WithPostIDs, error) {
			// Verify sourceID is nil for all sources
			assert.Nil(t, sourceID)
			return []summary.WithPostIDs{}, nil
		},
	}

	handler := summaryconnect.New(repo, newTestLogger())
	req := connect.NewRequest(&summaryv1.ListSummariesBySourceRequest{
		Limit: 10,
	})

	resp, err := handler.ListSummariesBySource(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetSummaries())
}

func TestHandler_ListSummariesBySource_InvalidLimit(t *testing.T) {
	repo := &mockRepository{}
	handler := summaryconnect.New(repo, newTestLogger())

	// Test limit = 0
	req := connect.NewRequest(&summaryv1.ListSummariesBySourceRequest{Limit: 0})
	_, err := handler.ListSummariesBySource(context.Background(), req)
	require.Error(t, err)
	connErr := new(connect.Error)
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())

	// Test limit = 101 (too high)
	req = connect.NewRequest(&summaryv1.ListSummariesBySourceRequest{Limit: 101})
	_, err = handler.ListSummariesBySource(context.Background(), req)
	require.Error(t, err)
	connErr = new(connect.Error)
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestHandler_ListSummariesBySource_InvalidSourceID(t *testing.T) {
	repo := &mockRepository{}
	handler := summaryconnect.New(repo, newTestLogger())
	req := connect.NewRequest(&summaryv1.ListSummariesBySourceRequest{
		SourceId: "invalid-uuid",
		Limit:    10,
	})

	_, err := handler.ListSummariesBySource(context.Background(), req)

	require.Error(t, err)
	connErr := new(connect.Error)
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestHandler_ListSummariesBySource_RepositoryError(t *testing.T) {
	repo := &mockRepository{
		listSummariesFunc: func(_ *uuid.UUID, _ int) ([]summary.WithPostIDs, error) {
			return nil, errors.New("database error")
		},
	}

	handler := summaryconnect.New(repo, newTestLogger())
	req := connect.NewRequest(&summaryv1.ListSummariesBySourceRequest{
		Limit: 10,
	})

	_, err := handler.ListSummariesBySource(context.Background(), req)

	require.Error(t, err)
	connErr := new(connect.Error)
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInternal, connErr.Code())
}
