package task

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/4itosik/feedium/internal/conf"
)

const defaultScoreReaperInterval = time.Minute

// ScoreReaper is the task-level view of biz.ScoreUsecase.ReapStuckScoreEvents.
type ScoreReaper interface {
	ReapStuckScoreEvents(ctx context.Context) (int, error)
}

// StuckScoreEventReaper implements REQ-06: periodically terminates score events
// whose lease expired past grace AND attempt_count reached the retry budget, so a
// post never hangs in scoring. Structural copy of StuckEventReaper (DEC-05).
type StuckScoreEventReaper struct {
	reaper ScoreReaper
	cfg    *conf.Scoring
	log    *slog.Logger

	done     chan struct{}
	doneOnce sync.Once
}

func NewStuckScoreEventReaper(reaper ScoreReaper, cfg *conf.Scoring, logger *slog.Logger) *StuckScoreEventReaper {
	return &StuckScoreEventReaper{
		reaper: reaper,
		cfg:    cfg,
		log:    logger,
		done:   make(chan struct{}),
	}
}

func (r *StuckScoreEventReaper) Start(ctx context.Context) error {
	interval := r.cfg.GetReaper().GetInterval().AsDuration()
	if interval <= 0 {
		interval = defaultScoreReaperInterval
	}
	go r.run(ctx, interval)
	return nil
}

func (r *StuckScoreEventReaper) Stop(_ context.Context) error {
	r.doneOnce.Do(func() { close(r.done) })
	return nil
}

func (r *StuckScoreEventReaper) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		r.reapOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-r.done:
			return
		case <-ticker.C:
		}
	}
}

// reapOnce runs a single reaping pass and logs the outcome. Kept separate from
// the ticking loop so the lifecycle and the work are independently readable.
func (r *StuckScoreEventReaper) reapOnce(ctx context.Context) {
	terminated, err := r.reaper.ReapStuckScoreEvents(ctx)
	if err != nil {
		r.log.ErrorContext(ctx, "reap stuck score events failed", "err", err)
		return
	}
	if terminated > 0 {
		r.log.WarnContext(ctx, "stuck score events terminally failed", "count", terminated)
	}
}
