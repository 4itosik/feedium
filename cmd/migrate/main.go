// Command migrate applies the project's goose SQL migrations against the
// configured PostgreSQL database. It is a deliberately dependency-light wrapper
// over the goose library (already required for tests) plus the lib/pq driver
// (already required by data/), so building it adds no new modules — unlike the
// upstream goose CLI, which pulls in clickhouse/mysql/mssql drivers.
//
// The feedium server never auto-migrates; the deployment runs this as a
// one-shot step before the app starts (see docs/deploy.md).
//
// Usage:
//
//	DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=disable \
//	  migrate -dir /app/migrations up
//
// The first positional argument is the goose command (default "up"); remaining
// arguments are forwarded to goose (e.g. "up-to 20260602100200").
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/lib/pq" // postgres driver registration for database/sql
	"github.com/pressly/goose/v3"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})).
			Error("migrate failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("dir", "migrations", "path to the goose migrations directory")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required (postgres connection string)")
	}

	command := "up"
	extra := flag.Args()
	if len(extra) > 0 {
		command = extra[0]
		extra = extra[1:]
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err = goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err = goose.RunContext(context.Background(), command, db, *dir, extra...); err != nil {
		return fmt.Errorf("run migrations (%s): %w", command, err)
	}
	return nil
}
