//go:build wireinject

package main

import (
	"log/slog"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/server"
	healthservice "github.com/4itosik/feedium/internal/service/health"
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
) *kratos.App {
	return kratos.New(
		kratos.Name("feedium"),
		kratos.Server(hs, gs, pool, scheduler, reaper),
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

func newSummaryLLMFromBootstrap(bc *conf.Bootstrap) *conf.SummaryLLM {
	if bc.GetSummary() == nil {
		return nil
	}
	return bc.GetSummary().GetLlm()
}

//nolint:revive,unused // wire injector shell used at generation time.
func wireApp(bc *conf.Bootstrap, logger *slog.Logger) (*kratos.App, func(), error) {
	wire.Build(
		newServerFromBootstrap,
		newDataFromBootstrap,
		newSummaryConfigFromBootstrap,
		newSummaryLLMFromBootstrap,
		server.ProviderSet,
		data.ProviderSet,
		healthservice.ProviderSet,
		biz.ProviderSet,
		sourceservice.ProviderSet,
		postservice.ProviderSet,
		summaryservice.ProviderSet,
		task.ProviderSet,
		wire.Bind(new(sourceservice.Usecase), new(*biz.SourceUsecase)),
		wire.Bind(new(postservice.Usecase), new(*biz.PostUsecase)),
		wire.Bind(new(summaryservice.Usecase), new(*biz.SummaryUsecase)),
		newApp,
	)
	return &kratos.App{}, nil, nil
}
