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
	NewInterestRepo,
	NewScoreRepo,
	NewScoreOutboxRepo,
	NewPostEmbeddingRepo,
	NewReactionRepo,
	NewEmbeddingProvider,
	NewTxManager,
	NewSummaryLLMRouter,
	NewRSSFetcher,
	NewWebFetcher,
	NewTelegramSession,
	wire.Bind(new(healthservice.Pinger), new(*HealthRepo)),
	wire.Bind(new(biz.SourceRepo), new(*sourceRepo)),
	wire.Bind(new(biz.PostRepo), new(*postRepo)),
	wire.Bind(new(biz.SummaryRepo), new(*summaryRepo)),
	wire.Bind(new(biz.SummaryOutboxRepo), new(*summaryOutboxRepo)),
	wire.Bind(new(biz.InterestRepo), new(*interestRepo)),
	wire.Bind(new(biz.ScoreRepo), new(*scoreRepo)),
	wire.Bind(new(biz.FeedQueryRepo), new(*scoreRepo)),
	wire.Bind(new(biz.ScoreOutboxRepo), new(*scoreOutboxRepo)),
	wire.Bind(new(biz.PostEmbeddingRepo), new(*postEmbeddingRepo)),
	wire.Bind(new(biz.ReactionRepo), new(*reactionRepo)),
	wire.Bind(new(biz.TxManager), new(*txManager)),
	wire.Bind(new(biz.SummaryLLMResolver), new(*summaryLLMRouter)),
	wire.Bind(new(biz.Scorer), new(*embeddingProvider)),
	// Each collector binds to a distinct named Fetcher port (DEC-06): both
	// fetchers satisfy the bare biz.Fetcher method set, so explicit binds
	// resolve which concrete implements which port.
	wire.Bind(new(biz.RSSFetcher), new(*rssFetcher)),
	wire.Bind(new(biz.WebFetcher), new(*webFetcher)),
	wire.Bind(new(biz.TelegramSession), new(*telegramSession)),
)
