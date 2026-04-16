package biz

import (
	"context"
)

type SummaryUsecase struct {
	summaryRepo SummaryRepo
	outboxRepo  SummaryOutboxRepo
	sourceRepo  SourceRepo
}

func NewSummaryUsecase(summaryRepo SummaryRepo, outboxRepo SummaryOutboxRepo, sourceRepo SourceRepo) *SummaryUsecase {
	return &SummaryUsecase{summaryRepo: summaryRepo, outboxRepo: outboxRepo, sourceRepo: sourceRepo}
}

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

	event := NewSummaryEvent(SummaryEventTypeSummarizeSource, sourceID, nil)
	saved, saveErr := uc.outboxRepo.Save(ctx, event)
	if saveErr != nil {
		return "", false, saveErr
	}

	return saved.ID, false, nil
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
