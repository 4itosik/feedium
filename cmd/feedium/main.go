package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"feedium/internal/bootstrap"
	"feedium/internal/platform/logger"
)

func main() {
	log := logger.Init()
	log.Info("Feedium is starting")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Run(ctx, log); err != nil {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
}
