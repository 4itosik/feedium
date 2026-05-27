package task

import (
	"context"
	"log/slog"
	"sync"

	"github.com/4itosik/feedium/internal/biz"
)

// ItemCollector is the task-level view of biz.CollectUsecase for push delivery:
// it persists items already obtained (CollectItems), reusing dedup/outbox/text
// filtering. The Telegram collector never lists or fetches — messages arrive via
// push (DEC-08).
type ItemCollector interface {
	CollectItems(ctx context.Context, source biz.Source, items []biz.CollectedItem) error
}

// TelegramCollector is a transport.Server that ingests Telegram posts in real
// time via a userbot push session (FT-009 REQ-02/REQ-03, DEC-08). Start runs the
// session in one goroutine; each new message is routed to its configured source
// by TgID and persisted via ItemCollector.CollectItems. An unknown peer (the
// userbot is in other dialogs) is ignored; a persist error is logged and never
// stops the stream (REQ-07, FM-T01). Stop cancels the session and waits.
//
// Push model (vs RSS/web polling) and its rationale: see
// memory-bank/features/FT-009-collectors/feature.md (DEC-08) and
// 2026-05-26-telegram-push-design.md.
type TelegramCollector struct {
	collector ItemCollector
	session   biz.TelegramSession
	sources   SourceLister
	log       *slog.Logger

	cancel    context.CancelFunc
	startOnce sync.Once
	wg        sync.WaitGroup
}

func NewTelegramCollector(
	collector ItemCollector,
	session biz.TelegramSession,
	sources SourceLister,
	logger *slog.Logger,
) *TelegramCollector {
	return &TelegramCollector{
		collector: collector,
		session:   session,
		sources:   sources,
		log:       logger,
	}
}

// Start launches the push session goroutine. The live gotd connection/auth lives
// in the data/ session impl and is gated by AG-02; tests inject a fake session.
// startOnce makes a second Start a no-op (kratos calls Start once), so the first
// session's cancel is never orphaned. Stop drains the goroutine via wg.Wait.
// The session derives from the Start context (consistent with EventWorkerPool) so
// a base-context cancellation also tears the session down even outside Stop.
func (c *TelegramCollector) Start(ctx context.Context) error {
	c.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		c.cancel = cancel
		c.wg.Go(func() {
			if err := c.session.Run(runCtx, c.handle); err != nil && runCtx.Err() == nil {
				c.log.Error("telegram collector: session ended", "err", err)
			}
		})
	})
	return nil
}

// Stop cancels the session and waits for the goroutine to drain (graceful).
func (c *TelegramCollector) Stop(_ context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	return nil
}

// handle routes one message to its configured source and persists it. Unknown
// peer → ignored; persist error → logged, stream continues (FM-T01).
func (c *TelegramCollector) handle(ctx context.Context, msg biz.TelegramMessage) error {
	src, ok, err := c.resolve(ctx, msg.TgID)
	if err != nil {
		c.log.ErrorContext(ctx, "telegram collector: resolve source failed", "tg_id", msg.TgID, "err", err)
		return nil
	}
	if !ok {
		return nil
	}
	if cErr := c.collector.CollectItems(ctx, src, []biz.CollectedItem{msg.Item}); cErr != nil {
		c.log.ErrorContext(ctx, "telegram collector: collect failed", "source_id", src.ID, "err", cErr)
	}
	return nil
}

// resolve finds the configured Telegram source whose TgID matches, listing both
// Telegram source types. Per-message listing reflects current sources, so added/
// removed sources are picked up without a restart (source volume is small, CON-03).
func (c *TelegramCollector) resolve(ctx context.Context, tgID int64) (biz.Source, bool, error) {
	for _, st := range []biz.SourceType{biz.SourceTypeTelegramChannel, biz.SourceTypeTelegramGroup} {
		filter := biz.ListSourcesFilter{Type: st, PageSize: collectorPageSize}
		for {
			res, err := c.sources.List(ctx, filter)
			if err != nil {
				return biz.Source{}, false, err
			}
			for _, s := range res.Items {
				if id, ok := tgIDOf(s); ok && id == tgID {
					return s, true, nil
				}
			}
			if res.NextPageToken == "" {
				break
			}
			filter.PageToken = res.NextPageToken
		}
	}
	return biz.Source{}, false, nil
}

func tgIDOf(s biz.Source) (int64, bool) {
	switch cfg := s.Config.(type) {
	case *biz.TelegramChannelConfig:
		return cfg.TgID, true
	case *biz.TelegramGroupConfig:
		return cfg.TgID, true
	default:
		return 0, false
	}
}
