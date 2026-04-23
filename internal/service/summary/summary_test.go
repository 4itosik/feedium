package summary_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/service/summary"
	"github.com/4itosik/feedium/internal/service/summary/mock"
)

func makeSummary(id, sourceID string, postID *string) biz.Summary {
	return biz.Summary{
		ID:        id,
		PostID:    postID,
		SourceID:  sourceID,
		Text:      "summary text",
		WordCount: 2,
		CreatedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
	}
}

func TestV1SummarizeSource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"

	t.Run("creates new task", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			TriggerSourceSummarize(gomock.Any(), sourceID).
			Return("task-1", false, nil)
		svc := summary.NewService(uc)

		// Act
		resp, err := svc.V1SummarizeSource(
			ctx,
			&feedium.V1SummarizeSourceRequest{SourceId: sourceID},
		)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "task-1", resp.GetTaskId())
		assert.False(t, resp.GetExisting())
	})

	t.Run("returns existing task", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			TriggerSourceSummarize(gomock.Any(), sourceID).
			Return("existing-id", true, nil)
		svc := summary.NewService(uc)

		// Act
		resp, err := svc.V1SummarizeSource(
			ctx,
			&feedium.V1SummarizeSourceRequest{SourceId: sourceID},
		)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "existing-id", resp.GetTaskId())
		assert.True(t, resp.GetExisting())
	})

	t.Run("source not found", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			TriggerSourceSummarize(gomock.Any(), gomock.Any()).
			Return("", false, biz.ErrSourceNotFound)
		svc := summary.NewService(uc)

		// Act
		_, err := svc.V1SummarizeSource(ctx, &feedium.V1SummarizeSourceRequest{SourceId: sourceID})

		// Assert
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_SOURCE_NOT_FOUND")
	})

	t.Run("self-contained source rejected", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			TriggerSourceSummarize(gomock.Any(), gomock.Any()).
			Return("", false, biz.ErrSummarizeSelfContainedSrc)
		svc := summary.NewService(uc)

		// Act
		_, err := svc.V1SummarizeSource(ctx, &feedium.V1SummarizeSourceRequest{SourceId: sourceID})

		// Assert
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_SUMMARIZE_SELF_CONTAINED_SOURCE")
	})

	t.Run("unknown error maps to internal", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			TriggerSourceSummarize(gomock.Any(), gomock.Any()).
			Return("", false, errors.New("boom"))
		svc := summary.NewService(uc)

		// Act
		_, err := svc.V1SummarizeSource(ctx, &feedium.V1SummarizeSourceRequest{SourceId: sourceID})

		// Assert
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
	})
}

func TestV1GetSummaryEvent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	eventID := "01961d9c-aaaa-7e2e-8c3a-5e7d9a1b2c3d"
	createdAt := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	t.Run("maps full event including optional fields", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		processedAt := createdAt.Add(time.Minute)
		event := &biz.SummaryEvent{
			ID:          eventID,
			PostID:      new("post-1"),
			SourceID:    "source-1",
			EventType:   biz.SummaryEventTypeSummarizePost,
			Status:      biz.SummaryEventStatusCompleted,
			SummaryID:   new("sum-1"),
			Error:       new("partial"),
			CreatedAt:   createdAt,
			ProcessedAt: &processedAt,
		}

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().GetSummaryEvent(gomock.Any(), eventID).Return(event, nil)
		svc := summary.NewService(uc)

		// Act
		resp, err := svc.V1GetSummaryEvent(ctx, &feedium.V1GetSummaryEventRequest{Id: eventID})

		// Assert
		require.NoError(t, err)
		got := resp.GetEvent()
		assert.Equal(t, eventID, got.GetId())
		assert.Equal(t, "source-1", got.GetSourceId())
		assert.Equal(t, "post-1", got.GetPostId())
		assert.Equal(t, "sum-1", got.GetSummaryId())
		assert.Equal(t, "partial", got.GetError())
		assert.Equal(
			t,
			feedium.SummaryEventType_SUMMARY_EVENT_TYPE_SUMMARIZE_POST,
			got.GetEventType(),
		)
		assert.Equal(t, feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_COMPLETED, got.GetStatus())
		assert.True(t, createdAt.Equal(got.GetCreatedAt().AsTime()))
		assert.True(t, processedAt.Equal(got.GetProcessedAt().AsTime()))
	})

	t.Run("maps minimal event with nil optional fields", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		event := &biz.SummaryEvent{
			ID:        eventID,
			SourceID:  "source-1",
			EventType: biz.SummaryEventTypeSummarizeSource,
			Status:    biz.SummaryEventStatusPending,
			CreatedAt: createdAt,
		}

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().GetSummaryEvent(gomock.Any(), eventID).Return(event, nil)
		svc := summary.NewService(uc)

		// Act
		resp, err := svc.V1GetSummaryEvent(ctx, &feedium.V1GetSummaryEventRequest{Id: eventID})

		// Assert
		require.NoError(t, err)
		got := resp.GetEvent()
		assert.Empty(t, got.GetPostId())
		assert.Empty(t, got.GetSummaryId())
		assert.Empty(t, got.GetError())
		assert.Nil(t, got.GetProcessedAt())
		assert.Equal(
			t,
			feedium.SummaryEventType_SUMMARY_EVENT_TYPE_SUMMARIZE_SOURCE,
			got.GetEventType(),
		)
		assert.Equal(t, feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_PENDING, got.GetStatus())
	})

	t.Run("event not found", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().GetSummaryEvent(gomock.Any(), eventID).Return(nil, biz.ErrSummaryEventNotFound)
		svc := summary.NewService(uc)

		// Act
		_, err := svc.V1GetSummaryEvent(ctx, &feedium.V1GetSummaryEventRequest{Id: eventID})

		// Assert
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_SUMMARY_EVENT_NOT_FOUND")
	})
}

