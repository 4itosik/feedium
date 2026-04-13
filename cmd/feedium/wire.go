//go:build wireinject

package main

import (
	"log/slog"

	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/server"
	healthservice "github.com/4itosik/feedium/internal/service/health"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/google/wire"
)

func newApp(logger *slog.Logger, hs *http.Server, gs *grpc.Server) *kratos.App {
	return kratos.New(
		kratos.Name("feedium"),
		kratos.Server(hs, gs),
	)
}

func newDataFromBootstrap(bc *conf.Bootstrap) *conf.Data {
	return bc.GetData()
}

func newServerFromBootstrap(bc *conf.Bootstrap) *conf.Server {
	return bc.GetServer()
}

func wireApp(bc *conf.Bootstrap, logger *slog.Logger) (*kratos.App, func(), error) {
	wire.Build(
		newServerFromBootstrap,
		newDataFromBootstrap,
		server.ProviderSet,
		data.ProviderSet,
		healthservice.ProviderSet,
		newApp,
	)
	return &kratos.App{}, nil, nil
}
