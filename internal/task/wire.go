package task

import (
	"github.com/google/wire"
)

// ProviderSet wires the FT-007 workers: an event worker pool for pull-based
// processing, a per-source scheduler for cumulative sources, and a reaper that
// terminates events whose attempt budget is exhausted.
var ProviderSet = wire.NewSet( //nolint:gochecknoglobals // wire dependency injection
	NewEventWorkerPool,
	NewSourceDueScheduler,
	NewStuckEventReaper,
)
