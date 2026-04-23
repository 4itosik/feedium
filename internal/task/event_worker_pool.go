// Package task implements background workers for FT-007 scalable event processing.
// See memory-bank/features/FT-007-scalable-event-processing/feature.md for the canonical
// requirements (REQ-01..09) and architectural invariants.
package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

const (
	defaultPollInterval      = 5 * time.Second
	defaultConcurrency       = 2
	defaultLeaseTTL          = 5 * time.Minute
	defaultHeartbeatInterval = 1 * time.Minute
	defaultGracefulTimeout   = 10 * time.Second
	defaultMaxAttempts       = 5
	defaultBackoffBase       = 10 * time.Second
	defaultBackoffMax        = 10 * time.Minute
	defaultEventTTL          = 24 * time.Hour
	defaultMaxWindow         = 72 * time.Hour
	listPageSize             = 500
	llmRetryBackoffBase      = 2
	finalizeTimeout          = 10 * time.Second
)

// EventWorkerPool implements REQ-01..04 and REQ-08: N goroutines that each pull one
// event at a time via ClaimOne (FOR UPDATE SKIP LOCKED), run the FT-005 domain
// handlers, and finalize under a guarded lease. A per-event heartbeat goroutine
// extends the lease while the handler runs.
type EventWorkerPool struct {
	outboxRepo  biz.SummaryOutboxRepo
	postRepo    biz.PostRepo
	summaryRepo biz.SummaryRepo
	llmProvider biz.LLMProvider
	cfg         *conf.Summary
	log         *slog.Logger

	processID string
	policy    biz.RetryPolicy

	wg       sync.WaitGroup
	cancel   context.CancelFunc
	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewEventWorkerPool(
	outboxRepo biz.SummaryOutboxRepo,
	postRepo biz.PostRepo,
	summaryRepo biz.SummaryRepo,
	llmProvider biz.LLMProvider,
	cfg *conf.Summary,
	logger *slog.Logger,
) *EventWorkerPool {
	host, _ := os.Hostname()
	processID := fmt.Sprintf("%s-%d-%s", host, os.Getpid(), uuid.NewString()[:8])

	policy := biz.RetryPolicy{
		MaxAttempts: int(cfg.GetWorker().GetMaxAttempts()),
		BackoffBase: cfg.GetWorker().GetBackoffBase().AsDuration(),
		BackoffMax:  cfg.GetWorker().GetBackoffMax().AsDuration(),
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaultMaxAttempts
	}
	if policy.BackoffBase <= 0 {
		policy.BackoffBase = defaultBackoffBase
	}
	if policy.BackoffMax <= 0 {
		policy.BackoffMax = defaultBackoffMax
	}

	return &EventWorkerPool{
		outboxRepo:  outboxRepo,
		postRepo:    postRepo,
		summaryRepo: summaryRepo,
		llmProvider: llmProvider,
		cfg:         cfg,
		log:         logger,
		processID:   processID,
		policy:      policy,
		stopCh:      make(chan struct{}),
	}
}

func (p *EventWorkerPool) Start(ctx context.Context) error {
	// cancel is invoked from Stop() either on graceful-timeout expiry or when the
	// caller's context is already done; see Stop() below. The pool owns its
	// lifetime through stopCh + cancel, so this is not a leak.
	runCtx, cancel := context.WithCancel(ctx) //nolint:gosec // cancel is stored on the pool and invoked by Stop
	p.cancel = cancel

	concurrency := int(p.cfg.GetWorker().GetConcurrency())
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}

	for i := range concurrency {
		workerID := fmt.Sprintf("%s-%d", p.processID, i)
		p.wg.Go(func() { p.runWorker(runCtx, workerID) })
	}

	return nil
}

func (p *EventWorkerPool) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() { close(p.stopCh) })

	gracefulTimeout := p.cfg.GetWorker().GetGracefulTimeout().AsDuration()
	if gracefulTimeout <= 0 {
		gracefulTimeout = defaultGracefulTimeout
	}

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(gracefulTimeout):
		p.log.WarnContext(ctx, "event worker pool graceful timeout exceeded; cancelling", "timeout", gracefulTimeout)
		if p.cancel != nil {
			p.cancel()
		}
		<-done
	case <-ctx.Done():
		if p.cancel != nil {
			p.cancel()
		}
		<-done
		return ctx.Err()
	}
	return nil
}

func (p *EventWorkerPool) runWorker(ctx context.Context, workerID string) {
	pollInterval := p.cfg.GetWorker().GetPollInterval().AsDuration()
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	leaseTTL := p.cfg.GetWorker().GetLeaseTtl().AsDuration()
	if leaseTTL <= 0 {
		leaseTTL = defaultLeaseTTL
	}

	for {
		if !p.isRunning(ctx) {
			return
		}

		event, claimErr := p.outboxRepo.ClaimOne(ctx, workerID, leaseTTL)
		if claimErr != nil {
			if errors.Is(claimErr, biz.ErrNoEventAvailable) {
				if !p.sleep(ctx, pollInterval) {
					return
				}
				continue
			}
			p.log.ErrorContext(ctx, "claim event failed", "err", claimErr, "worker", workerID)
			if !p.sleep(ctx, pollInterval) {
				return
			}
			continue
		}

		p.processEvent(ctx, workerID, event)
	}
}

