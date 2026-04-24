package task

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/4itosik/feedium/internal/conf"
)

const defaultReaperInterval = time.Minute

// StuckReaper is the task-level view of biz.SummaryUsecase.ReapStuckEvents.
type StuckReaper interface {
	ReapStuckEvents(ctx context.Context) (int, error)
}

// StuckEventReaper implements REQ-07: periodically asks the usecase to terminate
// events whose lease expired past grace AND attempt_count reached the retry
// budget. The usecase uses a guarded UPDATE (see FailExpired) so concurrent
// claims are not stomped. This worker owns only the ticking lifecycle.
type StuckEventReaper struct {
	reaper StuckReaper
	cfg    *conf.Summary
	log    *slog.Logger

	done     chan struct{}
	doneOnce sync.Once
}

func NewStuckEventReaper(reaper StuckReaper, cfg *conf.Summary, logger *slog.Logger) *StuckEventReaper {
	return &StuckEventReaper{
		reaper: reaper,
		cfg:    cfg,
		log:    logger,
		done:   make(chan struct{}),
	}
}

func (r *StuckEventReaper) Start(ctx context.Context) error {
	interval := r.cfg.GetReaper().GetInterval().AsDuration()
	if interval <= 0 {
		interval = defaultReaperInterval
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

		terminated, err := r.reaper.ReapStuckEvents(ctx)
		if err != nil {
			r.log.ErrorContext(ctx, "reap stuck events failed", "err", err)
		} else if terminated > 0 {
			r.log.WarnContext(ctx, "stuck events terminally failed", "count", terminated)
		}

		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return
		case <-r.done:
			return
		}
	}
}
