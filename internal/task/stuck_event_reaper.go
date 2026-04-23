package task

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

const (
	defaultReaperGrace = 30 * time.Second
	reaperBatchLimit   = 100
)

// StuckEventReaper implements REQ-07: periodically inspects events whose lease
// expired (status='processing' AND locked_until < now() - grace). If attempt_count
// has reached max_attempts, the event is terminally marked failed with a
// 'max attempts exceeded' error. Otherwise the reaper just logs — a worker's
// claim-loop will pick the event back up via ClaimOne's expired-lease path.
type StuckEventReaper struct {
	outboxRepo biz.SummaryOutboxRepo
	cfg        *conf.Summary
	log        *slog.Logger

	done     chan struct{}
	doneOnce sync.Once
}

func NewStuckEventReaper(
	outboxRepo biz.SummaryOutboxRepo,
	cfg *conf.Summary,
	logger *slog.Logger,
) *StuckEventReaper {
	return &StuckEventReaper{
		outboxRepo: outboxRepo,
		cfg:        cfg,
		log:        logger,
		done:       make(chan struct{}),
	}
}

func (r *StuckEventReaper) Start(ctx context.Context) error {
	interval := r.cfg.GetReaper().GetInterval().AsDuration()
	if interval <= 0 {
		interval = time.Minute
	}
	go r.run(ctx, interval)
	return nil
}

func (r *StuckEventReaper) Stop(_ context.Context) error {
	r.doneOnce.Do(func() { close(r.done) })
	return nil
}

func (r *StuckEventReaper) run(ctx context.Context, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.done:
			return
		default:
		}

		r.tick(ctx)

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return
		case <-r.done:
			return
		}
	}
}

func (r *StuckEventReaper) tick(ctx context.Context) {
	grace := r.cfg.GetReaper().GetGrace().AsDuration()
	if grace <= 0 {
		grace = defaultReaperGrace
	}

	maxAttempts := int(r.cfg.GetWorker().GetMaxAttempts())
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	events, err := r.outboxRepo.ListLeaseExpired(ctx, grace, reaperBatchLimit)
	if err != nil {
		r.log.ErrorContext(ctx, "list lease expired failed", "err", err)
		return
	}

	for _, ev := range events {
		if ev.AttemptCount >= maxAttempts {
			errText := "max attempts exceeded"
			if termErr := r.outboxRepo.UpdateStatus(
				ctx, ev.ID, biz.SummaryEventStatusFailed, nil, &errText,
			); termErr != nil {
				r.log.ErrorContext(ctx, "terminal update failed",
					"summary_event_id", ev.ID, "err", termErr)
				continue
			}
			r.log.WarnContext(ctx, "stuck event terminally failed",
				"summary_event_id", ev.ID, "attempt_count", ev.AttemptCount)
			continue
		}
		r.log.WarnContext(ctx, "stuck event detected; awaiting re-claim",
			"summary_event_id", ev.ID, "attempt_count", ev.AttemptCount, "max_attempts", maxAttempts)
	}
}
