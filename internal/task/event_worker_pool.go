// Package task implements background workers for FT-007 scalable event processing.
// See memory-bank/features/FT-007-scalable-event-processing/feature.md for the canonical
// requirements (REQ-01..09) and architectural invariants.
//
// This layer is intentionally thin: per architecture.md, task/ owns lifecycle
// (claim, heartbeat, lease-guarded finalize, retry decision, graceful shutdown)
// and delegates all business logic to biz usecases.
package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	finalizeTimeout          = 10 * time.Second
)

// EventProcessor is the task-level view of biz.SummaryUsecase: pure domain
// execution with no lease / retry awareness.
type EventProcessor interface {
	ProcessPostEvent(ctx context.Context, event biz.SummaryEvent) (string, error)
	ProcessSourceEvent(ctx context.Context, event biz.SummaryEvent) (string, error)
	RetryPolicy() biz.RetryPolicy
}

// EventWorkerPool implements REQ-01..04 and REQ-08: N goroutines that each pull
// one event at a time via ClaimOne (FOR UPDATE SKIP LOCKED), delegate execution
// to the usecase, and finalize under a guarded lease. A per-event heartbeat
// goroutine extends the lease while the handler runs.
type EventWorkerPool struct {
	outboxRepo biz.SummaryOutboxRepo
	processor  EventProcessor
	cfg        *conf.Summary
	log        *slog.Logger

	processID string
	policy    biz.RetryPolicy

	wg       sync.WaitGroup
	cancel   context.CancelFunc
	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewEventWorkerPool(
	outboxRepo biz.SummaryOutboxRepo,
	processor EventProcessor,
	cfg *conf.Summary,
	logger *slog.Logger,
) *EventWorkerPool {
	host, _ := os.Hostname()
	processID := fmt.Sprintf("%s-%d-%s", host, os.Getpid(), uuid.NewString()[:8])

	return &EventWorkerPool{
		outboxRepo: outboxRepo,
		processor:  processor,
		cfg:        cfg,
		log:        logger,
		processID:  processID,
		policy:     processor.RetryPolicy(),
		stopCh:     make(chan struct{}),
	}
}

func (p *EventWorkerPool) Start(ctx context.Context) error {
	// cancel is invoked from Stop() on graceful-timeout expiry or when the caller's
	// context is done; see Stop() below. Lifetime is owned by stopCh + cancel.
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

	// Per-event context, cancellable by heartbeat when lease is lost.
	processCtx, processCancel := context.WithCancel(parent)
	defer processCancel()

	heartbeatDone := p.startHeartbeat(processCtx, processCancel, workerID, event.ID)

	summaryID, handlerErr := p.dispatch(processCtx, event)

	processCancel()
	<-heartbeatDone

	p.finalize(parent, logger, workerID, event, summaryID, handlerErr, start)
}

func (p *EventWorkerPool) dispatch(ctx context.Context, event biz.SummaryEvent) (string, error) {
	switch event.EventType {
	case biz.SummaryEventTypeSummarizePost:
		return p.processor.ProcessPostEvent(ctx, event)
	case biz.SummaryEventTypeSummarizeSource:
		return p.processor.ProcessSourceEvent(ctx, event)
	default:
		return "", fmt.Errorf("unknown event type %q", event.EventType)
	}
}

func (p *EventWorkerPool) finalize(
	ctx context.Context,
	logger *slog.Logger,
	workerID string,
	event biz.SummaryEvent,
	summaryID string,
	handlerErr error,
	start time.Time,
) {
	if handlerErr == nil {
		logger.InfoContext(ctx, "event processed", "duration", time.Since(start), "status", "completed")
		var summaryIDPtr *string
		if summaryID != "" {
			summaryIDPtr = &summaryID
		}
		p.writeTerminal(ctx, logger, workerID, event.ID, biz.SummaryEventStatusCompleted, summaryIDPtr, nil)
		return
	}

	if errors.Is(handlerErr, biz.ErrLeaseLost) {
		logger.WarnContext(ctx, "lease lost during processing; abandoning without finalize",
			"duration", time.Since(start), "err", handlerErr)
		return
	}

	// Handler interrupted by context cancellation (graceful-timeout expiry, parent
	// shutdown, or heartbeat lease-loss). Abandoning aligns with FM-01: lease will
	// expire, another worker re-claims via the expired-lease path.
	if errors.Is(handlerErr, context.Canceled) || errors.Is(handlerErr, context.DeadlineExceeded) {
		logger.WarnContext(ctx, "handler cancelled; abandoning for lease expiry",
			"duration", time.Since(start), "err", handlerErr)
		return
	}

	if errors.Is(handlerErr, biz.ErrSummaryEventExpired) {
		logger.InfoContext(ctx, "event expired; finalizing",
			"duration", time.Since(start), "err", handlerErr)
		p.writeTerminal(ctx, logger, workerID, event.ID, biz.SummaryEventStatusExpired, nil, nil)
		return
	}

	if errors.Is(handlerErr, biz.ErrSummarySourceNoPosts) {
		logger.InfoContext(ctx, "source has no new posts; finalizing",
			"duration", time.Since(start))
		p.writeTerminal(ctx, logger, workerID, event.ID, biz.SummaryEventStatusCompleted, nil, nil)
		return
	}

	// event.AttemptCount was incremented by ClaimOne, so it reflects the current
	// (this) attempt. Both backoff growth and the transient-vs-permanent check use
	// the real count so retries ramp up in time and exhausted events go to Failed
	// without an extra bounce through the reaper.
	if p.policy.ShouldRetry(handlerErr, event.AttemptCount) {
		backoff := p.policy.CalculateBackoff(event.AttemptCount)
		logger.WarnContext(ctx, "transient failure; scheduling retry",
			"attempt", event.AttemptCount, "retry_in", backoff,
			"duration", time.Since(start), "err", handlerErr)
		finCtx, cancel := context.WithTimeout(context.Background(), finalizeTimeout)
		defer cancel()
		if err := p.outboxRepo.MarkForRetry(
			finCtx, event.ID, workerID, time.Now().Add(backoff), handlerErr.Error(),
		); err != nil && !errors.Is(err, biz.ErrLeaseLost) {
			logger.ErrorContext(ctx, "mark for retry failed", "err", err)
		}
		return
	}

	errText := handlerErr.Error()
	logger.WarnContext(ctx, "permanent failure", "duration", time.Since(start), "err", handlerErr)
	p.writeTerminal(ctx, logger, workerID, event.ID, biz.SummaryEventStatusFailed, nil, &errText)
}

func (p *EventWorkerPool) writeTerminal(
	ctx context.Context,
	logger *slog.Logger,
	workerID, eventID string,
	status biz.SummaryEventStatus,
	summaryID *string,
	errText *string,
) {
	// Fresh background-derived context so finalization survives parent cancellation
	// on normal completion (the caller already ruled out context-cancelled errors).
	finCtx, cancel := context.WithTimeout(context.Background(), finalizeTimeout)
	defer cancel()
	if err := p.outboxRepo.FinalizeWithLease(finCtx, eventID, workerID, status, summaryID, errText); err != nil &&
		!errors.Is(err, biz.ErrLeaseLost) {
		logger.ErrorContext(ctx, "finalize failed", "err", err, "status", status)
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
