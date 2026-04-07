package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"feedium/internal/platform/migrator"
	"feedium/internal/platform/postgres"
)

func Migrate(ctx context.Context, log *slog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	db, err := postgres.Open(dsn)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("open sql db: %w", err)
	}
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			log.WarnContext(ctx, "failed to close sql db", "error", closeErr)
		}
	}()

	log.InfoContext(ctx, "running migrations")
	migrateErr := migrator.Up(sqlDB)
	if migrateErr != nil {
		return migrateErr
	}
	log.InfoContext(ctx, "migrations completed")

	return nil
}
