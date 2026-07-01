package biz_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/biz/mock"
)

// TestScorePost_DigestBranch covers SC-01/EC-01/REQ-01..03: a digest score event
// (SummaryID set) reads the summary text directly via SummaryRepo.Get (not ListByPost),
// scores it with the same cosine/relevance path, and upserts a score keyed by
// summary_id with post_id nil.
func TestScorePost_DigestBranch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	scoreRepo := mock.NewMockScoreRepo(ctrl)
	interestRepo := mock.NewMockInterestRepo(ctrl)
	summaryRepo := mock.NewMockSummaryRepo(ctrl)
	outboxRepo := mock.NewMockScoreOutboxRepo(ctrl)
	scorer := mock.NewMockScorer(ctrl)
	postEmbeddingRepo := mock.NewMockPostEmbeddingRepo(ctrl)

	// Digest text is read directly by summary id — NOT via ListByPost.
	summaryRepo.EXPECT().Get(gomock.Any(), "sum-1").
		Return(biz.Summary{ID: "sum-1", Text: "digest text"}, nil)
	scorer.EXPECT().Model().Return("model-x")
	interestRepo.EXPECT().ListInterestVectors(gomock.Any()).
		Return([]biz.InterestVector{{Embedding: []float32{1, 0}, Model: "model-x"}}, nil)
	postEmbeddingRepo.EXPECT().Get(gomock.Any(), biz.ScoreKey{SummaryID: new("sum-1")}).
		Return(biz.PostEmbedding{}, biz.ErrPostEmbeddingNotFound)
	scorer.EXPECT().Embed(gomock.Any(), "digest text").Return([]float32{1, 0}, nil)
	postEmbeddingRepo.EXPECT().Upsert(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, pe biz.PostEmbedding) (biz.PostEmbedding, error) {
			require.NotNil(t, pe.SummaryID)
			assert.Equal(t, "sum-1", *pe.SummaryID)
			assert.Nil(t, pe.PostID, "digest embedding is keyed by summary_id only")
			return pe, nil
		})
	scoreRepo.EXPECT().Upsert(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s biz.Score) (biz.Score, error) {
			assert.InDelta(t, 1.0, s.Value, 1e-9)
			require.NotNil(t, s.SummaryID)
			assert.Equal(t, "sum-1", *s.SummaryID)
			assert.Nil(t, s.PostID, "digest score is keyed by summary_id only")
			s.ID = "score-d1"
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

	id, err := uc.ScorePost(
		context.Background(),
		biz.ScoreEvent{ID: "e-1", SummaryID: new("sum-1")},
	)

	require.NoError(t, err)
	assert.Equal(t, "score-d1", id)
}