func TestV1GetSummaryEvent_StatusAndTypeMapping(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	statusCases := []struct {
		name   string
		domain biz.SummaryEventStatus
		proto  feedium.SummaryEventStatus
	}{
		{
			"pending",
			biz.SummaryEventStatusPending,
			feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_PENDING,
		},
		{
			"processing",
			biz.SummaryEventStatusProcessing,
			feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_PROCESSING,
		},
		{
			"completed",
			biz.SummaryEventStatusCompleted,
			feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_COMPLETED,
		},
		{
			"failed",
			biz.SummaryEventStatusFailed,
			feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_FAILED,
		},
		{
			"expired",
			biz.SummaryEventStatusExpired,
			feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_EXPIRED,
		},
		{
			"unknown falls back to unspecified",
			biz.SummaryEventStatus("bogus"),
			feedium.SummaryEventStatus_SUMMARY_EVENT_STATUS_UNSPECIFIED,
		},
	}

	for _, tc := range statusCases {
		t.Run("status: "+tc.name, func(t *testing.T) {
			// Arrange
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			uc := mock.NewMockUsecase(ctrl)
			uc.EXPECT().GetSummaryEvent(gomock.Any(), "id").Return(&biz.SummaryEvent{
				ID:        "id",
				SourceID:  "s",
				EventType: biz.SummaryEventTypeSummarizeSource,
				Status:    tc.domain,
			}, nil)
			svc := summary.NewService(uc)

			// Act
			resp, err := svc.V1GetSummaryEvent(
				context.Background(),
				&feedium.V1GetSummaryEventRequest{Id: "id"},
			)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tc.proto, resp.GetEvent().GetStatus())
		})
	}

	typeCases := []struct {
		name   string
		domain biz.SummaryEventType
		proto  feedium.SummaryEventType
	}{
		{
			"summarize post",
			biz.SummaryEventTypeSummarizePost,
			feedium.SummaryEventType_SUMMARY_EVENT_TYPE_SUMMARIZE_POST,
		},
		{
			"summarize source",
			biz.SummaryEventTypeSummarizeSource,
			feedium.SummaryEventType_SUMMARY_EVENT_TYPE_SUMMARIZE_SOURCE,
		},
		{
			"unknown falls back to unspecified",
			biz.SummaryEventType("bogus"),
			feedium.SummaryEventType_SUMMARY_EVENT_TYPE_UNSPECIFIED,
		},
	}

	for _, tc := range typeCases {
		t.Run("type: "+tc.name, func(t *testing.T) {
			// Arrange
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			uc := mock.NewMockUsecase(ctrl)
			uc.EXPECT().GetSummaryEvent(gomock.Any(), "id").Return(&biz.SummaryEvent{
				ID:        "id",
				SourceID:  "s",
				EventType: tc.domain,
				Status:    biz.SummaryEventStatusPending,
			}, nil)
			svc := summary.NewService(uc)

			// Act
			resp, err := svc.V1GetSummaryEvent(
				context.Background(),
				&feedium.V1GetSummaryEventRequest{Id: "id"},
			)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tc.proto, resp.GetEvent().GetEventType())
		})
	}
}

