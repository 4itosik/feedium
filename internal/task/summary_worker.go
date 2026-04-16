package task

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

type SummaryWorker struct {
	outboxRepo  biz.SummaryOutboxRepo
	postRepo    biz.PostRepo
	summaryRepo biz.SummaryRepo
	llmProvider biz.LLMProvider
	cfg         *conf.Summary
	log         *slog.Logger
	done        chan struct{}
	wg          sync.WaitGroup
}

func NewSummaryWorker(
	outboxRepo biz.SummaryOutboxRepo,
	postRepo biz.PostRepo,
	summaryRepo biz.SummaryRepo,
	llmProvider biz.LLMProvider,
	cfg *conf.Summary,
	logger *slog.Logger,
) *SummaryWorker {
	return &SummaryWorker{
		outboxRepo:  outboxRepo,
		postRepo:    postRepo,
		summaryRepo: summaryRepo,
		llmProvider: llmProvider,
		cfg:         cfg,
		log:         logger,
		done:        make(chan struct{}),
	}
}

func (w *SummaryWorker) Start(ctx context.Context) error {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		pollInterval := w.cfg.GetWorker().GetPollInterval().AsDuration()
		if pollInterval == 0 {
			pollInterval = 5 * time.Second
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.done:
				return
			default:
				w.processBatch(ctx)
				select {
				case <-time.After(pollInterval):
				case <-ctx.Done():
					return
				case <-w.done:
					return
				}
			}
		}
	}()
	return nil
}

func (w *SummaryWorker) Stop(ctx context.Context) error {
	close(w.done)
	w.wg.Wait()
	return nil
}

func (w *SummaryWorker) processBatch(ctx context.Context) {
	batchSize := int(w.cfg.GetWorker().GetBatchSize())
	if batchSize <= 0 {
		batchSize = 10
	}

	events, err := w.outboxRepo.ListPending(ctx, batchSize)
	if err != nil {
		w.log.Error("failed to list pending events", "error", err)
		return
	}

	for _, event := range events {
		w.processEvent(ctx, event)
	}
}

func (w *SummaryWorker) processEvent(ctx context.Context, event biz.SummaryEvent) {
	start := time.Now()
	logger := w.log.With(
		"summary_event_id", event.ID,
		"event_type", event.EventType,
		"source_id", event.SourceID,
	)

	eventTTL := w.cfg.GetOutbox().GetEventTtl().AsDuration()
	if eventTTL == 0 {
		eventTTL = 24 * time.Hour
	}

	if time.Since(event.CreatedAt) > eventTTL {
		age := time.Since(event.CreatedAt)
		logger.Info("event expired", "age", age)
		if err := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusExpired, nil, nil); err != nil {
			logger.Error("failed to mark event as expired", "error", err)
		}
		return
	}

	if err := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusProcessing, nil, nil); err != nil {
		logger.Error("failed to mark event as processing", "error", err)
		return
	}

	switch event.EventType {
	case biz.SummaryEventTypeSummarizePost:
		w.processSummarizePost(ctx, logger, event)
	case biz.SummaryEventTypeSummarizeSource:
		w.processSummarizeSource(ctx, logger, event)
	}

	logger.Info("event processed", "status", "done", "duration", time.Since(start))
}

func (w *SummaryWorker) processSummarizePost(ctx context.Context, logger *slog.Logger, event biz.SummaryEvent) {
	post, err := w.postRepo.Get(ctx, *event.PostID)
	if err != nil {
		errText := "post not found"
		if !isNotFound(err) {
			errText = fmt.Sprintf("get post: %v", err)
		}
		if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusFailed, nil, &errText); updateErr != nil {
			logger.Error("failed to update status", "error", updateErr)
		}
		return
	}

	summaryText, err := w.summarizeWithRetry(ctx, logger, post.Text)
	if err != nil {
		errText := err.Error()
		if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusFailed, nil, &errText); updateErr != nil {
			logger.Error("failed to update status", "error", updateErr)
		}
		return
	}

	wordCount := len(strings.Fields(summaryText))
	sm := biz.Summary{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    event.PostID,
		SourceID:  event.SourceID,
		Text:      summaryText,
		WordCount: wordCount,
		CreatedAt: time.Now(),
	}

	saved, saveErr := w.summaryRepo.Save(ctx, sm)
	if saveErr != nil {
		errText := fmt.Sprintf("save summary: %v", saveErr)
		if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusFailed, nil, &errText); updateErr != nil {
			logger.Error("failed to update status", "error", updateErr)
		}
		return
	}

	summaryID := saved.ID
	if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusCompleted, &summaryID, nil); updateErr != nil {
		logger.Error("failed to update status", "error", updateErr)
	}
}

