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

func makePost(id, sourceID, externalID, text string) biz.Post {
	return biz.Post{
		ID:          id,
		SourceID:    sourceID,
		ExternalID:  externalID,
		Text:        text,
		Author:      nil,
		Metadata:    map[string]string{},
		Source:      biz.SourceInfo{ID: sourceID, Type: biz.SourceTypeRSS},
		PublishedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		CreatedAt:   time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
	}
}

func makeRSSSource(id string) biz.Source {
	return biz.Source{
		ID:             id,
		Type:           biz.SourceTypeRSS,
		ProcessingMode: biz.ProcessingModeSelfContained,
		Config:         &biz.RSSConfig{FeedURL: "https://example.com/feed"},
		CreatedAt:      time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
	}
}

func setupCreateMocks(
	t *testing.T,
	ctrl *gomock.Controller,
) (*mock.MockTxManager, *mock.MockSummaryOutboxRepo, *mock.MockSourceRepo) {
	t.Helper()
	txManager := mock.NewMockTxManager(ctrl)
	outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
	sourceRepo := mock.NewMockSourceRepo(ctrl)
	return txManager, outboxRepo, sourceRepo
}

func expectInTx(txManager *mock.MockTxManager) {
	txManager.EXPECT().
		InTx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, fn func(context.Context) error) error {
			return fn(context.Background())
		})
}

func TestPostUsecase_Create(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	publishedAt := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	t.Run("valid fields", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager, outboxRepo, sourceRepo := setupCreateMocks(t, ctrl)

		sourceRepo.EXPECT().Get(gomock.Any(), sourceID).Return(makeRSSSource(sourceID), nil)

		expectInTx(txManager)

		repo.EXPECT().
			Save(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, p biz.Post) (biz.Post, bool, error) {
				assert.NotEmpty(t, p.ID)
				assert.Equal(t, sourceID, p.SourceID)
				assert.Equal(t, "ext-1", p.ExternalID)
				return p, true, nil
			})

		outboxRepo.EXPECT().
			Save(gomock.Any(), gomock.Any()).
			Return(biz.SummaryEvent{}, nil)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		result, err := uc.Create(context.Background(), sourceID, "ext-1", "text", publishedAt, nil, nil)

		require.NoError(t, err)
		assert.NotEmpty(t, result.ID)
		assert.Equal(t, sourceID, result.SourceID)
	})

	t.Run("invalid fields", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager, outboxRepo, sourceRepo := setupCreateMocks(t, ctrl)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)

		_, err := uc.Create(context.Background(), "", "ext-1", "text", publishedAt, nil, nil)
		assert.ErrorIs(t, err, biz.ErrPostInvalidArgument)
	})

	t.Run("idempotent upsert", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		first := makePost("post-1", sourceID, "ext-1", "text")

		repo := mock.NewMockPostRepo(ctrl)
		txManager, outboxRepo, sourceRepo := setupCreateMocks(t, ctrl)

		sourceRepo.EXPECT().Get(gomock.Any(), sourceID).Return(makeRSSSource(sourceID), nil)

		expectInTx(txManager)

		repo.EXPECT().
			Save(gomock.Any(), gomock.Any()).
			Return(first, false, nil)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		result, err := uc.Create(context.Background(), sourceID, "ext-1", "text", publishedAt, nil, nil)

		require.NoError(t, err)
		assert.Equal(t, "post-1", result.ID)
	})

	t.Run("nil author is allowed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager, outboxRepo, sourceRepo := setupCreateMocks(t, ctrl)

		sourceRepo.EXPECT().Get(gomock.Any(), sourceID).Return(makeRSSSource(sourceID), nil)

		expectInTx(txManager)

		repo.EXPECT().
			Save(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, p biz.Post) (biz.Post, bool, error) {
				assert.Nil(t, p.Author)
				return p, true, nil
			})

		outboxRepo.EXPECT().
			Save(gomock.Any(), gomock.Any()).
			Return(biz.SummaryEvent{}, nil)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		result, err := uc.Create(context.Background(), sourceID, "ext-1", "text", publishedAt, nil, nil)

		require.NoError(t, err)
		assert.Nil(t, result.Author)
	})
}