func TestV1GetSummary(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	summaryID := "01961d9c-bbbb-7e2e-8c3a-5e7d9a1b2c3d"

	t.Run("found with post id", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().GetSummary(gomock.Any(), summaryID).
			Return(makeSummary(summaryID, "source-1", new("post-1")), nil)
		svc := summary.NewService(uc)

		// Act
		resp, err := svc.V1GetSummary(ctx, &feedium.V1GetSummaryRequest{Id: summaryID})

		// Assert
		require.NoError(t, err)
		got := resp.GetSummary()
		assert.Equal(t, summaryID, got.GetId())
		assert.Equal(t, "source-1", got.GetSourceId())
		assert.Equal(t, "post-1", got.GetPostId())
		assert.Equal(t, "summary text", got.GetText())
		assert.Equal(t, int32(2), got.GetWordCount())
	})

	t.Run("found without post id", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().GetSummary(gomock.Any(), summaryID).
			Return(makeSummary(summaryID, "source-1", nil), nil)
		svc := summary.NewService(uc)

		// Act
		resp, err := svc.V1GetSummary(ctx, &feedium.V1GetSummaryRequest{Id: summaryID})

		// Assert
		require.NoError(t, err)
		assert.Empty(t, resp.GetSummary().GetPostId())
	})

	t.Run("not found", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().GetSummary(gomock.Any(), summaryID).
			Return(biz.Summary{}, biz.ErrSummaryNotFound)
		svc := summary.NewService(uc)

		// Act
		_, err := svc.V1GetSummary(ctx, &feedium.V1GetSummaryRequest{Id: summaryID})

		// Assert
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_SUMMARY_NOT_FOUND")
	})
}

func TestV1ListPostSummaries(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	postID := "01961d9c-cccc-7e2e-8c3a-5e7d9a1b2c3d"

	t.Run("returns mapped summaries", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		items := []biz.Summary{
			makeSummary("s1", "source-1", new(postID)),
			makeSummary("s2", "source-1", new(postID)),
		}
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().ListPostSummaries(gomock.Any(), postID).Return(items, nil)
		svc := summary.NewService(uc)

		// Act
		resp, err := svc.V1ListPostSummaries(
			ctx,
			&feedium.V1ListPostSummariesRequest{PostId: postID},
		)

		// Assert
		require.NoError(t, err)
		require.Len(t, resp.GetSummaries(), 2)
		assert.Equal(t, "s1", resp.GetSummaries()[0].GetId())
		assert.Equal(t, postID, resp.GetSummaries()[0].GetPostId())
		assert.Equal(t, "s2", resp.GetSummaries()[1].GetId())
	})

	t.Run("empty list", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().ListPostSummaries(gomock.Any(), postID).Return(nil, nil)
		svc := summary.NewService(uc)

		// Act
		resp, err := svc.V1ListPostSummaries(
			ctx,
			&feedium.V1ListPostSummariesRequest{PostId: postID},
		)

		// Assert
		require.NoError(t, err)
		assert.Empty(t, resp.GetSummaries())
	})

	t.Run("propagates error", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().ListPostSummaries(gomock.Any(), postID).Return(nil, errors.New("boom"))
		svc := summary.NewService(uc)

		// Act
		_, err := svc.V1ListPostSummaries(ctx, &feedium.V1ListPostSummariesRequest{PostId: postID})

		// Assert
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Internal, st.Code())
	})
}

func TestV1ListSourceSummaries(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	sourceID := "01961d9c-dddd-7e2e-8c3a-5e7d9a1b2c3d"

	t.Run("returns mapped summaries with next page token", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			ListSourceSummaries(gomock.Any(), sourceID, 25, "page-token").
			Return(biz.ListSummariesResult{
				Items: []biz.Summary{
					makeSummary("s1", sourceID, nil),
					makeSummary("s2", sourceID, new("post-1")),
				},
				NextPageToken: "next-token",
			}, nil)
		svc := summary.NewService(uc)

		// Act
		resp, err := svc.V1ListSourceSummaries(ctx, &feedium.V1ListSourceSummariesRequest{
			SourceId:  sourceID,
			PageSize:  25,
			PageToken: "page-token",
		})

		// Assert
		require.NoError(t, err)
		require.Len(t, resp.GetSummaries(), 2)
		assert.Equal(t, "s1", resp.GetSummaries()[0].GetId())
		assert.Empty(t, resp.GetSummaries()[0].GetPostId())
		assert.Equal(t, "post-1", resp.GetSummaries()[1].GetPostId())
		assert.Equal(t, "next-token", resp.GetNextPageToken())
	})

	t.Run("source not found", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().
			ListSourceSummaries(gomock.Any(), sourceID, gomock.Any(), gomock.Any()).
			Return(biz.ListSummariesResult{}, biz.ErrSourceNotFound)
		svc := summary.NewService(uc)

		// Act
		_, err := svc.V1ListSourceSummaries(
			ctx,
			&feedium.V1ListSourceSummariesRequest{SourceId: sourceID},
		)

		// Assert
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
		assert.Contains(t, st.Message(), "ERROR_REASON_SOURCE_NOT_FOUND")
	})
}
