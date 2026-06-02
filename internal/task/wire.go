package task

import (
	"github.com/google/wire"
)

// ProviderSet wires the FT-007 workers (event worker pool, per-source
// scheduler, stuck-event reaper) and the FT-009 content collectors (RSS, web,
// Telegram). The Telegram collector consumes a push session (gotd updates.Manager,
// DEC-08); its live connection is gated by AG-02.
var ProviderSet = wire.NewSet( //nolint:gochecknoglobals // wire dependency injection
	NewEventWorkerPool,
	NewSourceDueScheduler,
	NewStuckEventReaper,
	NewScoreEventWorkerPool,
	NewScoreDueScheduler,
	NewStuckScoreEventReaper,
	NewRSSCollector,
	NewWebCollector,
	NewTelegramCollector,
)
