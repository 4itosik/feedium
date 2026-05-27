package task

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

const (
	// defaultCollectorPoll is used when the config omits or zeroes the interval.
	defaultCollectorPoll = 5 * time.Minute
	// collectorPageSize bounds how many sources are listed per page per tick.
	collectorPageSize = 100
)

// SourceLister is the task-level view of biz.SourceUsecase.List. Collectors
// read sources of their own type only; sources of other types are ignored,
// which is the first layer of cross-collector failure isolation (REQ-07).
type SourceLister interface {
	List(ctx context.Context, filter biz.ListSourcesFilter) (biz.ListSourcesResult, error)
}

// Collector is the task-level view of biz.CollectUsecase.CollectSource. The
// usecase owns text filtering, persistence and the outbox contract; collectors
// own only scheduling, fetcher choice, logging and retry (CON-02).
type Collector interface {
	CollectSource(ctx context.Context, source biz.Source, fetcher biz.Fetcher) error
}

// RSSCollector is a transport.Server that periodically collects every RSS
// source. It mirrors SourceDueScheduler's lifecycle (Start spawns one polling
// goroutine, Stop closes done once). A failure on one source is logged and
// skipped without stopping the others (REQ-07, FM-T01).
type RSSCollector struct {
	collector Collector
	fetcher   biz.RSSFetcher
	sources   SourceLister
	cfg       *conf.Collectors
	log       *slog.Logger

	// after is the wait primitive between ticks. Production uses time.After;
	// tests inject a controllable channel to drive the cadence deterministically
	// and assert the interval is sourced from config (SC-08).
	after    func(time.Duration) <-chan time.Time
	done     chan struct{}
	doneOnce sync.Once
	wg       sync.WaitGroup
}

// NewRSSCollector constructs the RSS collector.
func NewRSSCollector(
	collector Collector,
	fetcher biz.RSSFetcher,
	sources SourceLister,
	cfg *conf.Collectors,
	logger *slog.Logger,
) *RSSCollector {
	return &RSSCollector{
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
func (c *RSSCollector) Start(ctx context.Context) error {
	interval := c.cfg.GetRss().GetPollInterval().AsDuration()
	if interval <= 0 {
		interval = defaultCollectorPoll
	}
	c.wg.Go(func() {
		runCollectLoop(ctx, c.done, interval, c.after, c.tick)
	})
	return nil
}

// Stop signals the loop to exit and waits for the in-flight tick to finish, so
// no collection runs after kratos considers the server stopped (graceful
// shutdown).
func (c *RSSCollector) Stop(_ context.Context) error {
	c.doneOnce.Do(func() { close(c.done) })
	c.wg.Wait()
	return nil
}

func (c *RSSCollector) tick(ctx context.Context) {
	collectAllOfType(
		ctx,
		c.log,
		"rss collector",
		biz.SourceTypeRSS,
		c.sources,
		func(src biz.Source) error { return c.collector.CollectSource(ctx, src, c.fetcher) },
	)
}

// runCollectLoop runs tick immediately and then once per interval until the
// context is cancelled or done is closed (graceful shutdown via kratos). The
// wait between ticks is taken from after(interval); production passes
// [time.After], tests inject a controllable channel (SC-08).
func runCollectLoop(
	ctx context.Context,
	done <-chan struct{},
	interval time.Duration,
	after func(time.Duration) <-chan time.Time,
	tick func(context.Context),
) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		default:
		}

		tick(ctx)

		select {
		case <-after(interval):
		case <-ctx.Done():
			return
		case <-done:
			return
		}
	}
}

// collectAllOfType lists every source of the given type (paging through the
// results) and collects each one. Listing failure ends the tick; a single
// source failure is logged and skipped so the rest still run (REQ-07).
func collectAllOfType(
	ctx context.Context,
	log *slog.Logger,
	name string,
	sourceType biz.SourceType,
	sources SourceLister,
	collectOne func(biz.Source) error,
) {
	filter := biz.ListSourcesFilter{Type: sourceType, PageSize: collectorPageSize}
	for {
		res, err := sources.List(ctx, filter)
		if err != nil {
			log.ErrorContext(ctx, name+": list sources failed", "err", err)
			return
		}

		for _, src := range res.Items {
			if collectErr := collectOne(src); collectErr != nil {
				log.ErrorContext(ctx, name+": collect source failed", "source_id", src.ID, "err", collectErr)
				continue
			}
		}

		if res.NextPageToken == "" {
			return
		}
		filter.PageToken = res.NextPageToken
	}
}
