package task_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
	entgo "github.com/4itosik/feedium/internal/ent"
)

func setupTestStack(t *testing.T) (*data.Data, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:18.3-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)

	err = goose.SetDialect("postgres")
	require.NoError(t, err)

	wd := filepath.Join("..", "..", "migrations")
	require.NoError(t, goose.Up(db, wd))

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := entgo.NewClient(entgo.Driver(drv))

	cleanup := func() {
		_ = client.Close()
		_ = db.Close()
		_ = container.Terminate(ctx)
	}
	return &data.Data{DB: db, Ent: client}, cleanup
}

func createTestGroupSource(ctx context.Context, t *testing.T, d *data.Data) string {
	t.Helper()
	repo := data.NewSourceRepo(d)
	saved, err := repo.Save(ctx, biz.Source{
		Type: biz.SourceTypeTelegramGroup,
		Config: &biz.TelegramGroupConfig{
			TgID:     time.Now().UnixNano(),
			Username: "grp_" + uuid.NewString()[:8],
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	require.NoError(t, err)
	return saved.ID
}

func createTestPost(ctx context.Context, t *testing.T, d *data.Data, sourceID, text string) biz.Post {
	t.Helper()
	repo := data.NewPostRepo(d)
	saved, _, err := repo.Save(ctx, biz.Post{
		ID:          uuid.Must(uuid.NewV7()).String(),
		SourceID:    sourceID,
		ExternalID:  "ext-" + uuid.NewString()[:8],
		PublishedAt: time.Now(),
		Text:        text,
		Metadata:    map[string]string{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
	require.NoError(t, err)
	return saved
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testSummaryCfg() *conf.Summary {
	return &conf.Summary{
		Worker: &conf.SummaryWorker{
			PollInterval:      durationpb.New(20 * time.Millisecond),
			Concurrency:       2,
			LeaseTtl:          durationpb.New(2 * time.Second),
			HeartbeatInterval: durationpb.New(500 * time.Millisecond),
			GracefulTimeout:   durationpb.New(2 * time.Second),
			MaxAttempts:       3,
			BackoffBase:       durationpb.New(100 * time.Millisecond),
			BackoffMax:        durationpb.New(500 * time.Millisecond),
		},
		Cron:   &conf.SummaryCron{Interval: durationpb.New(1 * time.Second)},
		Llm:    &conf.SummaryLLM{MaxRetries: 1, Timeout: durationpb.New(1 * time.Second)},
		Outbox: &conf.SummaryOutbox{EventTtl: durationpb.New(24 * time.Hour)},
		Cumulative: &conf.SummaryCumulative{
			MaxWindow:     durationpb.New(72 * time.Hour),
			MaxInputChars: 50000,
		},
		SourceScheduler: &conf.SummarySourceScheduler{
			PollInterval: durationpb.New(50 * time.Millisecond),
		},
		Reaper: &conf.SummaryReaper{
			Interval: durationpb.New(50 * time.Millisecond),
			Grace:    durationpb.New(50 * time.Millisecond),
		},
	}
}

// fakeLLM is a simple counting LLM provider for tests.
type fakeLLM struct {
	mu    sync.Mutex
	calls int
	reply string
	err   error
	delay time.Duration
}

func (f *fakeLLM) Summarize(ctx context.Context, text string) (string, error) {
	f.mu.Lock()
	f.calls++
	d := f.delay
	err := f.err
	reply := f.reply
	f.mu.Unlock()
	if d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err != nil {
		return "", err
	}
	if reply == "" {
		return "summary of: " + text[:min(len(text), 20)], nil
	}
	return reply, nil
}

func (f *fakeLLM) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