func TestPostUsecase_Update(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	postID := "01961d9c-aaaa-7e2e-8c3a-5e7d9a1b2c3d"
	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	publishedAt := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)

	t.Run("valid update", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		existing := makePost(postID, sourceID, "ext-1", "old text")

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		gomock.InOrder(
			repo.EXPECT().Get(gomock.Any(), postID).Return(existing, nil),
			repo.EXPECT().
				Update(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, p biz.Post) (biz.Post, error) {
					assert.Equal(t, "new text", p.Text)
					assert.Equal(t, "ext-1", p.ExternalID)
					assert.True(t, p.UpdatedAt.After(existing.UpdatedAt) || p.UpdatedAt.Equal(existing.UpdatedAt))
					return p, nil
				}),
		)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		result, err := uc.Update(
			context.Background(),
			postID,
			"ext-1",
			"new text",
			publishedAt,
			nil,
			map[string]string{},
		)

		require.NoError(t, err)
		assert.Equal(t, "new text", result.Text)
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		repo.EXPECT().Get(gomock.Any(), postID).Return(biz.Post{}, biz.ErrPostNotFound)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		_, err := uc.Update(context.Background(), postID, "ext-1", "text", publishedAt, nil, nil)

		assert.ErrorIs(t, err, biz.ErrPostNotFound)
	})

	t.Run("conflict external_id", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		existing := makePost(postID, sourceID, "ext-1", "text")

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		gomock.InOrder(
			repo.EXPECT().Get(gomock.Any(), postID).Return(existing, nil),
			repo.EXPECT().
				Update(gomock.Any(), gomock.Any()).
				Return(biz.Post{}, biz.ErrPostAlreadyExists),
		)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		_, err := uc.Update(context.Background(), postID, "ext-1", "text", publishedAt, nil, nil)

		assert.ErrorIs(t, err, biz.ErrPostAlreadyExists)
	})

	t.Run("empty text validation error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)

		_, err := uc.Update(context.Background(), postID, "ext-1", "", publishedAt, nil, nil)
		assert.ErrorIs(t, err, biz.ErrPostInvalidArgument)
	})
}

func TestPostUsecase_Delete(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("valid", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		repo.EXPECT().Delete(gomock.Any(), "post-1").Return(nil)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		err := uc.Delete(context.Background(), "post-1")

		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		repo.EXPECT().Delete(gomock.Any(), "nonexistent").Return(biz.ErrPostNotFound)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		err := uc.Delete(context.Background(), "nonexistent")

		assert.ErrorIs(t, err, biz.ErrPostNotFound)
	})
}

func TestPostUsecase_Get(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		expected := makePost("post-1", "source-1", "ext-1", "text")

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		repo.EXPECT().Get(gomock.Any(), "post-1").Return(expected, nil)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		result, err := uc.Get(context.Background(), "post-1")

		require.NoError(t, err)
		assert.Equal(t, "post-1", result.ID)
		assert.Equal(t, biz.SourceInfo{ID: "source-1", Type: biz.SourceTypeRSS}, result.Source)
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		repo.EXPECT().Get(gomock.Any(), "nonexistent").Return(biz.Post{}, biz.ErrPostNotFound)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		_, err := uc.Get(context.Background(), "nonexistent")

		assert.ErrorIs(t, err, biz.ErrPostNotFound)
	})
}

func TestPostUsecase_List(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("page_size < 1 clamped to 1", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		repo.EXPECT().
			List(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, f biz.ListPostsFilter) (biz.ListPostsResult, error) {
				assert.Equal(t, 1, f.PageSize)
				return biz.ListPostsResult{}, nil
			})

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		_, _ = uc.List(context.Background(), biz.ListPostsFilter{
			PageSize: 0,
			OrderBy:  biz.SortByPublishedAt,
			OrderDir: biz.SortDesc,
		})
	})

	t.Run("page_size > 500 clamped to 500", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		repo.EXPECT().
			List(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, f biz.ListPostsFilter) (biz.ListPostsResult, error) {
				assert.Equal(t, 500, f.PageSize)
				return biz.ListPostsResult{}, nil
			})

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		_, _ = uc.List(context.Background(), biz.ListPostsFilter{
			PageSize: 1000,
			OrderBy:  biz.SortByPublishedAt,
			OrderDir: biz.SortDesc,
		})
	})

	t.Run("invalid OrderBy", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)

		_, err := uc.List(context.Background(), biz.ListPostsFilter{
			PageSize: 10,
			OrderBy:  biz.SortField(0),
			OrderDir: biz.SortDesc,
		})
		assert.ErrorIs(t, err, biz.ErrPostInvalidArgument)
	})

	t.Run("invalid OrderDir", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)

		_, err := uc.List(context.Background(), biz.ListPostsFilter{
			PageSize: 10,
			OrderBy:  biz.SortByPublishedAt,
			OrderDir: biz.SortDirection(0),
		})
		assert.ErrorIs(t, err, biz.ErrPostInvalidArgument)
	})

	t.Run("valid filter", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		posts := []biz.Post{
			makePost("p1", "s1", "e1", "t1"),
			makePost("p2", "s1", "e2", "t2"),
		}

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		repo.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(biz.ListPostsResult{Items: posts, NextPageToken: "token"}, nil)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		result, err := uc.List(context.Background(), biz.ListPostsFilter{
			PageSize: 10,
			OrderBy:  biz.SortByPublishedAt,
			OrderDir: biz.SortDesc,
		})

		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		assert.Equal(t, "token", result.NextPageToken)
	})

	t.Run("clamping happens before validation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		repo := mock.NewMockPostRepo(ctrl)
		txManager := mock.NewMockTxManager(ctrl)
		outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
		sourceRepo := mock.NewMockSourceRepo(ctrl)

		repo.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(biz.ListPostsResult{}, nil)

		uc := biz.NewPostUsecase(repo, txManager, outboxRepo, sourceRepo)
		_, _ = uc.List(context.Background(), biz.ListPostsFilter{
			PageSize: 0,
			OrderBy:  biz.SortByCreatedAt,
			OrderDir: biz.SortAsc,
		})
	})
}
