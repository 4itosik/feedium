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
	// Fetch the next pending event
	event, lockTime, err := w.outboxEventRepo.FetchAndLockPending(ctx)
	if err != nil {
		w.logger.ErrorContext(ctx, "failed to fetch pending event", "error", err)
		return false, err
	}

	if event == nil {
		// No pending events available
		return false, nil
	}

	// Create a logger with event_id context
	l := w.logger.With("event_id", event.ID.String())

	// Setup defer to handle all status updates
	var result *processResult
	defer func() {
		if result == nil {
			return
		}

		// Update event status based on processing result
		updateErr := w.outboxEventRepo.UpdateStatus(ctx, event.ID, result.status, result.incrementRetry)
		if updateErr != nil {
			l.ErrorContext(
				ctx,
				fmt.Sprintf(
					"failed to update event status: status=%s, increment_retry=%v",
					result.status,
					result.incrementRetry,
				),
				"error",
				updateErr,
			)
		}
	}()

	// Get the source to determine processing mode
	src, err := w.sourceQueryRepo.GetByID(ctx, event.SourceID)
	if err != nil {
		l.ErrorContext(ctx, fmt.Sprintf("failed to get source: source_id=%s", event.SourceID), "error", err)
		result = &processResult{status: EventStatusFailed, incrementRetry: false}
		return true, nil
	}

	// Determine processing mode
	mode, err := ProcessingModeForSourceType(src.Type)
	if err != nil {
		l.ErrorContext(
			ctx,
			fmt.Sprintf("unknown source type: source_id=%s, type=%s", event.SourceID, src.Type),
			"error",
			err,
		)
		result = &processResult{status: EventStatusFailed, incrementRetry: false}
		return true, nil
	}

	// Skip IMMEDIATE events for cumulative sources (they are only processed via SCHEDULED/MANUAL events)
	if mode == ModeCumulative && event.EventType == EventTypeImmediate {
		l.DebugContext(ctx, "skipping immediate event for cumulative source", "source_id", event.SourceID.String())
		result = &processResult{status: EventStatusCompleted, incrementRetry: false}
		return true, nil
	}

	// Process based on mode
	switch mode {
	case ModeSelfContained:
		result = w.processSelfContained(ctx, event, lockTime)
	case ModeCumulative:
		result = w.processCumulative(ctx, event, lockTime)
	}

	// Log any processing errors
	if result != nil && result.err != nil && result.status == EventStatusFailed {
		level := slog.LevelError
		if IsPermanentError(result.err) {
			level = slog.LevelWarn
		}
		l.Log(
			ctx,
			level,
			fmt.Sprintf("processing failed: increment_retry=%v", result.incrementRetry),
			"error",
			result.err,
		)
	}

	return true, nil
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
	content, err := w.processor.Process(ctx, []post.Post{*p})
	if err != nil {
		return &processResult{
			status:         EventStatusFailed,
			incrementRetry: true,
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
	content, err := w.processor.Process(ctx, posts)
	if err != nil {
		return &processResult{
			status:         EventStatusFailed,
			incrementRetry: true,
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
