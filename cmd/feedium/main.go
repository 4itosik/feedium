package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"

	"github.com/4itosik/feedium/internal/conf"
)

//nolint:gochecknoglobals // flag package requires global variable
var flagconf = flag.String(
	"conf",
	"../../configs",
	"config path, eg: -conf ./configs",
)

func main() {
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	c := config.New(
		config.WithSource(
			file.NewSource(*flagconf),
		),
	)
	if err := c.Load(); err != nil {
		logger.Error("failed to load config", "path", *flagconf, "error", err)
		os.Exit(1)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		logger.Error("failed to scan config", "error", err)
		os.Exit(1)
	}

	if bc.GetServer().GetHttp().GetAddr() == "" {
		logger.Error("server.http.addr is empty")
		os.Exit(1)
	}
	if bc.GetServer().GetGrpc().GetAddr() == "" {
		logger.Error("server.grpc.addr is empty")
		os.Exit(1)
	}

	app, cleanup, wireErr := wireApp(bc.GetServer(), logger)
	if wireErr != nil {
		logger.Error("failed to create app", "error", wireErr)
		os.Exit(1)
	}

	if runErr := runApp(app, cleanup); runErr != nil {
		logger.Error("failed to run app", "error", runErr)
		cleanup()
		os.Exit(1)
	}
}

func runApp(app *kratos.App, cleanup func()) error {
	defer cleanup()
	return app.Run()
}
