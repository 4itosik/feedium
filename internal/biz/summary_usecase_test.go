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

	uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, nil, nil, txMgr, cfg)

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

	uc := biz.NewSummaryUsecase(summaryRepo, outboxRepo, sourceRepo, nil, nil, txMgr, cfg)
	id, existed, err := uc.TriggerSourceSummarize(context.Background(), sourceID)
	require.NoError(t, err)
	assert.True(t, existed)
	assert.Equal(t, "existing-event", id)
}
