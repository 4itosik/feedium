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
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/biz/mock"
	"github.com/4itosik/feedium/internal/conf"
)

func TestSummaryUsecase_TriggerSourceSummarize_BumpsNextSummaryAtInTx(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sourceRepo := mock.NewMockSourceRepo(ctrl)
	outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
	summaryRepo := mock.NewMockSummaryRepo(ctrl)
	txMgr := mock.NewMockTxManager(ctrl)

	cfg := &conf.Summary{
		Cron: &conf.SummaryCron{Interval: durationpb.New(1 * time.Hour)},
	}

	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	sourceRepo.EXPECT().Get(gomock.Any(), sourceID).Return(biz.Source{
		ID:   sourceID,
		Type: biz.SourceTypeTelegramGroup,
	}, nil)

	outboxRepo.EXPECT().
		HasActiveEvent(gomock.Any(), sourceID, biz.SummaryEventTypeSummarizeSource).
		Return(false, nil, nil)

	// Both Save and BumpNextSummaryAt must be called under the same InTx closure.
	txMgr.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)

	outboxRepo.EXPECT().
		Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, e biz.SummaryEvent) (biz.SummaryEvent, error) {
			assert.Equal(t, biz.SummaryEventTypeSummarizeSource, e.EventType)
			return e, nil
		})

	sourceRepo.EXPECT().
		BumpNextSummaryAt(gomock.Any(), sourceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, at time.Time) error {
			// Must be ~cron.interval (1h) in the future.
			diff := time.Until(at)
			if diff < 55*time.Minute || diff > 65*time.Minute {
				t.Errorf("next_summary_at outside expected ~1h window: %s", diff)
			}
			return nil
		})

	uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, nil, nil, nil, txMgr, cfg)

	id, existed, err := uc.TriggerSourceSummarize(context.Background(), sourceID)
	require.NoError(t, err)
	assert.False(t, existed)
	assert.NotEmpty(t, id)
}

func TestSummaryUsecase_TriggerSourceSummarize_ExistingEventNoTx(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sourceRepo := mock.NewMockSourceRepo(ctrl)
	outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
	summaryRepo := mock.NewMockSummaryRepo(ctrl)
	txMgr := mock.NewMockTxManager(ctrl)
	cfg := &conf.Summary{Cron: &conf.SummaryCron{Interval: durationpb.New(time.Hour)}}

	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	sourceRepo.EXPECT().Get(gomock.Any(), sourceID).Return(biz.Source{
		ID:   sourceID,
		Type: biz.SourceTypeTelegramGroup,
	}, nil)

	existing := &biz.SummaryEvent{ID: "existing-event", Status: biz.SummaryEventStatusPending}
	outboxRepo.EXPECT().
		HasActiveEvent(gomock.Any(), sourceID, biz.SummaryEventTypeSummarizeSource).
		Return(true, existing, nil)
	// InTx / Save / Bump must NOT be called — gomock strict controller fails otherwise.

	uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, nil, nil, nil, txMgr, cfg)
	id, existed, err := uc.TriggerSourceSummarize(context.Background(), sourceID)
	require.NoError(t, err)
	assert.True(t, existed)
	assert.Equal(t, "existing-event", id)
}

// shortTimeoutSummaryCfg makes summarize-retry attempts fast in tests so that
// transient-error paths complete within the test budget without sleeping.
func shortTimeoutSummaryCfg() *conf.Summary {
	return &conf.Summary{
		Outbox:     &conf.SummaryOutbox{EventTtl: durationpb.New(24 * time.Hour)},
		Llm:        &conf.SummaryLLM{MaxRetries: 1, Timeout: durationpb.New(time.Second)},
		Cumulative: &conf.SummaryCumulative{MaxWindow: durationpb.New(72 * time.Hour), MaxInputChars: 50000},
	}
}

func TestSummaryUsecase_PromptByMode_SelfContained(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	summaryRepo := mock.NewMockSummaryRepo(ctrl)
	outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
	sourceRepo := mock.NewMockSourceRepo(ctrl)
	postRepo := mock.NewMockPostRepo(ctrl)
	llm := mock.NewMockLLMProvider(ctrl)
	prompts := mock.NewMockSummaryPromptResolver(ctrl)
	txMgr := mock.NewMockTxManager(ctrl)

	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	postID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3e"
	const sentinel = "SENTINEL-SELF-CONTAINED"

	event := biz.SummaryEvent{
		ID:        "evt-1",
		PostID:    &postID,
		SourceID:  sourceID,
		EventType: biz.SummaryEventTypeSummarizePost,
		CreatedAt: time.Now(),
	}

	summaryRepo.EXPECT().ListByPost(gomock.Any(), postID).Return(nil, nil)
	postRepo.EXPECT().Get(gomock.Any(), postID).Return(biz.Post{ID: postID, Text: "post-body"}, nil)

	prompts.EXPECT().
		For(biz.ProcessingModeSelfContained).
		Return(sentinel, nil).
		Times(1)

	var captured string
	llm.EXPECT().
		Summarize(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, systemPrompt, _ string) (string, error) {
			captured = systemPrompt
			return "summary", nil
		})

	summaryRepo.EXPECT().Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s biz.Summary) (biz.Summary, error) {
			s.ID = "saved-id"
			return s, nil
		})

	uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, postRepo, llm, prompts, txMgr, shortTimeoutSummaryCfg())

	id, err := uc.ProcessPostEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Equal(t, "saved-id", id)
	assert.Equal(t, sentinel, captured,
		"LLM must receive the prompt body returned by the resolver for self-contained mode")
}

