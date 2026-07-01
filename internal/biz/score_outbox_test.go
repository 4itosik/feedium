package biz_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/biz/mock"
	"github.com/4itosik/feedium/internal/conf"
)

func TestScoreUsecase_EnqueueDue(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("delegates to posts and digests and returns the summed count", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		outboxRepo.EXPECT().EnqueueDuePosts(gomock.Any()).Return(3, nil)
		// FT-020 REQ-01: digests are enqueued on the same tick; the count sums both.
		outboxRepo.EXPECT().EnqueueDueDigests(gomock.Any()).Return(2, nil)
		uc := biz.NewScoreUsecase(
			mock.NewMockScoreRepo(ctrl), mock.NewMockInterestRepo(ctrl),
			mock.NewMockSummaryRepo(ctrl), outboxRepo, mock.NewMockScorer(ctrl),
			mock.NewMockPostEmbeddingRepo(ctrl), &conf.Scoring{},
		)

		// Act
		n, err := uc.EnqueueDue(context.Background())

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 5, n)
	})
}

func TestScoreUsecase_ReapStuckScoreEvents(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	cfg := &conf.Scoring{
		Worker: &conf.ScoringWorker{MaxAttempts: 5},
		Reaper: &conf.ScoringReaper{Grace: durationpb.New(30 * time.Second)},
	}

	t.Run("under-budget events are skipped, at-budget are terminated", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		outboxRepo.EXPECT().ListLeaseExpired(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]biz.ScoreEvent{
				{ID: "young", AttemptCount: 2},
				{ID: "exhausted", AttemptCount: 5},
			}, nil)
		outboxRepo.EXPECT().
			FailExpired(gomock.Any(), "exhausted", 5, gomock.Any(), gomock.Any()).
			Return(true, nil)
		uc := biz.NewScoreUsecase(
			mock.NewMockScoreRepo(ctrl), mock.NewMockInterestRepo(ctrl),
			mock.NewMockSummaryRepo(ctrl), outboxRepo, mock.NewMockScorer(ctrl),
			mock.NewMockPostEmbeddingRepo(ctrl), cfg,
		)

		// Act
		terminated, err := uc.ReapStuckScoreEvents(context.Background())

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 1, terminated)
	})

	t.Run("a lost race does not count as terminated", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		outboxRepo.EXPECT().ListLeaseExpired(gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]biz.ScoreEvent{{ID: "exhausted", AttemptCount: 5}}, nil)
		outboxRepo.EXPECT().
			FailExpired(gomock.Any(), "exhausted", 5, gomock.Any(), gomock.Any()).
			Return(false, nil)
		uc := biz.NewScoreUsecase(
			mock.NewMockScoreRepo(ctrl), mock.NewMockInterestRepo(ctrl),
			mock.NewMockSummaryRepo(ctrl), outboxRepo, mock.NewMockScorer(ctrl),
			mock.NewMockPostEmbeddingRepo(ctrl), cfg,
		)

		// Act
		terminated, err := uc.ReapStuckScoreEvents(context.Background())

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 0, terminated)
	})
}
