package summary

import (
	"context"
	"log/slog"
	"time"

	"feedium/internal/app/source"
)

// Scheduler creates scheduled outbox events for TELEGRAM_GROUP sources at fixed times.
type Scheduler struct {
	outboxEventRepo OutboxEventRepository
	log             *slog.Logger
}

// NewScheduler creates a new scheduler instance.
func NewScheduler(outboxEventRepo OutboxEventRepository, log *slog.Logger) *Scheduler {
	return &Scheduler{
		outboxEventRepo: outboxEventRepo,
		log:             log,
	}
}

// RunScheduled creates scheduled outbox events for TELEGRAM_GROUP sources.
func (s *Scheduler) RunScheduled(ctx context.Context) error {
	count, err := s.outboxEventRepo.CreateScheduledForType(ctx, source.TypeTelegramGroup, time.Now())
	if err != nil {
		s.log.ErrorContext(ctx, "failed to create scheduled events", "error", err)
		return err
	}

	s.log.InfoContext(ctx, "created scheduled events", "count", count)
	return nil
}

// Start runs the scheduler in a separate goroutine, triggered at fixed times (00:00 and 12:00 UTC).
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		for {
			// Calculate time until next schedule point
			nextTime := nextScheduleTime(time.Now().UTC())
			duration := time.Until(nextTime)

			s.log.InfoContext(ctx, "scheduler waiting for next run", "next_run", nextTime, "duration", duration)

			// Wait until next schedule point
			timer := time.NewTimer(duration)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				// Execute scheduled task
				if err := s.RunScheduled(ctx); err != nil {
					s.log.ErrorContext(ctx, "scheduler run failed", "error", err)
				}
			}
		}
	}()
}

// nextScheduleTime returns the next scheduled time (00:00 or 12:00 UTC).
func nextScheduleTime(now time.Time) time.Time {
	// Convert to UTC if not already
	now = now.UTC()

	// Schedule times: 00:00 and 12:00 UTC
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	noon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)

	// Check if next midnight is closer than noon
	nextMidnight := midnight.Add(24 * time.Hour)

	if now.Before(midnight) {
		return midnight
	}
	if now.Before(noon) {
		return noon
	}
	// After noon, next is midnight tomorrow
	return nextMidnight
}
