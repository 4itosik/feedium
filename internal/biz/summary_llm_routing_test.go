package biz_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/biz/mock"
)

// staticLLMResolver routes every ProcessingMode to one provider; it keeps the
// tests that are not about per-mode routing (FT-021) focused on their subject.
type staticLLMResolver struct{ p biz.LLMProvider }

func (r staticLLMResolver) For(biz.ProcessingMode) (biz.LLMProvider, error) { return r.p, nil }

func llmFor(p biz.LLMProvider) biz.SummaryLLMResolver { return staticLLMResolver{p: p} }

// TestSummaryUsecase_LLMByMode asserts FT-021 REQ-02: each mode is summarized
// by the provider resolved for that mode. The opposing mode's provider is a
// strict mock with no expectations, so any cross-mode call fails the test.
func TestSummaryUsecase_LLMByMode(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	postID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3e"

	cases := []struct {
		name string
		mode biz.ProcessingMode
		run  func(uc *biz.SummaryUsecase, postRepo *mock.MockPostRepo, summaryRepo *mock.MockSummaryRepo) (string, error)
	}{
		{
			name: "self_contained",
			mode: biz.ProcessingModeSelfContained,
			run: func(uc *biz.SummaryUsecase, postRepo *mock.MockPostRepo, summaryRepo *mock.MockSummaryRepo) (string, error) {
				summaryRepo.EXPECT().ListByPost(gomock.Any(), postID).Return(nil, nil)
				postRepo.EXPECT().Get(gomock.Any(), postID).Return(biz.Post{ID: postID, Text: "post"}, nil)
				return uc.ProcessPostEvent(context.Background(), biz.SummaryEvent{
					ID:        "e-1",
					PostID:    &postID,
					SourceID:  sourceID,
					EventType: biz.SummaryEventTypeSummarizePost,
					CreatedAt: time.Now(),
				})
			},
		},
		{
			name: "cumulative",
			mode: biz.ProcessingModeCumulative,
			run: func(uc *biz.SummaryUsecase, postRepo *mock.MockPostRepo, summaryRepo *mock.MockSummaryRepo) (string, error) {
				summaryRepo.EXPECT().GetLastBySource(gomock.Any(), sourceID).Return(nil, nil)
				postRepo.EXPECT().List(gomock.Any(), gomock.Any()).Return(biz.ListPostsResult{
					Items: []biz.Post{{ID: "p1", Text: "post-1"}},
				}, nil)
				return uc.ProcessSourceEvent(context.Background(), biz.SummaryEvent{
					ID:        "e-2",
					SourceID:  sourceID,
					EventType: biz.SummaryEventTypeSummarizeSource,
					CreatedAt: time.Now(),
				})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			summaryRepo := mock.NewMockSummaryRepo(ctrl)
			postRepo := mock.NewMockPostRepo(ctrl)
			prompts := mock.NewMockSummaryPromptResolver(ctrl)
			llms := mock.NewMockSummaryLLMResolver(ctrl)
			routed := mock.NewMockLLMProvider(ctrl)
			other := mock.NewMockLLMProvider(ctrl) // strict: must never be called

			prompts.EXPECT().For(tc.mode).Return("SP", nil)
			llms.EXPECT().For(tc.mode).Return(routed, nil).Times(1)
			llms.EXPECT().For(gomock.Not(tc.mode)).Return(other, nil).AnyTimes()
			routed.EXPECT().Summarize(gomock.Any(), "SP", gomock.Any()).Return("summary", nil)
			summaryRepo.EXPECT().Save(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, s biz.Summary) (biz.Summary, error) {
					s.ID = "saved-id"
					return s, nil
				})

			uc := biz.NewSummaryUsecase(
				summaryRepo,
				mock.NewMockSummaryOutboxRepo(ctrl),
				mock.NewMockSourceRepo(ctrl),
				postRepo,
				llms,
				prompts,
				mock.NewMockTxManager(ctrl),
				shortTimeoutSummaryCfg(),
			)

			id, err := tc.run(uc, postRepo, summaryRepo)
			require.NoError(t, err)
			assert.Equal(t, "saved-id", id)
		})
	}
}

// TestSummaryUsecase_LLMResolverError_NoCall asserts that a provider that
// cannot be resolved for the mode fails the event without any LLM call and
// without entering the retry loop.
func TestSummaryUsecase_LLMResolverError_NoCall(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)

	summaryRepo := mock.NewMockSummaryRepo(ctrl)
	postRepo := mock.NewMockPostRepo(ctrl)
	prompts := mock.NewMockSummaryPromptResolver(ctrl)
	llms := mock.NewMockSummaryLLMResolver(ctrl)

	postID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3e"
	errNoProvider := errors.New("no provider for mode")

	summaryRepo.EXPECT().ListByPost(gomock.Any(), postID).Return(nil, nil)
	postRepo.EXPECT().Get(gomock.Any(), postID).Return(biz.Post{ID: postID, Text: "post"}, nil)
	prompts.EXPECT().For(biz.ProcessingModeSelfContained).Return("SP", nil)
	llms.EXPECT().For(biz.ProcessingModeSelfContained).Return(nil, errNoProvider).Times(1)

	uc := biz.NewSummaryUsecase(
		summaryRepo,
		mock.NewMockSummaryOutboxRepo(ctrl),
		mock.NewMockSourceRepo(ctrl),
		postRepo,
		llms,
		prompts,
		mock.NewMockTxManager(ctrl),
		shortTimeoutSummaryCfg(),
	)

	_, err := uc.ProcessPostEvent(context.Background(), biz.SummaryEvent{
		ID:        "e",
		PostID:    &postID,
		SourceID:  "s",
		EventType: biz.SummaryEventTypeSummarizePost,
		CreatedAt: time.Now(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errNoProvider)
}
