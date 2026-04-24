package task

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/4itosik/feedium/internal/conf"
)

const defaultSourceSchedulerPoll = 5 * time.Second

// SourceScheduler is the task-level view of biz.SummaryUsecase.ScheduleNextCumulative.
type SourceScheduler interface {
	ScheduleNextCumulative(ctx context.Context) (bool, error)
}

// SourceDueScheduler implements REQ-06: per-source due-time planning. On each
// tick it delegates to the usecase, which atomically claims one due source
// (FOR UPDATE SKIP LOCKED), bumps next_summary_at, and enqueues an event when
// new posts exist. Replaces the in-process CronWorker ticker from FT-005.
type SourceDueScheduler struct {
	scheduler SourceScheduler
	cfg       *conf.Summary
	log       *slog.Logger

	done     chan struct{}
	doneOnce sync.Once
}

func NewSourceDueScheduler(scheduler SourceScheduler, cfg *conf.Summary, logger *slog.Logger) *SourceDueScheduler {
	return &SourceDueScheduler{
		scheduler: scheduler,
		cfg:       cfg,
		log:       logger,
		done:      make(chan struct{}),
	}
}

func (s *SourceDueScheduler) Start(ctx context.Context) error {
	pollInterval := s.cfg.GetSourceScheduler().GetPollInterval().AsDuration()
	if pollInterval <= 0 {
		pollInterval = defaultSourceSchedulerPoll
	}
	go s.run(ctx, pollInterval)
	return nil
}

func (s *SourceDueScheduler) Stop(_ context.Context) error {
	s.doneOnce.Do(func() { close(s.done) })
	return nil
}

func (s *SourceDueScheduler) run(ctx context.Context, pollInterval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		s.tick(ctx)

		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return
		case <-s.done:
			return
		}
	}
}

// tick drains all currently-due sources, one per usecase call. The usecase
// returns scheduled=false either when nothing is due or when the claimed source
// has no new posts — both break us out of the drain loop.
func (s *SourceDueScheduler) tick(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		scheduled, err := s.scheduler.ScheduleNextCumulative(ctx)
		if err != nil {
			s.log.ErrorContext(ctx, "source scheduler tick failed", "err", err)
			return
		}
		if !scheduled {
			return
		}
	}
}
