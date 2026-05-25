package task

import (
	"github.com/google/wire"
)

// ProviderSet wires the FT-007 workers (event worker pool, per-source
// scheduler, stuck-event reaper) and the FT-009 content collectors (RSS, web).
// The Telegram collector is intentionally absent — see implement.md: it
// depends on a Telegram client library whose adoption (DEC-03) requires human
// approval and is escalated, not finalized here.
var ProviderSet = wire.NewSet( //nolint:gochecknoglobals // wire dependency injection
	NewEventWorkerPool,
	NewSourceDueScheduler,
	NewStuckEventReaper,
	NewRSSCollector,
	NewWebCollector,
)
