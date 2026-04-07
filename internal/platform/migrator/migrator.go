package migrator

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"feedium/migrations"
)

func Up(db *sql.DB) error {
	goose.SetBaseFS(migrations.Files)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("run goose up: %w", err)
	}

	return nil
}
