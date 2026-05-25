package task

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

// WebCollector is a transport.Server that periodically collects every web
// (html) source. Web extraction is brittle to markup changes (RISK-04), so it
// applies the architecture's web policy: a single retry, then skip the source
// with an observable error (FM-T02, CON-02). Like the RSS collector, one
// source's failure never stops the others (REQ-07).
type WebCollector struct {
	collector Collector
	fetcher   biz.WebFetcher
	sources   SourceLister
	cfg       *conf.Collectors
	log       *slog.Logger

	// after — see RSSCollector.after.
	after    func(time.Duration) <-chan time.Time
	done     chan struct{}
	doneOnce sync.Once
	wg       sync.WaitGroup
}

// NewWebCollector constructs the web collector.
func NewWebCollector(
	collector Collector,
	fetcher biz.WebFetcher,
	sources SourceLister,
	cfg *conf.Collectors,
	logger *slog.Logger,
) *WebCollector {
	return &WebCollector{
		collector: collector,
		fetcher:   fetcher,
		sources:   sources,
		cfg:       cfg,
		log:       logger,
		after:     time.After,
		done:      make(chan struct{}),
	}
}

// Start launches the polling goroutine. The interval is read from config
// (REQ-08, SC-08), falling back to defaultCollectorPoll when unset.
func (c *WebCollector) Start(ctx context.Context) error {
	interval := c.cfg.GetWeb().GetPollInterval().AsDuration()
	if interval <= 0 {
		interval = defaultCollectorPoll
	}
	c.wg.Go(func() {
		runCollectLoop(ctx, c.done, interval, c.after, c.tick)
	})
	return nil
}

// Stop signals the loop to exit and waits for the in-flight tick to finish
// (graceful shutdown).
func (c *WebCollector) Stop(_ context.Context) error {
	c.doneOnce.Do(func() { close(c.done) })
	c.wg.Wait()
	return nil
}

func (c *WebCollector) tick(ctx context.Context) {
	collectAllOfType(ctx, c.log, "web collector", biz.SourceTypeHTML, c.sources, func(src biz.Source) error {
		return c.collectWithRetry(ctx, src)
	})
}

// collectWithRetry implements single-retry-then-skip (FM-T02): if the first
// attempt fails, retry once; the second error is returned so the caller logs
// it and skips the source. It does not retry when the context is already
// cancelled or expired — during graceful shutdown a retry would issue a
// pointless extra HTTP request and delay Stop (AGENT_3 #2).
func (c *WebCollector) collectWithRetry(ctx context.Context, src biz.Source) error {
	err := c.collector.CollectSource(ctx, src, c.fetcher)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return err
	}
	return c.collector.CollectSource(ctx, src, c.fetcher)
}
