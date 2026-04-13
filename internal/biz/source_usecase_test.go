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
)

func TestSourceUsecase_Create(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("valid RSS source", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			Save(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, source biz.Source) (biz.Source, error) {
				source.ID = "test-id"
				source.CreatedAt = time.Now()
				source.UpdatedAt = time.Now()
				return source, nil
			})

		uc := biz.NewSourceUsecase(mockRepo)
		cfg := &biz.RSSConfig{FeedURL: "https://example.com/feed"}
		result, err := uc.Create(context.Background(), biz.SourceTypeRSS, cfg)

		require.NoError(t, err)
		assert.Equal(t, biz.SourceTypeRSS, result.Type)
		assert.Equal(t, biz.ProcessingModeSelfContained, result.ProcessingMode)
		assert.NotEmpty(t, result.ID)
	})

	t.Run("invalid source type", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)

		uc := biz.NewSourceUsecase(mockRepo)
		_, err := uc.Create(context.Background(), "invalid", &biz.RSSConfig{FeedURL: "https://example.com/feed"})

		assert.ErrorIs(t, err, biz.ErrInvalidSourceType)
	})

	t.Run("invalid config", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)

		uc := biz.NewSourceUsecase(mockRepo)
		_, err := uc.Create(context.Background(), biz.SourceTypeRSS, &biz.RSSConfig{FeedURL: ""})

		assert.ErrorIs(t, err, biz.ErrInvalidConfig)
	})
}

func TestSourceUsecase_Update(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("valid update", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		existingSource := biz.Source{
			ID:             "test-id",
			Type:           biz.SourceTypeRSS,
			ProcessingMode: biz.ProcessingModeSelfContained,
			Config:         &biz.RSSConfig{FeedURL: "https://old.example.com/feed"},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		mockRepo := mock.NewMockSourceRepo(ctrl)
		gomock.InOrder(
			mockRepo.EXPECT().
				Get(gomock.Any(), "test-id").
				Return(existingSource, nil),
			mockRepo.EXPECT().
				Update(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, source biz.Source) (biz.Source, error) {
					source.UpdatedAt = time.Now()
					return source, nil
				}),
		)

		uc := biz.NewSourceUsecase(mockRepo)
		result, err := uc.Update(
			context.Background(),
			"test-id",
			biz.SourceTypeRSS,
			&biz.RSSConfig{FeedURL: "https://new.example.com/feed"},
		)

		require.NoError(t, err)
		assert.Equal(t, "test-id", result.ID)
	})

	t.Run("type immutable", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		existingSource := biz.Source{
			ID:     "test-id",
			Type:   biz.SourceTypeRSS,
			Config: &biz.RSSConfig{FeedURL: "https://example.com/feed"},
		}

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			Get(gomock.Any(), "test-id").
			Return(existingSource, nil)

		uc := biz.NewSourceUsecase(mockRepo)
		htmlCfg := &biz.HTMLConfig{URL: "https://example.com"}
		_, err := uc.Update(context.Background(), "test-id", biz.SourceTypeHTML, htmlCfg)

		assert.ErrorIs(t, err, biz.ErrTypeImmutable)
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			Get(gomock.Any(), "nonexistent").
			Return(biz.Source{}, biz.ErrSourceNotFound)

		uc := biz.NewSourceUsecase(mockRepo)
		_, err := uc.Update(
			context.Background(),
			"nonexistent",
			biz.SourceTypeRSS,
			&biz.RSSConfig{FeedURL: "https://example.com/feed"},
		)

		assert.ErrorIs(t, err, biz.ErrSourceNotFound)
	})

	t.Run("invalid config", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		existingSource := biz.Source{
			ID:     "test-id",
			Type:   biz.SourceTypeRSS,
			Config: &biz.RSSConfig{FeedURL: "https://example.com/feed"},
		}

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			Get(gomock.Any(), "test-id").
			Return(existingSource, nil)

		uc := biz.NewSourceUsecase(mockRepo)
		_, err := uc.Update(context.Background(), "test-id", biz.SourceTypeRSS, &biz.RSSConfig{FeedURL: ""})

		assert.ErrorIs(t, err, biz.ErrInvalidConfig)
	})
}

func TestSourceUsecase_Delete(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("successful delete", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			Delete(gomock.Any(), "test-id").
			Return(nil)

		uc := biz.NewSourceUsecase(mockRepo)
		err := uc.Delete(context.Background(), "test-id")

		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			Delete(gomock.Any(), "nonexistent").
			Return(biz.ErrSourceNotFound)

		uc := biz.NewSourceUsecase(mockRepo)
		err := uc.Delete(context.Background(), "nonexistent")

		assert.ErrorIs(t, err, biz.ErrSourceNotFound)
	})
}

func TestSourceUsecase_Get(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expectedSource := biz.Source{
			ID:     "test-id",
			Type:   biz.SourceTypeRSS,
			Config: &biz.RSSConfig{FeedURL: "https://example.com/feed"},
		}

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			Get(gomock.Any(), "test-id").
			Return(expectedSource, nil)

		uc := biz.NewSourceUsecase(mockRepo)
		result, err := uc.Get(context.Background(), "test-id")

		require.NoError(t, err)
		assert.Equal(t, expectedSource.ID, result.ID)
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			Get(gomock.Any(), "nonexistent").
			Return(biz.Source{}, biz.ErrSourceNotFound)

		uc := biz.NewSourceUsecase(mockRepo)
		_, err := uc.Get(context.Background(), "nonexistent")

		assert.ErrorIs(t, err, biz.ErrSourceNotFound)
	})
}

func TestSourceUsecase_List(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("happy path with pagination", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sources := []biz.Source{
			{ID: "id1", Type: biz.SourceTypeRSS, Config: &biz.RSSConfig{FeedURL: "url1"}},
			{ID: "id2", Type: biz.SourceTypeRSS, Config: &biz.RSSConfig{FeedURL: "url2"}},
			{ID: "id3", Type: biz.SourceTypeRSS, Config: &biz.RSSConfig{FeedURL: "url3"}},
		}

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(biz.ListSourcesResult{
				Items:         sources[:2],
				NextPageToken: "token",
			}, nil)

		uc := biz.NewSourceUsecase(mockRepo)
		result, err := uc.List(context.Background(), biz.ListSourcesFilter{PageSize: 2})

		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		assert.Equal(t, "token", result.NextPageToken)
	})

	t.Run("empty list", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(biz.ListSourcesResult{Items: []biz.Source{}, NextPageToken: ""}, nil)

		uc := biz.NewSourceUsecase(mockRepo)
		result, err := uc.List(context.Background(), biz.ListSourcesFilter{})

		require.NoError(t, err)
		assert.Empty(t, result.Items)
		assert.Empty(t, result.NextPageToken)
	})

	t.Run("page_size < 1 clamped to 1", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(biz.ListSourcesResult{}, nil)

		uc := biz.NewSourceUsecase(mockRepo)
		_, _ = uc.List(context.Background(), biz.ListSourcesFilter{PageSize: 0})
	})

	t.Run("page_size > 500 clamped to 500", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)
		mockRepo.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(biz.ListSourcesResult{}, nil)

		uc := biz.NewSourceUsecase(mockRepo)
		_, _ = uc.List(context.Background(), biz.ListSourcesFilter{PageSize: 1000})
	})

	t.Run("invalid type filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := mock.NewMockSourceRepo(ctrl)

		uc := biz.NewSourceUsecase(mockRepo)
		_, err := uc.List(context.Background(), biz.ListSourcesFilter{Type: "invalid"})

		assert.ErrorIs(t, err, biz.ErrInvalidSourceType)
	})
}
