package data_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
	entgo "github.com/4itosik/feedium/internal/ent"
)

func TestIntegration_Save(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSourceRepo(&data.Data{Ent: client})

	t.Run("save telegram_channel source", func(t *testing.T) {
		config := &biz.TelegramChannelConfig{TgID: 123456789, Username: "test_channel"}
		source := biz.Source{
			Type:      biz.SourceTypeTelegramChannel,
			Config:    config,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		saved, err := repo.Save(ctx, source)
		require.NoError(t, err)
		assert.NotEmpty(t, saved.ID)
		assert.Equal(t, biz.SourceTypeTelegramChannel, saved.Type)
		assert.Equal(t, biz.ProcessingModeSelfContained, saved.ProcessingMode)
		assert.False(t, saved.CreatedAt.IsZero())
		assert.False(t, saved.UpdatedAt.IsZero())

		savedConfig, ok := saved.Config.(*biz.TelegramChannelConfig)
		require.True(t, ok)
		assert.Equal(t, int64(123456789), savedConfig.TgID)
		assert.Equal(t, "test_channel", savedConfig.Username)
	})

	t.Run("save telegram_group source", func(t *testing.T) {
		config := &biz.TelegramGroupConfig{TgID: 987654321, Username: "test_group"}
		source := biz.Source{
			Type:      biz.SourceTypeTelegramGroup,
			Config:    config,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		saved, err := repo.Save(ctx, source)
		require.NoError(t, err)
		assert.NotEmpty(t, saved.ID)
		assert.Equal(t, biz.SourceTypeTelegramGroup, saved.Type)
		assert.Equal(t, biz.ProcessingModeCumulative, saved.ProcessingMode)
	})

	t.Run("save RSS source", func(t *testing.T) {
		config := &biz.RSSConfig{FeedURL: "https://example.com/feed"}
		source := biz.Source{
			Type:      biz.SourceTypeRSS,
			Config:    config,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		saved, err := repo.Save(ctx, source)
		require.NoError(t, err)
		assert.NotEmpty(t, saved.ID)
		assert.Equal(t, biz.SourceTypeRSS, saved.Type)
		assert.Equal(t, biz.ProcessingModeSelfContained, saved.ProcessingMode)

		savedConfig, ok := saved.Config.(*biz.RSSConfig)
		require.True(t, ok)
		assert.Equal(t, "https://example.com/feed", savedConfig.FeedURL)
	})

	t.Run("save HTML source", func(t *testing.T) {
		config := &biz.HTMLConfig{URL: "https://example.com"}
		source := biz.Source{
			Type:      biz.SourceTypeHTML,
			Config:    config,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		saved, err := repo.Save(ctx, source)
		require.NoError(t, err)
		assert.NotEmpty(t, saved.ID)
		assert.Equal(t, biz.SourceTypeHTML, saved.Type)
		assert.Equal(t, biz.ProcessingModeSelfContained, saved.ProcessingMode)
	})
}

func TestIntegration_Get(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSourceRepo(&data.Data{Ent: client})

	t.Run("get saved source", func(t *testing.T) {
		config := &biz.RSSConfig{FeedURL: "https://example.com/feed"}
		now := time.Now()
		created, err := repo.Save(ctx, biz.Source{
			Type:      biz.SourceTypeRSS,
			Config:    config,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		fetched, err := repo.Get(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, fetched.ID)
		assert.Equal(t, created.Type, fetched.Type)
		assert.Equal(t, created.ProcessingMode, fetched.ProcessingMode)
		assert.WithinDuration(t, created.CreatedAt, fetched.CreatedAt, time.Second)
		assert.WithinDuration(t, created.UpdatedAt, fetched.UpdatedAt, time.Second)

		fetchedConfig, ok := fetched.Config.(*biz.RSSConfig)
		require.True(t, ok)
		assert.Equal(t, "https://example.com/feed", fetchedConfig.FeedURL)
	})

	t.Run("get not found", func(t *testing.T) {
		_, err := repo.Get(ctx, "00000000-0000-0000-0000-000000000001")
		assert.ErrorIs(t, err, biz.ErrSourceNotFound)
	})
}

func TestIntegration_Update(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSourceRepo(&data.Data{Ent: client})

	t.Run("update source", func(t *testing.T) {
		now := time.Now()
		created, err := repo.Save(ctx, biz.Source{
			Type:      biz.SourceTypeRSS,
			Config:    &biz.RSSConfig{FeedURL: "https://example.com/feed"},
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		updated, err := repo.Update(ctx, biz.Source{
			ID:        created.ID,
			Type:      biz.SourceTypeRSS,
			Config:    &biz.RSSConfig{FeedURL: "https://updated.com/feed"},
			UpdatedAt: time.Now(),
		})
		require.NoError(t, err)
		assert.Equal(t, created.ID, updated.ID)
		assert.Greater(t, updated.UpdatedAt, created.UpdatedAt)

		updatedConfig, ok := updated.Config.(*biz.RSSConfig)
		require.True(t, ok)
		assert.Equal(t, "https://updated.com/feed", updatedConfig.FeedURL)
	})

	t.Run("update not found", func(t *testing.T) {
		_, err := repo.Update(ctx, biz.Source{
			ID:     "00000000-0000-0000-0000-000000000001",
			Type:   biz.SourceTypeRSS,
			Config: &biz.RSSConfig{FeedURL: "https://example.com/feed"},
		})
		assert.ErrorIs(t, err, biz.ErrSourceNotFound)
	})
}

func TestIntegration_Delete(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSourceRepo(&data.Data{Ent: client})

	t.Run("delete source", func(t *testing.T) {
		now := time.Now()
		created, err := repo.Save(ctx, biz.Source{
			Type:      biz.SourceTypeRSS,
			Config:    &biz.RSSConfig{FeedURL: "https://example.com/feed"},
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		err = repo.Delete(ctx, created.ID)
		require.NoError(t, err)

		_, err = repo.Get(ctx, created.ID)
		assert.ErrorIs(t, err, biz.ErrSourceNotFound)
	})

	t.Run("delete not found", func(t *testing.T) {
		err := repo.Delete(ctx, "00000000-0000-0000-0000-000000000001")
		assert.ErrorIs(t, err, biz.ErrSourceNotFound)
	})
}

func TestIntegration_List(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSourceRepo(&data.Data{Ent: client})

	result, err := repo.List(ctx, biz.ListSourcesFilter{})
	require.NoError(t, err)
	assert.Empty(t, result.Items)
	assert.Empty(t, result.NextPageToken)
}

func TestIntegration_ListAllSources(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSourceRepo(&data.Data{Ent: client})

	now := time.Now()
	sources := []biz.Source{
		{
			Type:      biz.SourceTypeRSS,
			Config:    &biz.RSSConfig{FeedURL: "https://1-list-all.com/feed"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Type:      biz.SourceTypeRSS,
			Config:    &biz.RSSConfig{FeedURL: "https://2-list-all.com/feed"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			Type:      biz.SourceTypeHTML,
			Config:    &biz.HTMLConfig{URL: "https://3-list-all.com"},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	for _, src := range sources {
		_, err := repo.Save(ctx, src)
		require.NoError(t, err)
	}

	result, err := repo.List(ctx, biz.ListSourcesFilter{PageSize: 100})
	require.NoError(t, err)
	assert.Len(t, result.Items, 3)
}

func TestIntegration_ListPagination(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSourceRepo(&data.Data{Ent: client})

	for i := range 5 {
		now := time.Now()
		_, err := repo.Save(ctx, biz.Source{
			Type:      biz.SourceTypeTelegramChannel,
			Config:    &biz.TelegramChannelConfig{TgID: int64(i + 1), Username: fmt.Sprintf("pagination-%d", i)},
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	page1, err := repo.List(ctx, biz.ListSourcesFilter{PageSize: 2})
	require.NoError(t, err)
	assert.Len(t, page1.Items, 2)
	assert.NotEmpty(t, page1.NextPageToken)

	page2, err := repo.List(ctx, biz.ListSourcesFilter{PageSize: 2, PageToken: page1.NextPageToken})
	require.NoError(t, err)
	assert.Len(t, page2.Items, 2)
	assert.NotEmpty(t, page2.NextPageToken)

	page3, err := repo.List(ctx, biz.ListSourcesFilter{PageSize: 2, PageToken: page2.NextPageToken})
	require.NoError(t, err)
	assert.Len(t, page3.Items, 1)
	assert.Empty(t, page3.NextPageToken)
}

func TestIntegration_ListFilterByType(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSourceRepo(&data.Data{Ent: client})

	now := time.Now()
	_, err := repo.Save(ctx, biz.Source{
		Type:      biz.SourceTypeRSS,
		Config:    &biz.RSSConfig{FeedURL: "https://filter-rss.com/feed"},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	_, err = repo.Save(ctx, biz.Source{
		Type:      biz.SourceTypeHTML,
		Config:    &biz.HTMLConfig{URL: "https://filter-html.com"},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	result, err := repo.List(ctx, biz.ListSourcesFilter{
		Type:     biz.SourceTypeRSS,
		PageSize: 100,
	})
	require.NoError(t, err)

	for _, item := range result.Items {
		assert.Equal(t, biz.SourceTypeRSS, item.Type)
	}
}

func TestIntegration_ConfigRoundtrip(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSourceRepo(&data.Data{Ent: client})

	t.Run("telegram_channel config", func(t *testing.T) {
		now := time.Now()
		originalConfig := &biz.TelegramChannelConfig{TgID: 123456789, Username: "test_channel"}
		saved, err := repo.Save(ctx, biz.Source{
			Type:      biz.SourceTypeTelegramChannel,
			Config:    originalConfig,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		fetched, err := repo.Get(ctx, saved.ID)
		require.NoError(t, err)

		fetchedConfig, ok := fetched.Config.(*biz.TelegramChannelConfig)
		require.True(t, ok)
		assert.Equal(t, originalConfig.TgID, fetchedConfig.TgID)
		assert.Equal(t, originalConfig.Username, fetchedConfig.Username)
	})

	t.Run("telegram_group config", func(t *testing.T) {
		now := time.Now()
		originalConfig := &biz.TelegramGroupConfig{TgID: 987654321, Username: "test_group"}
		saved, err := repo.Save(ctx, biz.Source{
			Type:      biz.SourceTypeTelegramGroup,
			Config:    originalConfig,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		fetched, err := repo.Get(ctx, saved.ID)
		require.NoError(t, err)

		fetchedConfig, ok := fetched.Config.(*biz.TelegramGroupConfig)
		require.True(t, ok)
		assert.Equal(t, originalConfig.TgID, fetchedConfig.TgID)
		assert.Equal(t, originalConfig.Username, fetchedConfig.Username)
	})

	t.Run("RSS config", func(t *testing.T) {
		now := time.Now()
		originalConfig := &biz.RSSConfig{FeedURL: "https://example.com/feed?param=value"}
		saved, err := repo.Save(ctx, biz.Source{
			Type:      biz.SourceTypeRSS,
			Config:    originalConfig,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		fetched, err := repo.Get(ctx, saved.ID)
		require.NoError(t, err)

		fetchedConfig, ok := fetched.Config.(*biz.RSSConfig)
		require.True(t, ok)
		assert.Equal(t, originalConfig.FeedURL, fetchedConfig.FeedURL)
	})

	t.Run("HTML config", func(t *testing.T) {
		now := time.Now()
		originalConfig := &biz.HTMLConfig{URL: "https://example.com/page"}
		saved, err := repo.Save(ctx, biz.Source{
			Type:      biz.SourceTypeHTML,
			Config:    originalConfig,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.NoError(t, err)

		fetched, err := repo.Get(ctx, saved.ID)
		require.NoError(t, err)

		fetchedConfig, ok := fetched.Config.(*biz.HTMLConfig)
		require.True(t, ok)
		assert.Equal(t, originalConfig.URL, fetchedConfig.URL)
	})
}

func setupTestDB(t *testing.T) (*entgo.Client, func()) {
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

	wd, err := os.Getwd()
	require.NoError(t, err)

	migrationsDir := filepath.Join(wd, "..", "..", "migrations")
	err = goose.Up(db, migrationsDir)
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.Postgres, db)
	entClient := entgo.NewClient(entgo.Driver(drv))

	cleanup := func() {
		if closeErr := entClient.Close(); closeErr != nil {
			t.Logf("failed to close ent client: %v", closeErr)
		}
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("failed to close db: %v", closeErr)
		}
		if closeErr := container.Terminate(ctx); closeErr != nil {
			t.Logf("failed to terminate container: %v", closeErr)
		}
	}

	return entClient, cleanup
}
