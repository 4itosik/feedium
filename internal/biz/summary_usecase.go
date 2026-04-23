package biz

import (
	"context"
	"time"

	"github.com/4itosik/feedium/internal/conf"
)

type SummaryUsecase struct {
	summaryRepo SummaryRepo
	outboxRepo  SummaryOutboxRepo
	sourceRepo  SourceRepo
	txManager   TxManager
	cfg         *conf.Summary
}

func NewSummaryUsecase(
	summaryRepo SummaryRepo,
	outboxRepo SummaryOutboxRepo,
	sourceRepo SourceRepo,
	txManager TxManager,
	cfg *conf.Summary,
) *SummaryUsecase {
	return &SummaryUsecase{
		summaryRepo: summaryRepo,
		outboxRepo:  outboxRepo,
		sourceRepo:  sourceRepo,
		txManager:   txManager,
		cfg:         cfg,
	}
}

// TriggerSourceSummarize enqueues a summarize_source event for a cumulative source and
// bumps sources.next_summary_at by cron.interval in the same transaction (OQ-02).
// A manual trigger thus behaves as an early-fired scheduled tick: the next scheduled
// summarization still lands a full cron.interval after the manual one.
func (uc *SummaryUsecase) TriggerSourceSummarize(ctx context.Context, sourceID string) (string, bool, error) {
	source, err := uc.sourceRepo.Get(ctx, sourceID)
	if err != nil {
		return "", false, err
	}

	if ProcessingModeForType(source.Type) != ProcessingModeCumulative {
		return "", false, ErrSummarizeSelfContainedSrc
	}

	found, activeEvent, err := uc.outboxRepo.HasActiveEvent(ctx, sourceID, SummaryEventTypeSummarizeSource)
	if err != nil {
		return "", false, err
	}
	if found {
		return activeEvent.ID, true, nil
	}

	cronInterval := uc.cfg.GetCron().GetInterval().AsDuration()
	if cronInterval <= 0 {
		cronInterval = time.Hour
	}
	nextAt := time.Now().Add(cronInterval)

	var eventID string
	txErr := uc.txManager.InTx(ctx, func(txCtx context.Context) error {
		event := NewSummaryEvent(SummaryEventTypeSummarizeSource, sourceID, nil)
		saved, saveErr := uc.outboxRepo.Save(txCtx, event)
		if saveErr != nil {
			return saveErr
		}
		eventID = saved.ID
		return uc.sourceRepo.BumpNextSummaryAt(txCtx, sourceID, nextAt)
	})
	if txErr != nil {
		return "", false, txErr
	}

	return eventID, false, nil
}

func (uc *SummaryUsecase) GetSummaryEvent(ctx context.Context, id string) (*SummaryEvent, error) {
	event, err := uc.outboxRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (uc *SummaryUsecase) GetSummary(ctx context.Context, id string) (Summary, error) {
	return uc.summaryRepo.Get(ctx, id)
}

func (uc *SummaryUsecase) ListPostSummaries(ctx context.Context, postID string) ([]Summary, error) {
	return uc.summaryRepo.ListByPost(ctx, postID)
}

func (uc *SummaryUsecase) ListSourceSummaries(
	ctx context.Context,
	sourceID string,
	pageSize int,
	pageToken string,
) (ListSummariesResult, error) {
	return uc.summaryRepo.ListBySource(ctx, sourceID, pageSize, pageToken)
}