func TestSummaryUsecase_PromptByMode_Cumulative(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	summaryRepo := mock.NewMockSummaryRepo(ctrl)
	outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
	sourceRepo := mock.NewMockSourceRepo(ctrl)
	postRepo := mock.NewMockPostRepo(ctrl)
	llm := mock.NewMockLLMProvider(ctrl)
	prompts := mock.NewMockSummaryPromptResolver(ctrl)
	txMgr := mock.NewMockTxManager(ctrl)

	sourceID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d"
	const sentinel = "SENTINEL-CUMULATIVE"

	event := biz.SummaryEvent{
		ID:        "evt-2",
		SourceID:  sourceID,
		EventType: biz.SummaryEventTypeSummarizeSource,
		CreatedAt: time.Now(),
	}

	summaryRepo.EXPECT().GetLastBySource(gomock.Any(), sourceID).Return(nil, nil)
	postRepo.EXPECT().List(gomock.Any(), gomock.Any()).Return(biz.ListPostsResult{
		Items: []biz.Post{{ID: "p1", Text: "post-1"}, {ID: "p2", Text: "post-2"}},
	}, nil)

	prompts.EXPECT().
		For(biz.ProcessingModeCumulative).
		Return(sentinel, nil).
		Times(1)

	var captured string
	llm.EXPECT().
		Summarize(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, systemPrompt, _ string) (string, error) {
			captured = systemPrompt
			return "summary", nil
		})

	summaryRepo.EXPECT().Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s biz.Summary) (biz.Summary, error) {
			s.ID = "saved-id"
			return s, nil
		})

	uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, postRepo, llm, prompts, txMgr, shortTimeoutSummaryCfg())

	id, err := uc.ProcessSourceEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Equal(t, "saved-id", id)
	assert.Equal(t, sentinel, captured,
		"LLM must receive the prompt body returned by the resolver for cumulative mode")
}

// TestSummaryUsecase_PromptByMode_Isolation asserts that the prompt selected
// for one mode does not flow through the other call site. Each subtest stubs
// the resolver so that "its" mode returns a unique sentinel and the opposing
// mode returns ErrUnknownSummaryPromptMode (which the resolver mock is never
// asked for in production code paths) — gomock's strict controller catches
// any cross-mode resolution.
func TestSummaryUsecase_PromptByMode_Isolation(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	cases := []struct {
		name     string
		run      func(t *testing.T)
		sentinel string
	}{
		{
			name:     "self_contained_does_not_leak_cumulative_body",
			sentinel: "SC-ONLY",
			run: func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				summaryRepo := mock.NewMockSummaryRepo(ctrl)
				outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
				sourceRepo := mock.NewMockSourceRepo(ctrl)
				postRepo := mock.NewMockPostRepo(ctrl)
				llm := mock.NewMockLLMProvider(ctrl)
				prompts := mock.NewMockSummaryPromptResolver(ctrl)
				txMgr := mock.NewMockTxManager(ctrl)

				postID := "p-1"
				summaryRepo.EXPECT().ListByPost(gomock.Any(), postID).Return(nil, nil)
				postRepo.EXPECT().Get(gomock.Any(), postID).Return(biz.Post{ID: postID, Text: "x"}, nil)
				prompts.EXPECT().For(biz.ProcessingModeSelfContained).Return("SC-ONLY", nil)
				// Cumulative resolver call is unexpected — strict gomock fails the test
				// if any code path queries it.
				var got string
				llm.EXPECT().Summarize(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, sp, _ string) (string, error) {
						got = sp
						return "ok", nil
					})
				summaryRepo.EXPECT().Save(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, s biz.Summary) (biz.Summary, error) {
						s.ID = "ok-id"
						return s, nil
					})

				uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, postRepo, llm, prompts, txMgr, shortTimeoutSummaryCfg())
				_, err := uc.ProcessPostEvent(context.Background(), biz.SummaryEvent{
					ID:        "e",
					PostID:    &postID,
					SourceID:  "s",
					EventType: biz.SummaryEventTypeSummarizePost,
					CreatedAt: time.Now(),
				})
				require.NoError(t, err)
				assert.Equal(t, "SC-ONLY", got)
			},
		},
		{
			name:     "cumulative_does_not_leak_self_contained_body",
			sentinel: "CUM-ONLY",
			run: func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				summaryRepo := mock.NewMockSummaryRepo(ctrl)
				outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
				sourceRepo := mock.NewMockSourceRepo(ctrl)
				postRepo := mock.NewMockPostRepo(ctrl)
				llm := mock.NewMockLLMProvider(ctrl)
				prompts := mock.NewMockSummaryPromptResolver(ctrl)
				txMgr := mock.NewMockTxManager(ctrl)

				summaryRepo.EXPECT().GetLastBySource(gomock.Any(), gomock.Any()).Return(nil, nil)
				postRepo.EXPECT().List(gomock.Any(), gomock.Any()).Return(biz.ListPostsResult{
					Items: []biz.Post{{ID: "p", Text: "x"}},
				}, nil)
				prompts.EXPECT().For(biz.ProcessingModeCumulative).Return("CUM-ONLY", nil)
				var got string
				llm.EXPECT().Summarize(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, sp, _ string) (string, error) {
						got = sp
						return "ok", nil
					})
				summaryRepo.EXPECT().Save(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, s biz.Summary) (biz.Summary, error) {
						s.ID = "ok-id"
						return s, nil
					})

				uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, postRepo, llm, prompts, txMgr, shortTimeoutSummaryCfg())
				_, err := uc.ProcessSourceEvent(context.Background(), biz.SummaryEvent{
					ID:        "e",
					SourceID:  "s",
					EventType: biz.SummaryEventTypeSummarizeSource,
					CreatedAt: time.Now(),
				})
				require.NoError(t, err)
				assert.Equal(t, "CUM-ONLY", got)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}

