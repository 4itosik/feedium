package biz_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/biz/mock"
	"github.com/4itosik/feedium/internal/conf"
)

func TestFeedUsecase_RankedFeed(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("valid query is delegated with clamped page size", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockFeedQueryRepo(ctrl)
		repo.EXPECT().ListRanked(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, q biz.FeedQuery) (biz.FeedResult, error) {
				assert.Equal(t, 1, q.PageSize)
				return biz.FeedResult{Items: []biz.FeedItem{{Score: 0.9}}}, nil
			})
		uc := biz.NewFeedUsecase(repo, &conf.Scoring{})

		// Act
		got, err := uc.RankedFeed(context.Background(), biz.FeedQuery{MinScore: 0.5, PageSize: 0})

		// Assert
		require.NoError(t, err)
		require.Len(t, got.Items, 1)
	})

	t.Run("threshold below zero is rejected", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockFeedQueryRepo(ctrl)
		uc := biz.NewFeedUsecase(repo, &conf.Scoring{})

		// Act
		_, err := uc.RankedFeed(context.Background(), biz.FeedQuery{MinScore: -0.1})

		// Assert
		assert.ErrorIs(t, err, biz.ErrInvalidThreshold)
	})

	t.Run("threshold above one is rejected", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockFeedQueryRepo(ctrl)
		uc := biz.NewFeedUsecase(repo, &conf.Scoring{})

		// Act
		_, err := uc.RankedFeed(context.Background(), biz.FeedQuery{MinScore: 1.5})

		// Assert
		assert.ErrorIs(t, err, biz.ErrInvalidThreshold)
	})

	t.Run("inverted time range is rejected", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockFeedQueryRepo(ctrl)
		uc := biz.NewFeedUsecase(repo, &conf.Scoring{})
		from := time.Now()
		to := from.Add(-time.Hour)

		// Act
		_, err := uc.RankedFeed(context.Background(), biz.FeedQuery{From: &from, To: &to})

		// Assert
		assert.ErrorIs(t, err, biz.ErrInvalidThreshold)
	})
}
