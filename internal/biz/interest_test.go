package biz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/biz/mock"
)

func TestInterestUsecase_Create(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("valid topic is embedded and saved", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockInterestRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		scorer.EXPECT().Embed(gomock.Any(), "go programming").Return([]float32{1, 0}, nil)
		scorer.EXPECT().Model().Return("text-embedding-3-small")
		repo.EXPECT().Save(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, in biz.Interest) (biz.Interest, error) {
				return in, nil
			})
		uc := biz.NewInterestUsecase(repo, scorer)

		// Act
		got, err := uc.Create(context.Background(), "go programming")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "go programming", got.Topic)
		assert.Equal(t, []float32{1, 0}, got.Embedding)
		assert.Equal(t, "text-embedding-3-small", got.Model)
		assert.NotEmpty(t, got.ID)
	})

	t.Run("empty topic is rejected before embedding", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockInterestRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		uc := biz.NewInterestUsecase(repo, scorer)

		// Act
		_, err := uc.Create(context.Background(), "   ")

		// Assert
		assert.ErrorIs(t, err, biz.ErrInterestValidation)
	})

	t.Run("embedding failure is propagated", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockInterestRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		scorer.EXPECT().Embed(gomock.Any(), "ai").Return(nil, errors.New("provider down"))
		uc := biz.NewInterestUsecase(repo, scorer)

		// Act
		_, err := uc.Create(context.Background(), "ai")

		// Assert
		require.Error(t, err)
	})
}

func TestInterestUsecase_Update(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("topic is re-embedded and updated in place", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockInterestRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		repo.EXPECT().Get(gomock.Any(), "id-1").Return(biz.Interest{ID: "id-1", Topic: "old"}, nil)
		scorer.EXPECT().Embed(gomock.Any(), "new topic").Return([]float32{0, 1}, nil)
		scorer.EXPECT().Model().Return("model-x")
		repo.EXPECT().Update(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, in biz.Interest) (biz.Interest, error) {
				return in, nil
			})
		uc := biz.NewInterestUsecase(repo, scorer)

		// Act
		got, err := uc.Update(context.Background(), "id-1", "new topic")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "new topic", got.Topic)
		assert.Equal(t, []float32{0, 1}, got.Embedding)
	})

	t.Run("missing interest is propagated", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockInterestRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		repo.EXPECT().Get(gomock.Any(), "missing").Return(biz.Interest{}, biz.ErrInterestNotFound)
		uc := biz.NewInterestUsecase(repo, scorer)

		// Act
		_, err := uc.Update(context.Background(), "missing", "topic")

		// Assert
		assert.ErrorIs(t, err, biz.ErrInterestNotFound)
	})
}

func TestInterestUsecase_List(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("page size is clamped to the minimum", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockInterestRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		repo.EXPECT().List(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, f biz.ListInterestsFilter) (biz.ListInterestsResult, error) {
				assert.Equal(t, 1, f.PageSize)
				return biz.ListInterestsResult{}, nil
			})
		uc := biz.NewInterestUsecase(repo, scorer)

		// Act
		_, err := uc.List(context.Background(), biz.ListInterestsFilter{PageSize: 0})

		// Assert
		require.NoError(t, err)
	})
}

func TestInterestUsecase_Get(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("get delegates to repo", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockInterestRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		repo.EXPECT().Get(gomock.Any(), "id-1").Return(biz.Interest{ID: "id-1", Topic: "ai"}, nil)
		uc := biz.NewInterestUsecase(repo, scorer)

		// Act
		got, err := uc.Get(context.Background(), "id-1")

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "ai", got.Topic)
	})
}

func TestInterestUsecase_Delete(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("delete delegates to repo", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		repo := mock.NewMockInterestRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		repo.EXPECT().Delete(gomock.Any(), "id-1").Return(nil)
		uc := biz.NewInterestUsecase(repo, scorer)

		// Act
		err := uc.Delete(context.Background(), "id-1")

		// Assert
		require.NoError(t, err)
	})
}