func TestSummaryUsecase_PromptResolveError_IsTerminal_SelfContained(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	summaryRepo := mock.NewMockSummaryRepo(ctrl)
	outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
	sourceRepo := mock.NewMockSourceRepo(ctrl)
	postRepo := mock.NewMockPostRepo(ctrl)
	llm := mock.NewMockLLMProvider(ctrl) // strict: no Summarize call expected
	prompts := mock.NewMockSummaryPromptResolver(ctrl)
	txMgr := mock.NewMockTxManager(ctrl)

	postID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3e"

	summaryRepo.EXPECT().ListByPost(gomock.Any(), postID).Return(nil, nil)
	postRepo.EXPECT().Get(gomock.Any(), postID).Return(biz.Post{ID: postID, Text: "post"}, nil)
	prompts.EXPECT().For(biz.ProcessingModeSelfContained).
		Return("", biz.ErrUnknownSummaryPromptMode)

	uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, postRepo, llm, prompts, txMgr, shortTimeoutSummaryCfg())

	_, err := uc.ProcessPostEvent(context.Background(), biz.SummaryEvent{
		ID:        "e",
		PostID:    &postID,
		SourceID:  "s",
		EventType: biz.SummaryEventTypeSummarizePost,
		CreatedAt: time.Now(),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, biz.ErrUnknownSummaryPromptMode),
		"resolver failure must propagate as biz.ErrUnknownSummaryPromptMode (errors.Is)")
}

func TestSummaryUsecase_PromptResolvedOncePerEvent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	summaryRepo := mock.NewMockSummaryRepo(ctrl)
	outboxRepo := mock.NewMockSummaryOutboxRepo(ctrl)
	sourceRepo := mock.NewMockSourceRepo(ctrl)
	postRepo := mock.NewMockPostRepo(ctrl)
	llm := mock.NewMockLLMProvider(ctrl)
	prompts := mock.NewMockSummaryPromptResolver(ctrl)
	txMgr := mock.NewMockTxManager(ctrl)

	postID := "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3e"

	summaryRepo.EXPECT().ListByPost(gomock.Any(), postID).Return(nil, nil)
	postRepo.EXPECT().Get(gomock.Any(), postID).Return(biz.Post{ID: postID, Text: "x"}, nil)
	// EXACTLY ONE resolver call even though the LLM is retried.
	prompts.EXPECT().For(biz.ProcessingModeSelfContained).Return("SP", nil).Times(1)

	// One transient error, then success — exercises the retry loop while
	// keeping the unit test budget tight (one ~1s sleep).
	gomock.InOrder(
		llm.EXPECT().Summarize(gomock.Any(), "SP", gomock.Any()).Return("", errors.New("boom")),
		llm.EXPECT().Summarize(gomock.Any(), "SP", gomock.Any()).Return("ok", nil),
	)
	summaryRepo.EXPECT().Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s biz.Summary) (biz.Summary, error) {
			s.ID = "ok-id"
			return s, nil
		})

	cfg := &conf.Summary{
		Outbox: &conf.SummaryOutbox{EventTtl: durationpb.New(24 * time.Hour)},
		// Two attempts: one failure (with ~1s backoff) followed by success.
		Llm: &conf.SummaryLLM{MaxRetries: 2, Timeout: durationpb.New(time.Second)},
	}

	uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, postRepo, llm, prompts, txMgr, cfg)

	id, err := uc.ProcessPostEvent(context.Background(), biz.SummaryEvent{
		ID:        "e",
		PostID:    &postID,
		SourceID:  "s",
		EventType: biz.SummaryEventTypeSummarizePost,
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)
	assert.Equal(t, "ok-id", id)
}
