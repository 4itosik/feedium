//go:build wireinject

package main

import (
	"log/slog"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/biz/prompt"
	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/server"
	feedservice "github.com/4itosik/feedium/internal/service/feed"
	healthservice "github.com/4itosik/feedium/internal/service/health"
	interestservice "github.com/4itosik/feedium/internal/service/interest"
	postservice "github.com/4itosik/feedium/internal/service/post"
	sourceservice "github.com/4itosik/feedium/internal/service/source"
	summaryservice "github.com/4itosik/feedium/internal/service/summary"
	"github.com/4itosik/feedium/internal/task"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/google/wire"
)

func newApp(
	logger *slog.Logger,
	hs *http.Server,
	gs *grpc.Server,
	pool *task.EventWorkerPool,
	scheduler *task.SourceDueScheduler,
	reaper *task.StuckEventReaper,
	scorePool *task.ScoreEventWorkerPool,
	scoreScheduler *task.ScoreDueScheduler,
	scoreReaper *task.StuckScoreEventReaper,
	rssCollector *task.RSSCollector,
	webCollector *task.WebCollector,
	telegramCollector *task.TelegramCollector,
) *kratos.App {
	return kratos.New(
		kratos.Name("feedium"),
		kratos.Server(
			hs, gs, pool, scheduler, reaper,
			scorePool, scoreScheduler, scoreReaper,
			rssCollector, webCollector, telegramCollector,
		),
	)
}

func newDataFromBootstrap(bc *conf.Bootstrap) *conf.Data {
	return bc.GetData()
}

func newServerFromBootstrap(bc *conf.Bootstrap) *conf.Server {
	return bc.GetServer()
}

func newSummaryConfigFromBootstrap(bc *conf.Bootstrap) *conf.Summary {
	return bc.GetSummary()
}

func newCollectorsConfigFromBootstrap(bc *conf.Bootstrap) *conf.Collectors {
	return bc.GetCollectors()
}

func newSummaryLLMFromBootstrap(bc *conf.Bootstrap) *conf.SummaryLLM {
	if bc.GetSummary() == nil {
		return nil
	}
	return bc.GetSummary().GetLlm()
}

func newScoringConfigFromBootstrap(bc *conf.Bootstrap) *conf.Scoring {
	return bc.GetScoring()
}

func newScoringEmbeddingFromBootstrap(bc *conf.Bootstrap) *conf.ScoringEmbedding {
	if bc.GetScoring() == nil {
		return nil
	}
	return bc.GetScoring().GetEmbedding()
}

//nolint:revive,unused // wire injector shell used at generation time.
func wireApp(bc *conf.Bootstrap, logger *slog.Logger) (*kratos.App, func(), error) {
	wire.Build(
		newServerFromBootstrap,
		newDataFromBootstrap,
		newSummaryConfigFromBootstrap,
		newSummaryLLMFromBootstrap,
		newScoringConfigFromBootstrap,
		newScoringEmbeddingFromBootstrap,
		newCollectorsConfigFromBootstrap,
		server.ProviderSet,
		data.ProviderSet,
		healthservice.ProviderSet,
		biz.ProviderSet,
		prompt.ProviderSet,
		sourceservice.ProviderSet,
		postservice.ProviderSet,
		summaryservice.ProviderSet,
		interestservice.ProviderSet,
		feedservice.ProviderSet,
		task.ProviderSet,
		wire.Bind(new(sourceservice.Usecase), new(*biz.SourceUsecase)),
		wire.Bind(new(postservice.Usecase), new(*biz.PostUsecase)),
		wire.Bind(new(summaryservice.Usecase), new(*biz.SummaryUsecase)),
		wire.Bind(new(interestservice.Usecase), new(*biz.InterestUsecase)),
		wire.Bind(new(feedservice.Usecase), new(*biz.FeedUsecase)),
		wire.Bind(new(task.EventProcessor), new(*biz.SummaryUsecase)),
		wire.Bind(new(task.SourceScheduler), new(*biz.SummaryUsecase)),
		wire.Bind(new(task.StuckReaper), new(*biz.SummaryUsecase)),
		wire.Bind(new(task.ScoreProcessor), new(*biz.ScoreUsecase)),
		wire.Bind(new(task.ScoreScheduler), new(*biz.ScoreUsecase)),
		wire.Bind(new(task.ScoreReaper), new(*biz.ScoreUsecase)),
		wire.Bind(new(task.Collector), new(*biz.CollectUsecase)),
		wire.Bind(new(task.ItemCollector), new(*biz.CollectUsecase)),
		wire.Bind(new(task.SourceLister), new(*biz.SourceUsecase)),
		newApp,
	)
	return &kratos.App{}, nil, nil
}
