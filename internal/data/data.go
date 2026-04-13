package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq" // Blank import for SQL driver registration

	"github.com/4itosik/feedium/internal/conf"
	entgo "github.com/4itosik/feedium/internal/ent"
)

type Data struct {
	DB  *sql.DB
	Ent *entgo.Client
}

func NewData(c *conf.Data, logger *slog.Logger) (*Data, func(), error) {
	if c == nil || c.GetDatabase() == nil {
		return nil, nil, errors.New("database configuration is required")
	}

	dbConfig := c.GetDatabase()
	//nolint:nosprintfhostport // PostgreSQL DSN requires host:port format
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		dbConfig.GetUser(),
		dbConfig.GetPassword(),
		dbConfig.GetHost(),
		dbConfig.GetPort(),
		dbConfig.GetDatabase(),
		dbConfig.GetSslmode())

	db, openErr := sql.Open("postgres", dsn)
	if openErr != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", openErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:mnd // Startup ping timeout
	defer cancel()

	if pingErr := db.PingContext(ctx); pingErr != nil {
		return nil, nil, fmt.Errorf("failed to ping database: %w", pingErr)
	}

	drv := entsql.OpenDB(dialect.Postgres, db)
	entClient := entgo.NewClient(entgo.Driver(drv))

	logger.Info(fmt.Sprintf("database connected: host=%s port=%d database=%s",
		dbConfig.GetHost(), dbConfig.GetPort(), dbConfig.GetDatabase()))

	cleanup := func() {
		if closeErr := entClient.Close(); closeErr != nil {
			logger.Error("failed to close ent client", "error", closeErr)
		}
		if closeErr := db.Close(); closeErr != nil {
			logger.Error("failed to close database", "error", closeErr)
		}
	}

	return &Data{DB: db, Ent: entClient}, cleanup, nil
}

type HealthRepo struct {
	data *Data
	log  *slog.Logger
}

func NewHealthRepo(data *Data, logger *slog.Logger) *HealthRepo {
	return &HealthRepo{data: data, log: logger}
}

func (h *HealthRepo) Ping(ctx context.Context) error {
	return h.data.DB.PingContext(ctx)
}

func (h *HealthRepo) GetDB() *sql.DB {
	return h.data.DB
}
