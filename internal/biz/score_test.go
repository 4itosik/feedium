package biz_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

func TestCosine(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	tests := []struct {
		name    string
		a, b    []float32
		want    float64
		wantErr bool
	}{
		{name: "identical vectors", a: []float32{1, 0}, b: []float32{1, 0}, want: 1},
		{name: "orthogonal vectors", a: []float32{1, 0}, b: []float32{0, 1}, want: 0},
		{name: "opposite vectors", a: []float32{1, 0}, b: []float32{-1, 0}, want: -1},
		{name: "zero magnitude yields zero", a: []float32{0, 0}, b: []float32{1, 0}, want: 0},
		{name: "dimension mismatch is error", a: []float32{1, 0}, b: []float32{1}, wantErr: true},
		{name: "empty vector is error", a: []float32{}, b: []float32{1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			a, b := tt.a, tt.b

			// Act
			got, err := biz.Cosine(a, b)

			// Assert
			if tt.wantErr {
				assert.ErrorIs(t, err, biz.ErrScoreValidation)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

func TestRelevance(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	tests := []struct {
		name      string
		post      []float32
		interests [][]float32
		want      float64
		wantErr   bool
	}{
		{
			name:      "empty interest set yields lowest score",
			post:      []float32{1, 0},
			interests: nil,
			want:      0,
		},
		{
			name:      "max cosine over interests",
			post:      []float32{1, 0},
			interests: [][]float32{{0, 1}, {1, 0}},
			want:      1,
		},
		{
			name:      "irrelevant content clamps to zero",
			post:      []float32{1, 0},
			interests: [][]float32{{-1, 0}},
			want:      0,
		},
		{
			name:      "dimension mismatch is permanent error",
			post:      []float32{1, 0},
			interests: [][]float32{{1, 0, 0}},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			post, interests := tt.post, tt.interests

			// Act
			got, err := biz.Relevance(post, interests)

			// Assert
			if tt.wantErr {
				assert.ErrorIs(t, err, biz.ErrScoreValidation)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

func TestIsTransientScoreError(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil is not transient", err: nil, want: false},
		{name: "validation is permanent", err: biz.ErrScoreValidation, want: false},
		{name: "context canceled is permanent", err: context.Canceled, want: false},
		{
			name: "provider error is transient",
			err:  errors.New("503 service unavailable"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			err := tt.err

			// Act
			got := biz.IsTransientScoreError(err)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func newScoreUsecase(
	t *testing.T,
	scoreRepo *mock.MockScoreRepo,
	interestRepo *mock.MockInterestRepo,
	summaryRepo *mock.MockSummaryRepo,
	outboxRepo *mock.MockScoreOutboxRepo,
	scorer *mock.MockScorer,
	postEmbeddingRepo *mock.MockPostEmbeddingRepo,
) *biz.ScoreUsecase {
	t.Helper()
	return biz.NewScoreUsecase(
		scoreRepo,
		interestRepo,
		summaryRepo,
		outboxRepo,
		scorer,
		postEmbeddingRepo,
		&conf.Scoring{},
	)
}

func TestScoreUsecase_ScorePost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("happy path upserts a single score", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		scoreRepo := mock.NewMockScoreRepo(ctrl)
		interestRepo := mock.NewMockInterestRepo(ctrl)
		summaryRepo := mock.NewMockSummaryRepo(ctrl)
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		postEmbeddingRepo := mock.NewMockPostEmbeddingRepo(ctrl)
		summaryRepo.EXPECT().ListByPost(gomock.Any(), "post-1").
			Return([]biz.Summary{{ID: "s-1", Text: "a summary"}}, nil)
		scorer.EXPECT().Model().Return("model-x")
		interestRepo.EXPECT().ListInterestVectors(gomock.Any()).
			Return([]biz.InterestVector{{Embedding: []float32{1, 0}, Model: "model-x"}}, nil)
		postEmbeddingRepo.EXPECT().Get(gomock.Any(), biz.ScoreKey{PostID: new("post-1")}).
			Return(biz.PostEmbedding{}, biz.ErrPostEmbeddingNotFound)
		scorer.EXPECT().Embed(gomock.Any(), "a summary").Return([]float32{1, 0}, nil)
		postEmbeddingRepo.EXPECT().Upsert(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, pe biz.PostEmbedding) (biz.PostEmbedding, error) {
				return pe, nil
			})
		scoreRepo.EXPECT().Upsert(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, s biz.Score) (biz.Score, error) {
				assert.InDelta(t, 1.0, s.Value, 1e-9)
				require.NotNil(t, s.PostID)
				assert.Equal(t, "post-1", *s.PostID)
				s.ID = "score-1"
				return s, nil
			})
		uc := newScoreUsecase(
			t,
			scoreRepo,
			interestRepo,
			summaryRepo,
			outboxRepo,
			scorer,
			postEmbeddingRepo,
		)

		// Act
		id, err := uc.ScorePost(
			context.Background(),
			biz.ScoreEvent{ID: "e-1", PostID: new("post-1")},
		)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "score-1", id)
	})

	t.Run("no interests yields lowest score without embedding", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		scoreRepo := mock.NewMockScoreRepo(ctrl)
		interestRepo := mock.NewMockInterestRepo(ctrl)
		summaryRepo := mock.NewMockSummaryRepo(ctrl)
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		postEmbeddingRepo := mock.NewMockPostEmbeddingRepo(ctrl)
		summaryRepo.EXPECT().ListByPost(gomock.Any(), "post-1").
			Return([]biz.Summary{{ID: "s-1", Text: "a summary"}}, nil)
		scorer.EXPECT().Model().Return("model-x")
		interestRepo.EXPECT().ListInterestVectors(gomock.Any()).Return(nil, nil)
		scoreRepo.EXPECT().Upsert(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, s biz.Score) (biz.Score, error) {
				assert.InDelta(t, 0.0, s.Value, 1e-9)
				s.ID = "score-1"
				return s, nil
			})
		uc := newScoreUsecase(
			t,
			scoreRepo,
			interestRepo,
			summaryRepo,
			outboxRepo,
			scorer,
			postEmbeddingRepo,
		)

		// Act
		id, err := uc.ScorePost(
			context.Background(),
			biz.ScoreEvent{ID: "e-1", PostID: new("post-1")},
		)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "score-1", id)
	})

	t.Run("missing summary is a permanent error", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		scoreRepo := mock.NewMockScoreRepo(ctrl)
		interestRepo := mock.NewMockInterestRepo(ctrl)
		summaryRepo := mock.NewMockSummaryRepo(ctrl)
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		postEmbeddingRepo := mock.NewMockPostEmbeddingRepo(ctrl)
		summaryRepo.EXPECT().ListByPost(gomock.Any(), "post-1").Return(nil, nil)
		uc := newScoreUsecase(
			t,
			scoreRepo,
			interestRepo,
			summaryRepo,
			outboxRepo,
			scorer,
			postEmbeddingRepo,
		)

		// Act
		_, err := uc.ScorePost(
			context.Background(),
			biz.ScoreEvent{ID: "e-1", PostID: new("post-1")},
		)

		// Assert
		require.ErrorIs(t, err, biz.ErrScoreValidation)
		assert.False(t, biz.IsTransientScoreError(err))
	})

	t.Run("transient embedding failure is propagated as transient", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		scoreRepo := mock.NewMockScoreRepo(ctrl)
		interestRepo := mock.NewMockInterestRepo(ctrl)
		summaryRepo := mock.NewMockSummaryRepo(ctrl)
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		postEmbeddingRepo := mock.NewMockPostEmbeddingRepo(ctrl)
		summaryRepo.EXPECT().ListByPost(gomock.Any(), "post-1").
			Return([]biz.Summary{{ID: "s-1", Text: "a summary"}}, nil)
		scorer.EXPECT().Model().Return("model-x")
		interestRepo.EXPECT().ListInterestVectors(gomock.Any()).
			Return([]biz.InterestVector{{Embedding: []float32{1, 0}, Model: "model-x"}}, nil)
		postEmbeddingRepo.EXPECT().Get(gomock.Any(), biz.ScoreKey{PostID: new("post-1")}).
			Return(biz.PostEmbedding{}, biz.ErrPostEmbeddingNotFound)
		scorer.EXPECT().Embed(gomock.Any(), "a summary").Return(nil, errors.New("503 provider"))
		uc := newScoreUsecase(
			t,
			scoreRepo,
			interestRepo,
			summaryRepo,
			outboxRepo,
			scorer,
			postEmbeddingRepo,
		)

		// Act
		_, err := uc.ScorePost(
			context.Background(),
			biz.ScoreEvent{ID: "e-1", PostID: new("post-1")},
		)

		// Assert
		require.Error(t, err)
		assert.True(t, biz.IsTransientScoreError(err))
	})
}

func TestScorePost_VectorReuse(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	const (
		model       = "model-x"
		summaryText = "a summary"
		postID      = "post-1"
	)
	matchingHashSum := sha256.Sum256([]byte(summaryText))
	matchingHash := hex.EncodeToString(matchingHashSum[:])

	tests := []struct {
		name string
		// stored is the embedding returned by PostEmbeddingRepo.Get; getErr its error.
		stored      biz.PostEmbedding
		getErr      error
		embedTimes  int
		upsertTimes int
		// interestModel tags the single interest vector's model.
		interestModel string
		wantValidErr  bool
	}{
		{
			name: "reuse on matching hash and model",
			stored: biz.PostEmbedding{
				PostID: new(postID), Vector: []float32{1, 0}, Model: model, InputHash: matchingHash,
			},
			embedTimes:    0,
			upsertTimes:   0,
			interestModel: model,
		},
		{
			name: "recompute on input hash change",
			stored: biz.PostEmbedding{
				PostID: new(postID), Vector: []float32{0, 1}, Model: model, InputHash: "stale-hash",
			},
			embedTimes:    1,
			upsertTimes:   1,
			interestModel: model,
		},
		{
			name: "recompute on post model change",
			stored: biz.PostEmbedding{
				PostID: new(postID), Vector: []float32{0, 1}, Model: "old", InputHash: matchingHash,
			},
			embedTimes:    1,
			upsertTimes:   1,
			interestModel: model,
		},
		{
			name:          "recompute when no stored vector",
			getErr:        biz.ErrPostEmbeddingNotFound,
			embedTimes:    1,
			upsertTimes:   1,
			interestModel: model,
		},
		{
			name:          "interest model mismatch fails terminally before embed",
			interestModel: "old",
			embedTimes:    0,
			upsertTimes:   0,
			wantValidErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			scoreRepo := mock.NewMockScoreRepo(ctrl)
			interestRepo := mock.NewMockInterestRepo(ctrl)
			summaryRepo := mock.NewMockSummaryRepo(ctrl)
			outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
			scorer := mock.NewMockScorer(ctrl)
			postEmbeddingRepo := mock.NewMockPostEmbeddingRepo(ctrl)

			summaryRepo.EXPECT().ListByPost(gomock.Any(), postID).
				Return([]biz.Summary{{ID: "s-1", Text: summaryText}}, nil)
			scorer.EXPECT().Model().Return(model)
			interestRepo.EXPECT().ListInterestVectors(gomock.Any()).
				Return([]biz.InterestVector{{Embedding: []float32{1, 0}, Model: tt.interestModel}}, nil)

			if !tt.wantValidErr {
				postEmbeddingRepo.EXPECT().
					Get(gomock.Any(), biz.ScoreKey{PostID: new(postID)}).
					Return(tt.stored, tt.getErr)
				scorer.EXPECT().Embed(gomock.Any(), summaryText).
					Return([]float32{1, 0}, nil).Times(tt.embedTimes)
				postEmbeddingRepo.EXPECT().Upsert(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, pe biz.PostEmbedding) (biz.PostEmbedding, error) {
						assert.Equal(t, matchingHash, pe.InputHash)
						assert.Equal(t, model, pe.Model)
						require.NotNil(t, pe.PostID)
						assert.Equal(t, postID, *pe.PostID)
						return pe, nil
					}).
					Times(tt.upsertTimes)
				scoreRepo.EXPECT().Upsert(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, s biz.Score) (biz.Score, error) {
						assert.InDelta(t, 1.0, s.Value, 1e-9)
						s.ID = "score-1"
						return s, nil
					})
			}

			uc := newScoreUsecase(
				t,
				scoreRepo,
				interestRepo,
				summaryRepo,
				outboxRepo,
				scorer,
				postEmbeddingRepo,
			)

			// Act
			_, err := uc.ScorePost(
				context.Background(),
				biz.ScoreEvent{ID: "e-1", PostID: new(postID)},
			)

			// Assert
			if tt.wantValidErr {
				require.ErrorIs(t, err, biz.ErrScoreValidation)
				return
			}
			require.NoError(t, err)
		})
	}

	t.Run("generic Get error is propagated without embed or upsert", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		scoreRepo := mock.NewMockScoreRepo(ctrl)
		interestRepo := mock.NewMockInterestRepo(ctrl)
		summaryRepo := mock.NewMockSummaryRepo(ctrl)
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		scorer := mock.NewMockScorer(ctrl)
		postEmbeddingRepo := mock.NewMockPostEmbeddingRepo(ctrl)

		summaryRepo.EXPECT().ListByPost(gomock.Any(), postID).
			Return([]biz.Summary{{ID: "s-1", Text: summaryText}}, nil)
		scorer.EXPECT().Model().Return(model)
		interestRepo.EXPECT().ListInterestVectors(gomock.Any()).
			Return([]biz.InterestVector{{Embedding: []float32{1, 0}, Model: model}}, nil)
		postEmbeddingRepo.EXPECT().Get(gomock.Any(), biz.ScoreKey{PostID: new(postID)}).
			Return(biz.PostEmbedding{}, errors.New("db connection refused"))
		scorer.EXPECT().Embed(gomock.Any(), gomock.Any()).Times(0)
		postEmbeddingRepo.EXPECT().Upsert(gomock.Any(), gomock.Any()).Times(0)

		uc := newScoreUsecase(
			t,
			scoreRepo,
			interestRepo,
			summaryRepo,
			outboxRepo,
			scorer,
			postEmbeddingRepo,
		)

		// Act
		_, err := uc.ScorePost(context.Background(), biz.ScoreEvent{ID: "e-1", PostID: new(postID)})

		// Assert
		require.Error(t, err)
		require.ErrorContains(t, err, "get embedding")
		assert.NotErrorIs(t, err, biz.ErrScoreValidation)
	})
}

