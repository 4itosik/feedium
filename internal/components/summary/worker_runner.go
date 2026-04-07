// Package summary provides components for the summary processing subsystem.
package summary

import (
	"context"
	"log/slog"
	"time"

	"feedium/internal/app/summary"
)

// WorkerRunner manages the lifecycle of a summary worker polling loop.
type WorkerRunner struct {
	worker *summary.Worker
	logger *slog.Logger
}

// NewWorkerRunner creates a new WorkerRunner instance.
func NewWorkerRunner(worker *summary.Worker, logger *slog.Logger) *WorkerRunner {
	return &WorkerRunner{
		worker: worker,
		logger: logger,
	}
}

// Start runs the polling loop until the context is cancelled.
// The loop polls every 1 second for pending events.
// Errors are logged but do not stop the polling loop.
// When ctx is cancelled, the loop gracefully exits after completing the current iteration.
func (r *WorkerRunner) Start(ctx context.Context) {
	const pollInterval = 1 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	logger := r.logger.With("component", "worker_runner")
	logger.InfoContext(ctx, "starting worker polling loop")

	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "stopping worker polling loop")
			return
		case <-ticker.C:
			_, err := r.worker.ProcessNext(ctx)
			if err != nil {
				// Log error but continue polling - this is likely a transient DB error
				logger.ErrorContext(ctx, "worker processing error", "error", err)
			}
		}
	}
}
