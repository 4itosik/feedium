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

// ScoreProcessor is the task-level view of biz.ScoreUsecase: pure scoring
// execution with no lease/retry awareness.
type ScoreProcessor interface {
	ScorePost(ctx context.Context, event biz.ScoreEvent) (string, error)
	RetryPolicy() biz.RetryPolicy
}

// ScoreEventWorkerPool implements the FT-010 auto-scoring worker (REQ-02, REQ-05,
// REQ-06): N goroutines each pull one score_event via ClaimOne (FOR UPDATE SKIP
// LOCKED), delegate to the usecase, and finalize under a guarded lease. A
// per-event heartbeat extends the lease while scoring runs. Structural copy of
// the FT-007 EventWorkerPool restricted to scoring (DEC-05).
type ScoreEventWorkerPool struct {
	outboxRepo biz.ScoreOutboxRepo
	processor  ScoreProcessor
	cfg        *conf.Scoring
	log        *slog.Logger

	processID string
	policy    biz.RetryPolicy

	wg       sync.WaitGroup
	cancel   context.CancelFunc
	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewScoreEventWorkerPool(
	outboxRepo biz.ScoreOutboxRepo,
	processor ScoreProcessor,
	cfg *conf.Scoring,
	logger *slog.Logger,
) *ScoreEventWorkerPool {
	host, _ := os.Hostname()
	processID := fmt.Sprintf("score-%s-%d-%s", host, os.Getpid(), uuid.NewString()[:8])

	return &ScoreEventWorkerPool{
		outboxRepo: outboxRepo,
		processor:  processor,
		cfg:        cfg,
		log:        logger,
		processID:  processID,
		policy:     processor.RetryPolicy(),
		stopCh:     make(chan struct{}),
	}
}

func (p *ScoreEventWorkerPool) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
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

func (p *ScoreEventWorkerPool) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() { close(p.stopCh) })
	return p.awaitShutdown(ctx, p.gracefulTimeout())
}

func (p *ScoreEventWorkerPool) gracefulTimeout() time.Duration {
	d := p.cfg.GetWorker().GetGracefulTimeout().AsDuration()
	if d <= 0 {
		return defaultGracefulTimeout
	}
	return d
}

// awaitShutdown waits for all workers to drain, forcing cancellation if the
// graceful window elapses or the caller's context is cancelled first.
func (p *ScoreEventWorkerPool) awaitShutdown(ctx context.Context, graceful time.Duration) error {
	drained := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		return nil
	case <-time.After(graceful):
		p.log.WarnContext(ctx, "score worker pool graceful timeout exceeded; cancelling", "timeout", graceful)
		p.forceCancel()
		<-drained
		return nil
	case <-ctx.Done():
		p.forceCancel()
		<-drained
		return ctx.Err()
	}
}

func (p *ScoreEventWorkerPool) forceCancel() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *ScoreEventWorkerPool) runWorker(ctx context.Context, workerID string) {
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
			if errors.Is(claimErr, biz.ErrNoScoreEventAvailable) {
				if !p.sleep(ctx, pollInterval) {
					return
				}
				continue
			}
			p.log.ErrorContext(ctx, "claim score event failed", "err", claimErr, "worker", workerID)
			if !p.sleep(ctx, pollInterval) {
				return
			}
			continue
		}
		p.processEvent(ctx, workerID, event)
	}
}

func (p *ScoreEventWorkerPool) isRunning(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-p.stopCh:
		return false
	default:
		return true
	}
}

func (p *ScoreEventWorkerPool) sleep(ctx context.Context, d time.Duration) bool {
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

func (p *ScoreEventWorkerPool) processEvent(parent context.Context, workerID string, event biz.ScoreEvent) {
	logger := p.log.With("worker", workerID, "score_event_id", event.ID, "post_id", event.PostID)
	start := time.Now()

	processCtx, processCancel := context.WithCancel(parent)
	defer processCancel()

	heartbeatDone := p.startHeartbeat(processCtx, processCancel, workerID, event.ID)

	_, handlerErr := p.processor.ScorePost(processCtx, event)

	processCancel()
	<-heartbeatDone

	p.finalize(parent, logger, workerID, event, handlerErr, start)
}

func (p *ScoreEventWorkerPool) finalize(
	ctx context.Context,
	logger *slog.Logger,
	workerID string,
	event biz.ScoreEvent,
	handlerErr error,
	start time.Time,
) {
	if handlerErr == nil {
		logger.InfoContext(ctx, "score completed", "duration", time.Since(start))
		p.writeTerminal(ctx, logger, workerID, event.ID, biz.ScoreEventStatusCompleted, nil)
		return
	}

	if errors.Is(handlerErr, biz.ErrLeaseLost) {
		logger.WarnContext(ctx, "lease lost during scoring; abandoning", "err", handlerErr)
		return
	}
	if errors.Is(handlerErr, context.Canceled) || errors.Is(handlerErr, context.DeadlineExceeded) {
		logger.WarnContext(ctx, "scoring cancelled; abandoning for lease expiry", "err", handlerErr)
		return
	}

	// AttemptCount was incremented by ClaimOne, so it reflects the current attempt.
	if biz.IsTransientScoreError(handlerErr) && event.AttemptCount < p.policy.MaxAttempts {
		backoff := p.policy.CalculateBackoff(event.AttemptCount)
		logger.WarnContext(ctx, "transient scoring failure; scheduling retry",
			"attempt", event.AttemptCount, "retry_in", backoff, "err", handlerErr)
		finCtx, cancel := context.WithTimeout(context.Background(), finalizeTimeout)
		defer cancel()
		if err := p.outboxRepo.MarkForRetry(
			finCtx, event.ID, workerID, time.Now().Add(backoff), handlerErr.Error(),
		); err != nil && !errors.Is(err, biz.ErrLeaseLost) {
			logger.ErrorContext(ctx, "mark score event for retry failed", "err", err)
		}
		return
	}

	errText := handlerErr.Error()
	logger.WarnContext(ctx, "permanent scoring failure", "err", handlerErr)
	p.writeTerminal(ctx, logger, workerID, event.ID, biz.ScoreEventStatusFailed, &errText)
}

func (p *ScoreEventWorkerPool) writeTerminal(
	ctx context.Context,
	logger *slog.Logger,
	workerID, eventID string,
	status biz.ScoreEventStatus,
	errText *string,
) {
	finCtx, cancel := context.WithTimeout(context.Background(), finalizeTimeout)
	defer cancel()
	if err := p.outboxRepo.FinalizeWithLease(finCtx, eventID, workerID, status, errText); err != nil &&
		!errors.Is(err, biz.ErrLeaseLost) {
		logger.ErrorContext(ctx, "finalize score event failed", "err", err, "status", status)
	}
}

func (p *ScoreEventWorkerPool) startHeartbeat(
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
						p.log.WarnContext(ctx, "score heartbeat lost lease", "score_event_id", eventID)
						cancelOnLoss()
						return
					}
					p.log.ErrorContext(ctx, "score heartbeat failed", "score_event_id", eventID, "err", err)
				}
			}
		}
	})
	return done
}
