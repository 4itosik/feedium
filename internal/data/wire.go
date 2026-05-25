package data

import (
	"github.com/google/wire"

	"github.com/4itosik/feedium/internal/biz"
	healthservice "github.com/4itosik/feedium/internal/service/health"
)

var ProviderSet = wire.NewSet( //nolint:gochecknoglobals // wire dependency injection
	NewData,
	NewHealthRepo,
	NewSourceRepo,
	NewPostRepo,
	NewSummaryRepo,
	NewSummaryOutboxRepo,
	NewTxManager,
	NewOpenRouterProvider,
	NewRSSFetcher,
	NewWebFetcher,
	wire.Bind(new(healthservice.Pinger), new(*HealthRepo)),
	wire.Bind(new(biz.SourceRepo), new(*sourceRepo)),
	wire.Bind(new(biz.PostRepo), new(*postRepo)),
	wire.Bind(new(biz.SummaryRepo), new(*summaryRepo)),
	wire.Bind(new(biz.SummaryOutboxRepo), new(*summaryOutboxRepo)),
	wire.Bind(new(biz.TxManager), new(*txManager)),
	wire.Bind(new(biz.LLMProvider), new(*openRouterProvider)),
	// Each collector binds to a distinct named Fetcher port (DEC-06): both
	// fetchers satisfy the bare biz.Fetcher method set, so explicit binds
	// resolve which concrete implements which port.
	wire.Bind(new(biz.RSSFetcher), new(*rssFetcher)),
	wire.Bind(new(biz.WebFetcher), new(*webFetcher)),
)
