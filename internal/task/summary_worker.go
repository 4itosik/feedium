package task

import (
	"context"
	"errors"
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

const (
	defaultPollInterval = 5 * time.Second
	defaultEventTTL     = 24 * time.Hour
	defaultMaxWindow    = 72 * time.Hour
	listPageSize        = 500
	backoffBase         = 2
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
	w.wg.Go(func() {
		pollInterval := w.cfg.GetWorker().GetPollInterval().AsDuration()
		if pollInterval == 0 {
			pollInterval = defaultPollInterval
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
	})
	return nil
}

func (w *SummaryWorker) Stop(_ context.Context) error {
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
		w.log.ErrorContext(ctx, "failed to list pending events", "error", err)
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
		eventTTL = defaultEventTTL
	}

	if time.Since(event.CreatedAt) > eventTTL {
		age := time.Since(event.CreatedAt)
		logger.InfoContext(ctx, "event expired", "age", age)
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusExpired, nil, nil,
		)
		return
	}

	w.updateEventStatus(
		ctx, logger, event.ID,
		biz.SummaryEventStatusProcessing, nil, nil,
	)

	switch event.EventType {
	case biz.SummaryEventTypeSummarizePost:
		w.processSummarizePost(ctx, logger, event)
	case biz.SummaryEventTypeSummarizeSource:
		w.processSummarizeSource(ctx, logger, event)
	}

	logger.InfoContext(ctx, "event processed", "status", "done", "duration", time.Since(start))
}

func (w *SummaryWorker) processSummarizePost(
	ctx context.Context,
	logger *slog.Logger,
	event biz.SummaryEvent,
) {
	if event.PostID == nil {
		errText := "post id is nil for summarize_post event"
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusFailed, nil, &errText,
		)
		return
	}

	post, err := w.postRepo.Get(ctx, *event.PostID)
	if err != nil {
		errText := "post not found"
		if !isNotFound(err) {
			errText = fmt.Sprintf("get post: %v", err)
		}
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusFailed, nil, &errText,
		)
		return
	}

	summaryText, err := w.summarizeWithRetry(ctx, logger, post.Text)
	if err != nil {
		errText := err.Error()
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusFailed, nil, &errText,
		)
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
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusFailed, nil, &errText,
		)
		return
	}

	summaryID := saved.ID
	w.updateEventStatus(
		ctx, logger, event.ID,
		biz.SummaryEventStatusCompleted, &summaryID, nil,
	)
}

//nolint:funlen // complex event processing with multiple error handling paths
func (w *SummaryWorker) processSummarizeSource(
	ctx context.Context,
	logger *slog.Logger,
	event biz.SummaryEvent,
) {
	maxWindow := w.cfg.GetCumulative().GetMaxWindow().AsDuration()
	if maxWindow == 0 {
		maxWindow = defaultMaxWindow
	}
	maxInputChars := int(w.cfg.GetCumulative().GetMaxInputChars())
	if maxInputChars <= 0 {
		maxInputChars = 50000
	}

	lastSummary, err := w.summaryRepo.GetLastBySource(ctx, event.SourceID)
	if err != nil {
		errText := fmt.Sprintf("get last summary: %v", err)
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusFailed, nil, &errText,
		)
		return
	}

	now := time.Now()
	windowStart := now.Add(-maxWindow)
	if lastSummary != nil && lastSummary.CreatedAt.After(windowStart) {
		windowStart = lastSummary.CreatedAt
	}

	var sb strings.Builder
	foundPosts := false
	pageToken := ""
	exceeded := false

	for {
		postsResult, listErr := w.postRepo.List(ctx, biz.ListPostsFilter{
			SourceID:      event.SourceID,
			PageSize:      listPageSize,
			PageToken:     pageToken,
			OrderBy:       biz.SortByCreatedAt,
			OrderDir:      biz.SortAsc,
			CreatedAfter:  &windowStart,
			CreatedBefore: &now,
		})
		if listErr != nil {
			errText := fmt.Sprintf("list posts: %v", listErr)
			w.updateEventStatus(
				ctx, logger, event.ID,
				biz.SummaryEventStatusFailed, nil, &errText,
			)
			return
		}

		for _, p := range postsResult.Items {
			foundPosts = true
			sb.WriteString(p.Text)
			sb.WriteString("\n\n")
			if sb.Len() > maxInputChars {
				exceeded = true
				break
			}
		}

		if postsResult.NextPageToken == "" || exceeded {
			break
		}
		pageToken = postsResult.NextPageToken
	}

	if !foundPosts {
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusCompleted, nil, nil,
		)
		return
	}

	concat := sb.String()
	if len(concat) > maxInputChars {
		errText := "input text exceeds max_input_chars limit"
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusFailed, nil, &errText,
		)
		return
	}

	summaryText, err := w.summarizeWithRetry(ctx, logger, concat)
	if err != nil {
		errText := err.Error()
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusFailed, nil, &errText,
		)
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
		w.updateEventStatus(
			ctx, logger, event.ID,
			biz.SummaryEventStatusFailed, nil, &errText,
		)
		return
	}

	summaryID := saved.ID
	w.updateEventStatus(
		ctx, logger, event.ID,
		biz.SummaryEventStatusCompleted, &summaryID, nil,
	)
}

func (w *SummaryWorker) updateEventStatus(
	ctx context.Context,
	logger *slog.Logger,
	eventID string,
	status biz.SummaryEventStatus,
	summaryID *string,
	errText *string,
) {
	if updateErr := w.outboxRepo.UpdateStatus(
		ctx, eventID, status, summaryID, errText,
	); updateErr != nil {
		logger.ErrorContext(ctx, "failed to update status", "error", updateErr)
	}
}

func (w *SummaryWorker) summarizeWithRetry(
	ctx context.Context,
	logger *slog.Logger,
	text string,
) (string, error) {
	maxRetries := int(w.cfg.GetLlm().GetMaxRetries())
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := range maxRetries {
		result, err := w.llmProvider.Summarize(ctx, text)
		if err != nil {
			lastErr = err
			logger.WarnContext(
				ctx, "llm summarize attempt failed",
				"attempt", attempt+1, "error", err,
			)
			backoff := time.Duration(
				math.Pow(backoffBase, float64(attempt)),
			) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			continue
		}

		if strings.TrimSpace(result) == "" {
			lastErr = fmt.Errorf(
				"llm returned empty summary on attempt %d",
				attempt+1,
			)
			logger.WarnContext(ctx, "empty summary", "attempt", attempt+1)
			continue
		}

		return result, nil
	}

	return "", fmt.Errorf("all %d attempts failed: %w", maxRetries, lastErr)
}

func isNotFound(err error) bool {
	return errors.Is(err, biz.ErrPostNotFound) ||
		errors.Is(err, biz.ErrSourceNotFound) ||
		errors.Is(err, biz.ErrSummaryNotFound)
}
