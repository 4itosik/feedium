package summary

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"feedium/internal/app/post"
)

// processResult contains the result of event processing.
type processResult struct {
	status         EventStatus
	incrementRetry bool
	err            error
}

// Worker processes outbox events and creates summaries.
type Worker struct {
	outboxEventRepo OutboxEventRepository
	summaryRepo     Repository
	postQueryRepo   PostQueryRepository
	sourceQueryRepo SourceQueryRepository
	processor       Processor
	logger          *slog.Logger
}

// NewWorker creates a new Worker instance.
func NewWorker(
	outboxEventRepo OutboxEventRepository,
	summaryRepo Repository,
	postQueryRepo PostQueryRepository,
	sourceQueryRepo SourceQueryRepository,
	processor Processor,
	logger *slog.Logger,
) *Worker {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Worker{
		outboxEventRepo: outboxEventRepo,
		summaryRepo:     summaryRepo,
		postQueryRepo:   postQueryRepo,
		sourceQueryRepo: sourceQueryRepo,
		processor:       processor,
		logger:          logger,
	}
}

// ProcessNext processes the next pending event.
// Returns true if an event was processed, false if no events were available.
// Returns error only for unexpected errors (should be logged and retried).
func (w *Worker) ProcessNext(ctx context.Context) (bool, error) {
	event, lockTime, err := w.outboxEventRepo.FetchAndLockPending(ctx)
	if err != nil {
		w.logger.ErrorContext(ctx, "failed to fetch pending event", "error", err)
		return false, err
	}

	if event == nil {
		return false, nil
	}

	l := w.logger.With("event_id", event.ID.String())
	result := w.processEvent(ctx, l, event, lockTime)
	w.handleProcessingResult(ctx, l, event, result)
	w.logProcessingError(ctx, l, result)

	return true, nil
}

// processEvent processes a single event and returns the result.
func (w *Worker) processEvent(
	ctx context.Context,
	l *slog.Logger,
	event *OutboxEvent,
	lockTime time.Time,
) *processResult {
	src, err := w.sourceQueryRepo.GetByID(ctx, event.SourceID)
	if err != nil {
		l.ErrorContext(ctx, fmt.Sprintf("failed to get source: source_id=%s", event.SourceID), "error", err)
		return &processResult{status: EventStatusFailed, incrementRetry: false}
	}

	mode, err := ProcessingModeForSourceType(src.Type)
	if err != nil {
		l.ErrorContext(
			ctx,
			fmt.Sprintf("unknown source type: source_id=%s, type=%s", event.SourceID, src.Type),
			"error", err,
		)
		return &processResult{status: EventStatusFailed, incrementRetry: false}
	}

	if mode == ModeCumulative && event.EventType == EventTypeImmediate {
		l.DebugContext(ctx, "skipping immediate event for cumulative source", "source_id", event.SourceID.String())
		return &processResult{status: EventStatusCompleted, incrementRetry: false}
	}

	switch mode {
	case ModeSelfContained:
		return w.processSelfContained(ctx, event, lockTime)
	case ModeCumulative:
		return w.processCumulative(ctx, event, lockTime)
	}

	return nil
}

// handleProcessingResult handles status updates and retry logic after processing.
func (w *Worker) handleProcessingResult(
	ctx context.Context,
	l *slog.Logger,
	event *OutboxEvent,
	result *processResult,
) {
	if result == nil {
		return
	}

	if result.status == EventStatusFailed && result.err != nil && !IsPermanentError(result.err) {
		if event.RetryCount < MaxRetries {
			backoffDuration := time.Duration(1<<event.RetryCount) * time.Minute
			scheduledAt := time.Now().Add(backoffDuration)

			requeueErr := w.outboxEventRepo.Requeue(ctx, event.ID, scheduledAt)
			if requeueErr != nil {
				l.ErrorContext(ctx, "failed to requeue event", "error", requeueErr)
			} else {
				l.DebugContext(ctx, "event requeued for retry",
					"retry_count", event.RetryCount+1,
					"scheduled_at", scheduledAt,
				)
			}
			return
		}
	}

	updateErr := w.outboxEventRepo.UpdateStatus(ctx, event.ID, result.status, result.incrementRetry)
	if updateErr != nil {
		msg := fmt.Sprintf(
			"failed to update event status: status=%s, increment_retry=%v",
			result.status,
			result.incrementRetry,
		)
		l.ErrorContext(ctx, msg, "error", updateErr)
	}
}

