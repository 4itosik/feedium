package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"feedium/internal/bootstrap"
	"feedium/internal/platform/logger"
)

func main() {
	os.Exit(run())
}

func run() int {
	log := logger.Init()
	log.Info("Feedium is starting")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd, err := parseCommand(os.Args[1:])
	if err != nil {
		log.Error("invalid command", "error", err)
		fmt.Fprintln(os.Stderr, "usage: feedium [run [migrate]]")
		return 1
	}

	switch cmd {
	case commandServe:
		runErr := bootstrap.Run(ctx, log)
		if runErr != nil {
			log.Error("server error", "error", runErr)
			return 1
		}
	case commandMigrate:
		migrateErr := bootstrap.Migrate(ctx, log)
		if migrateErr != nil {
			log.Error("migration error", "error", migrateErr)
			return 1
		}
	}

	return 0
}
