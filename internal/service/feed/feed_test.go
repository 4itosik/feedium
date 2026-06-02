package feed_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/service/feed"
	"github.com/4itosik/feedium/internal/service/feed/mock"
)

func TestFeedService_V1ListFeed(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("maps query and ranked items to proto", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		author := "alice"
		uc.EXPECT().RankedFeed(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, q biz.FeedQuery) (biz.FeedResult, error) {
				assert.InDelta(t, 0.5, q.MinScore, 1e-9)
				return biz.FeedResult{Items: []biz.FeedItem{
					{Post: biz.Post{ID: "p-1", SourceID: "s-1", Author: &author, Text: "t"}, Score: 0.9},
				}}, nil
			})
		svc := feed.NewFeedService(uc)

		// Act
		resp, err := svc.V1ListFeed(context.Background(), &feedium.V1ListFeedRequest{MinScore: 0.5})

		// Assert
		require.NoError(t, err)
		require.Len(t, resp.GetItems(), 1)
		assert.Equal(t, "p-1", resp.GetItems()[0].GetPostId())
		assert.Equal(t, "alice", resp.GetItems()[0].GetAuthor())
		assert.InDelta(t, 0.9, resp.GetItems()[0].GetScore(), 1e-9)
	})

	t.Run("invalid threshold maps to InvalidArgument", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().RankedFeed(gomock.Any(), gomock.Any()).
			Return(biz.FeedResult{}, biz.ErrInvalidThreshold)
		svc := feed.NewFeedService(uc)

		// Act
		_, err := svc.V1ListFeed(context.Background(), &feedium.V1ListFeedRequest{MinScore: 2})

		// Assert
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("unexpected error maps to Internal", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().RankedFeed(gomock.Any(), gomock.Any()).
			Return(biz.FeedResult{}, errors.New("db down"))
		svc := feed.NewFeedService(uc)

		// Act
		_, err := svc.V1ListFeed(context.Background(), &feedium.V1ListFeedRequest{})

		// Assert
		assert.Equal(t, codes.Internal, status.Code(err))
	})
}