func (w *SummaryWorker) processSummarizeSource(ctx context.Context, logger *slog.Logger, event biz.SummaryEvent) {
	maxWindow := w.cfg.GetCumulative().GetMaxWindow().AsDuration()
	if maxWindow == 0 {
		maxWindow = 72 * time.Hour
	}
	maxInputChars := int(w.cfg.GetCumulative().GetMaxInputChars())
	if maxInputChars <= 0 {
		maxInputChars = 50000
	}

	lastSummary, err := w.summaryRepo.GetLastBySource(ctx, event.SourceID)
	if err != nil {
		errText := fmt.Sprintf("get last summary: %v", err)
		if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusFailed, nil, &errText); updateErr != nil {
			logger.Error("failed to update status", "error", updateErr)
		}
		return
	}

	now := time.Now()
	windowStart := now.Add(-maxWindow)
	if lastSummary != nil {
		if lastSummary.CreatedAt.After(windowStart) {
			windowStart = lastSummary.CreatedAt
		}
	}

	postsResult, err := w.postRepo.List(ctx, biz.ListPostsFilter{
		SourceID:      event.SourceID,
		PageSize:      500,
		OrderBy:       biz.SortByCreatedAt,
		OrderDir:      biz.SortAsc,
		CreatedAfter:  &windowStart,
		CreatedBefore: &now,
	})
	if err != nil {
		errText := fmt.Sprintf("list posts: %v", err)
		if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusFailed, nil, &errText); updateErr != nil {
			logger.Error("failed to update status", "error", updateErr)
		}
		return
	}

	if len(postsResult.Items) == 0 {
		if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusCompleted, nil, nil); updateErr != nil {
			logger.Error("failed to update status", "error", updateErr)
		}
		return
	}

	var sb strings.Builder
	for _, p := range postsResult.Items {
		sb.WriteString(p.Text)
		sb.WriteString("\n\n")
	}

	concat := sb.String()
	if len(concat) > maxInputChars {
		errText := "input text exceeds max_input_chars limit"
		if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusFailed, nil, &errText); updateErr != nil {
			logger.Error("failed to update status", "error", updateErr)
		}
		return
	}

	summaryText, err := w.summarizeWithRetry(ctx, logger, concat)
	if err != nil {
		errText := err.Error()
		if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusFailed, nil, &errText); updateErr != nil {
			logger.Error("failed to update status", "error", updateErr)
		}
		return
	}

	wordCount := len(strings.Fields(summaryText))
	sm := biz.Summary{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    nil,
		SourceID:  event.SourceID,
		Text:      summaryText,
		WordCount: wordCount,
		CreatedAt: time.Now(),
	}

	saved, saveErr := w.summaryRepo.Save(ctx, sm)
	if saveErr != nil {
		errText := fmt.Sprintf("save summary: %v", saveErr)
		if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusFailed, nil, &errText); updateErr != nil {
			logger.Error("failed to update status", "error", updateErr)
		}
		return
	}

	summaryID := saved.ID
	if updateErr := w.outboxRepo.UpdateStatus(ctx, event.ID, biz.SummaryEventStatusCompleted, &summaryID, nil); updateErr != nil {
		logger.Error("failed to update status", "error", updateErr)
	}
}

func (w *SummaryWorker) summarizeWithRetry(ctx context.Context, logger *slog.Logger, text string) (string, error) {
	maxRetries := int(w.cfg.GetLlm().GetMaxRetries())
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := range maxRetries {
		result, err := w.llmProvider.Summarize(ctx, text)
		if err != nil {
			lastErr = err
			logger.Warn("llm summarize attempt failed", "attempt", attempt+1, "error", err)
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			continue
		}

		if strings.TrimSpace(result) == "" {
			lastErr = fmt.Errorf("llm returned empty summary on attempt %d", attempt+1)
			logger.Warn("empty summary", "attempt", attempt+1)
			continue
		}

		return result, nil
	}

	return "", fmt.Errorf("all %d attempts failed: %w", maxRetries, lastErr)
}

func isNotFound(err error) bool {
	return err != nil && (err == biz.ErrPostNotFound || err == biz.ErrSourceNotFound || err == biz.ErrSummaryNotFound)
}
