package task

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/4itosik/feedium/internal/conf"
)

const defaultScoreSchedulerPoll = 5 * time.Second

// ScoreScheduler is the task-level view of biz.ScoreUsecase.EnqueueDue.
type ScoreScheduler interface {
	EnqueueDue(ctx context.Context) (int, error)
}

// ScoreDueScheduler implements REQ-02: on each tick it enqueues scoring events
// for summarized-unscored posts. Structural copy of SourceDueScheduler (DEC-05).
type ScoreDueScheduler struct {
	scheduler ScoreScheduler
	cfg       *conf.Scoring
	log       *slog.Logger

	done     chan struct{}
	doneOnce sync.Once
}

func NewScoreDueScheduler(scheduler ScoreScheduler, cfg *conf.Scoring, logger *slog.Logger) *ScoreDueScheduler {
	return &ScoreDueScheduler{
		scheduler: scheduler,
		cfg:       cfg,
		log:       logger,
		done:      make(chan struct{}),
	}
}

func (s *ScoreDueScheduler) Start(ctx context.Context) error {
	pollInterval := s.cfg.GetScheduler().GetPollInterval().AsDuration()
	if pollInterval <= 0 {
		pollInterval = defaultScoreSchedulerPoll
	}
	go s.run(ctx, pollInterval)
	return nil
}

func (s *ScoreDueScheduler) Stop(_ context.Context) error {
	s.doneOnce.Do(func() { close(s.done) })
	return nil
}

func (s *ScoreDueScheduler) run(ctx context.Context, pollInterval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		if n, err := s.scheduler.EnqueueDue(ctx); err != nil {
			s.log.ErrorContext(ctx, "score scheduler tick failed", "err", err)
		} else if n > 0 {
			s.log.InfoContext(ctx, "scoring events enqueued", "count", n)
		}

		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return
		case <-s.done:
			return
		}
	}
}