func TestScoreUsecase_RetryPolicy(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("policy reads worker config", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cfg := &conf.Scoring{Worker: &conf.ScoringWorker{
			MaxAttempts: 3,
			BackoffBase: durationpb.New(2 * time.Second),
			BackoffMax:  durationpb.New(time.Minute),
		}}
		uc := biz.NewScoreUsecase(
			mock.NewMockScoreRepo(ctrl), mock.NewMockInterestRepo(ctrl),
			mock.NewMockSummaryRepo(ctrl), mock.NewMockScoreOutboxRepo(ctrl),
			mock.NewMockScorer(ctrl), mock.NewMockPostEmbeddingRepo(ctrl), cfg,
		)

		// Act
		policy := uc.RetryPolicy()

		// Assert
		assert.Equal(t, 3, policy.MaxAttempts)
		assert.Equal(t, 2*time.Second, policy.BackoffBase)
	})

	t.Run("empty config falls back to defaults", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := biz.NewScoreUsecase(
			mock.NewMockScoreRepo(ctrl), mock.NewMockInterestRepo(ctrl),
			mock.NewMockSummaryRepo(ctrl), mock.NewMockScoreOutboxRepo(ctrl),
			mock.NewMockScorer(ctrl), mock.NewMockPostEmbeddingRepo(ctrl), &conf.Scoring{},
		)

		// Act
		policy := uc.RetryPolicy()

		// Assert
		assert.Positive(t, policy.MaxAttempts)
		assert.Positive(t, policy.BackoffBase)
		assert.Positive(t, policy.BackoffMax)
	})
}