func (p *EventWorkerPool) isRunning(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-p.stopCh:
		return false
	default:
		return true
	}
}

func (p *EventWorkerPool) sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	case <-p.stopCh:
		return false
	}
}

func (p *EventWorkerPool) processEvent(parent context.Context, workerID string, event biz.SummaryEvent) {
	logger := p.log.With(
		"worker", workerID,
		"summary_event_id", event.ID,
		"event_type", event.EventType,
		"source_id", event.SourceID,
	)
	start := time.Now()

	eventTTL := p.cfg.GetOutbox().GetEventTtl().AsDuration()
	if eventTTL <= 0 {
		eventTTL = defaultEventTTL
	}
	if time.Since(event.CreatedAt) > eventTTL {
		logger.InfoContext(parent, "event expired", "age", time.Since(event.CreatedAt))
		finCtx, finCancel := context.WithTimeout(context.Background(), finalizeTimeout)
		defer finCancel()
		if err := p.outboxRepo.FinalizeWithLease(
			finCtx, event.ID, workerID, biz.SummaryEventStatusExpired, nil, nil,
		); err != nil && !errors.Is(err, biz.ErrLeaseLost) {
			logger.ErrorContext(parent, "finalize expired failed", "err", err)
		}
		return
	}

	// Per-event context, cancellable by heartbeat when lease is lost.
	processCtx, processCancel := context.WithCancel(parent)
	defer processCancel()

	heartbeatDone := p.startHeartbeat(processCtx, processCancel, workerID, event.ID)

	handlerErr := p.dispatch(processCtx, logger, workerID, event)

	processCancel()
	<-heartbeatDone

	p.finalize(parent, logger, workerID, event, handlerErr, start)
}

func (p *EventWorkerPool) dispatch(
	ctx context.Context,
	logger *slog.Logger,
	workerID string,
	event biz.SummaryEvent,
) error {
	switch event.EventType {
	case biz.SummaryEventTypeSummarizePost:
		return p.processSummarizePost(ctx, logger, workerID, event)
	case biz.SummaryEventTypeSummarizeSource:
		return p.processSummarizeSource(ctx, logger, workerID, event)
	default:
		return fmt.Errorf("unknown event type %q", event.EventType)
	}
}

func (p *EventWorkerPool) finalize(
	ctx context.Context,
	logger *slog.Logger,
	workerID string,
	event biz.SummaryEvent,
	handlerErr error,
	start time.Time,
) {
	if handlerErr == nil {
		logger.InfoContext(ctx, "event processed", "duration", time.Since(start), "status", "completed")
		return
	}

	if errors.Is(handlerErr, biz.ErrLeaseLost) {
		logger.WarnContext(ctx, "lease lost during processing; abandoning without finalize",
			"duration", time.Since(start), "err", handlerErr)
		return
	}

	// Use a fresh background-derived context so we can finalize even if the parent
	// context was cancelled (e.g. graceful shutdown).
	finCtx, cancel := context.WithTimeout(context.Background(), finalizeTimeout)
	defer cancel()

	// event.AttemptCount was incremented by ClaimOne, so it reflects the current
	// (this) attempt. Both backoff growth and the transient-vs-permanent check use
	// the real count so retries ramp up in time and exhausted events go to Failed
	// without an extra bounce through the reaper.
	if p.policy.ShouldRetry(handlerErr, event.AttemptCount) {
		backoff := p.policy.CalculateBackoff(event.AttemptCount)
		logger.WarnContext(ctx, "transient failure; scheduling retry",
			"attempt", event.AttemptCount, "retry_in", backoff,
			"duration", time.Since(start), "err", handlerErr)
		if err := p.outboxRepo.MarkForRetry(
			finCtx, event.ID, workerID, time.Now().Add(backoff), handlerErr.Error(),
		); err != nil && !errors.Is(err, biz.ErrLeaseLost) {
			logger.ErrorContext(ctx, "mark for retry failed", "err", err)
		}
		return
	}

	errText := handlerErr.Error()
	logger.WarnContext(ctx, "permanent failure", "duration", time.Since(start), "err", handlerErr)
	if err := p.outboxRepo.FinalizeWithLease(
		finCtx, event.ID, workerID, biz.SummaryEventStatusFailed, nil, &errText,
	); err != nil && !errors.Is(err, biz.ErrLeaseLost) {
		logger.ErrorContext(ctx, "finalize failed", "err", err)
	}
}

