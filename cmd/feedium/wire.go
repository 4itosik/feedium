//go:build wireinject

package main

import (
	"log/slog"

	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/server"
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

func wireApp(*conf.Server, *slog.Logger) (*kratos.App, func(), error) {
	wire.Build(
		server.ProviderSet,
		newApp,
	)
	return &kratos.App{}, nil, nil
}