func TestScoreUsecase_EnqueueDue_Error(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("outbox error is wrapped", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		outboxRepo.EXPECT().EnqueueDuePosts(gomock.Any()).Return(0, errors.New("db down"))
		uc := biz.NewScoreUsecase(
			mock.NewMockScoreRepo(ctrl), mock.NewMockInterestRepo(ctrl),
			mock.NewMockSummaryRepo(ctrl), outboxRepo, mock.NewMockScorer(ctrl),
			mock.NewMockPostEmbeddingRepo(ctrl), &conf.Scoring{},
		)

		// Act
		_, err := uc.EnqueueDue(context.Background())

		// Assert
		require.Error(t, err)
	})
}

func TestScoreUsecase_ReapStuckScoreEvents_ListError(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("list error is propagated", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
		outboxRepo.EXPECT().ListLeaseExpired(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("db down"))
		uc := biz.NewScoreUsecase(
			mock.NewMockScoreRepo(ctrl), mock.NewMockInterestRepo(ctrl),
			mock.NewMockSummaryRepo(ctrl), outboxRepo, mock.NewMockScorer(ctrl),
			mock.NewMockPostEmbeddingRepo(ctrl), &conf.Scoring{},
		)

		// Act
		_, err := uc.ReapStuckScoreEvents(context.Background())

		// Assert
		require.Error(t, err)
	})
}