// logProcessingError logs processing errors with appropriate level.
func (w *Worker) logProcessingError(
	ctx context.Context,
	l *slog.Logger,
	result *processResult,
) {
	if result == nil || result.err == nil || result.status != EventStatusFailed {
		return
	}

	level := slog.LevelError
	if IsPermanentError(result.err) {
		level = slog.LevelWarn
	}

	l.Log(ctx, level, fmt.Sprintf("processing failed: increment_retry=%v", result.incrementRetry), "error", result.err)
}

// processSelfContained processes a self-contained event (one post per summary).
// Returns a processResult indicating success or failure with optional error details.
// This is a pure function with no side effects or logging.
func (w *Worker) processSelfContained(ctx context.Context, event *OutboxEvent, _ time.Time) *processResult {
	// Get the post
	if event.PostID == nil {
		return &processResult{
			status:         EventStatusFailed,
			incrementRetry: false,
			err:            ErrPostNotFound,
		}
	}

	p, err := w.postQueryRepo.GetByID(ctx, *event.PostID)
	if err != nil {
		return &processResult{
			status:         EventStatusFailed,
			incrementRetry: false,
			err:            err,
		}
	}

	// Process the post
	content, err := w.processor.Process(ctx, ModeSelfContained, []post.Post{*p})
	if err != nil {
		return &processResult{
			status:         EventStatusFailed,
			incrementRetry: !IsPermanentError(err),
			err:            err,
		}
	}

	// Create summary
	summary := &Summary{
		SourceID: event.SourceID,
		EventID:  event.ID,
		Content:  content,
	}

	err = w.summaryRepo.Create(ctx, summary, []uuid.UUID{p.ID})
	if err != nil {
		return &processResult{
			status:         EventStatusFailed,
			incrementRetry: true,
			err:            err,
		}
	}

	return &processResult{
		status:         EventStatusCompleted,
		incrementRetry: false,
		err:            nil,
	}
}

// processCumulative processes a cumulative event (multiple posts per summary).
// Returns a processResult indicating success or failure with optional error details.
// This is a pure function with no side effects or logging.
func (w *Worker) processCumulative(ctx context.Context, event *OutboxEvent, lockTime time.Time) *processResult {
	// Find unprocessed posts from the last 24 hours
	since := lockTime.Add(-24 * time.Hour)
	posts, err := w.postQueryRepo.FindUnprocessedBySource(ctx, event.SourceID, since)
	if err != nil {
		return &processResult{
			status:         EventStatusFailed,
			incrementRetry: true,
			err:            err,
		}
	}

	// If no posts found, mark event as completed without creating summary
	if len(posts) == 0 {
		return &processResult{
			status:         EventStatusCompleted,
			incrementRetry: false,
			err:            nil,
		}
	}

	// Process the posts
	content, err := w.processor.Process(ctx, ModeCumulative, posts)
	if err != nil {
		return &processResult{
			status:         EventStatusFailed,
			incrementRetry: !IsPermanentError(err),
			err:            err,
		}
	}

	// Create summary with all post IDs
	postIDs := make([]uuid.UUID, len(posts))
	for i, p := range posts {
		postIDs[i] = p.ID
	}

	summary := &Summary{
		SourceID: event.SourceID,
		EventID:  event.ID,
		Content:  content,
	}

	err = w.summaryRepo.Create(ctx, summary, postIDs)
	if err != nil {
		return &processResult{
			status:         EventStatusFailed,
			incrementRetry: true,
			err:            err,
		}
	}

	return &processResult{
		status:         EventStatusCompleted,
		incrementRetry: false,
		err:            nil,
	}
}