func (p *EventWorkerPool) startHeartbeat(
	ctx context.Context,
	cancelOnLoss context.CancelFunc,
	workerID, eventID string,
) <-chan struct{} {
	interval := p.cfg.GetWorker().GetHeartbeatInterval().AsDuration()
	if interval <= 0 {
		interval = defaultHeartbeatInterval
	}
	leaseTTL := p.cfg.GetWorker().GetLeaseTtl().AsDuration()
	if leaseTTL <= 0 {
		leaseTTL = defaultLeaseTTL
	}

	done := make(chan struct{})
	p.wg.Go(func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := p.outboxRepo.ExtendLease(ctx, eventID, workerID, leaseTTL); err != nil {
					if errors.Is(err, biz.ErrLeaseLost) {
						p.log.WarnContext(ctx, "heartbeat lost lease", "summary_event_id", eventID, "worker", workerID)
						cancelOnLoss()
						return
					}
					p.log.ErrorContext(ctx, "heartbeat failed", "summary_event_id", eventID, "err", err)
				}
			}
		}
	})
	return done
}

func (p *EventWorkerPool) processSummarizePost(
	ctx context.Context,
	logger *slog.Logger,
	workerID string,
	event biz.SummaryEvent,
) error {
	if event.PostID == nil {
		return fmt.Errorf("%w: post id is nil for summarize_post event", biz.ErrSummaryValidation)
	}

	// Idempotency (FM-05): if a summary for this post already exists, finalize as completed
	// without invoking the LLM. This prevents duplicates on recovery after lease expiry.
	existing, err := p.summaryRepo.ListByPost(ctx, *event.PostID)
	if err != nil {
		return fmt.Errorf("check existing summary: %w", err)
	}
	if len(existing) > 0 {
		summaryID := existing[0].ID
		return p.outboxRepo.FinalizeWithLease(
			ctx, event.ID, workerID, biz.SummaryEventStatusCompleted, &summaryID, nil,
		)
	}

	post, err := p.postRepo.Get(ctx, *event.PostID)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}

	summaryText, err := p.summarizeWithRetry(ctx, logger, post.Text)
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
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

	saved, saveErr := p.summaryRepo.Save(ctx, sm)
	if saveErr != nil {
		return fmt.Errorf("save summary: %w", saveErr)
	}

	summaryID := saved.ID
	return p.outboxRepo.FinalizeWithLease(
		ctx, event.ID, workerID, biz.SummaryEventStatusCompleted, &summaryID, nil,
	)
}

func (p *EventWorkerPool) processSummarizeSource(
	ctx context.Context,
	logger *slog.Logger,
	workerID string,
	event biz.SummaryEvent,
) error {
	maxWindow := p.cfg.GetCumulative().GetMaxWindow().AsDuration()
	if maxWindow <= 0 {
		maxWindow = defaultMaxWindow
	}
	maxInputChars := int(p.cfg.GetCumulative().GetMaxInputChars())
	if maxInputChars <= 0 {
		maxInputChars = 50000
	}

	lastSummary, err := p.summaryRepo.GetLastBySource(ctx, event.SourceID)
	if err != nil {
		return fmt.Errorf("get last summary: %w", err)
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
		postsResult, listErr := p.postRepo.List(ctx, biz.ListPostsFilter{
			SourceID:      event.SourceID,
			PageSize:      listPageSize,
			PageToken:     pageToken,
			OrderBy:       biz.SortByCreatedAt,
			OrderDir:      biz.SortAsc,
			CreatedAfter:  &windowStart,
			CreatedBefore: &now,
		})
		if listErr != nil {
			return fmt.Errorf("list posts: %w", listErr)
		}
		for _, post := range postsResult.Items {
			foundPosts = true
			sb.WriteString(post.Text)
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
		return p.outboxRepo.FinalizeWithLease(
			ctx, event.ID, workerID, biz.SummaryEventStatusCompleted, nil, nil,
		)
	}

	concat := sb.String()
	if len(concat) > maxInputChars {
		return fmt.Errorf("%w: input text exceeds max_input_chars limit", biz.ErrSummaryValidation)
	}

	summaryText, err := p.summarizeWithRetry(ctx, logger, concat)
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
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

	saved, saveErr := p.summaryRepo.Save(ctx, sm)
	if saveErr != nil {
		return fmt.Errorf("save summary: %w", saveErr)
	}

	summaryID := saved.ID
	return p.outboxRepo.FinalizeWithLease(
		ctx, event.ID, workerID, biz.SummaryEventStatusCompleted, &summaryID, nil,
	)
}

// summarizeWithRetry preserves the FT-005 LLM retry contract (NS-04): up to
// cfg.llm.max_retries in-call attempts with exponential backoff between them.
// The outer FT-007 retry loop kicks in only if all LLM attempts fail.
func (p *EventWorkerPool) summarizeWithRetry(
	ctx context.Context,
	logger *slog.Logger,
	text string,
) (string, error) {
	maxRetries := int(p.cfg.GetLlm().GetMaxRetries())
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := range maxRetries {
		result, err := p.llmProvider.Summarize(ctx, text)
		if err != nil {
			lastErr = err
			logger.WarnContext(ctx, "llm attempt failed", "attempt", attempt+1, "err", err)
			backoff := time.Duration(math.Pow(llmRetryBackoffBase, float64(attempt))) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			continue
		}
		if strings.TrimSpace(result) == "" {
			lastErr = fmt.Errorf("llm returned empty summary on attempt %d", attempt+1)
			continue
		}
		return result, nil
	}
	return "", fmt.Errorf("all %d attempts failed: %w", maxRetries, lastErr)
}
