package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"feedium/internal/app/source"

	mocket "github.com/Selvatico/go-mocket"
	"github.com/google/uuid"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRepositoryMappingHelpers(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	src := &source.Source{
		ID:        uuid.New(),
		Type:      source.TypeRSS,
		Name:      "name",
		URL:       "https://example.com",
		Config:    map[string]any{"feed_url": "https://feed"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	row := fromDomain(src)
	if row.ID != src.ID || row.Type != string(src.Type) || row.Name != src.Name || row.URL != src.URL {
		t.Fatalf("fromDomain basic fields mismatch: %+v", row)
	}

	back := toDomain(&row)
	if back.ID != src.ID || back.Type != src.Type || back.Name != src.Name || back.URL != src.URL {
		t.Fatalf("toDomain basic fields mismatch: %+v", back)
	}
	if back.Config["feed_url"] != "https://feed" {
		t.Fatalf("toDomain config mismatch: %+v", back.Config)
	}

	dst := &source.Source{}
	applyRow(dst, &row)
	if dst.ID != row.ID || dst.CreatedAt != row.CreatedAt || dst.UpdatedAt != row.UpdatedAt {
		t.Fatalf("applyRow mismatch: %+v", dst)
	}
}

func TestRepositoryCreate(t *testing.T) {
	repo := newMockRepo(t)

	src := &source.Source{
		ID:        uuid.New(),
		Type:      source.TypeRSS,
		Name:      "name",
		URL:       "https://example.com",
		Config:    map[string]any{"feed_url": "https://feed"},
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		UpdatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}

	mocket.Catcher.Reset().NewMock().WithQuery(`INSERT INTO "sources"`).WithRowsNum(1)
	err := repo.Create(context.Background(), src)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
}

func TestRepositoryGetByID(t *testing.T) {
	repo := newMockRepo(t)
	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("success", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`FROM "sources"`).WithReply([]map[string]any{
			{
				"id":         id.String(),
				"type":       "rss",
				"name":       "name",
				"url":        "https://example.com",
				"config":     []byte(`{"feed_url":"https://feed"}`),
				"created_at": now,
				"updated_at": now,
			},
		})
		got, err := repo.GetByID(context.Background(), id)
		if err != nil {
			t.Fatalf("GetByID failed: %v", err)
		}
		if got.ID != id || got.Type != source.TypeRSS || got.Name != "name" {
			t.Fatalf("unexpected source: %+v", got)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`FROM "sources"`).WithReply([]map[string]any{})
		_, err := repo.GetByID(context.Background(), id)
		if !errors.Is(err, source.ErrNotFound) {
			t.Fatalf("expected source.ErrNotFound, got: %v", err)
		}
	})

	t.Run("query_error", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`FROM "sources"`).WithQueryException()
		_, err := repo.GetByID(context.Background(), id)
		if err == nil {
			t.Fatal("expected query error")
		}
		if errors.Is(err, source.ErrNotFound) {
			t.Fatalf("expected non-ErrNotFound error, got: %v", err)
		}
	})
}

func TestRepositoryUpdate(t *testing.T) {
	repo := newMockRepo(t)
	src := &source.Source{
		ID:     uuid.New(),
		Type:   source.TypeRSS,
		Name:   "updated",
		URL:    "https://example.com/updated",
		Config: map[string]any{"feed_url": "https://feed/updated"},
	}

	t.Run("success", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`UPDATE "sources"`).WithRowsNum(1)
		err := repo.Update(context.Background(), src)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`UPDATE "sources"`).WithRowsNum(0)
		err := repo.Update(context.Background(), src)
		if !errors.Is(err, source.ErrNotFound) {
			t.Fatalf("expected source.ErrNotFound, got: %v", err)
		}
	})
}

func TestRepositoryDelete(t *testing.T) {
	repo := newMockRepo(t)
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`DELETE FROM "sources"`).WithRowsNum(1)
		err := repo.Delete(context.Background(), id)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`DELETE FROM "sources"`).WithRowsNum(0)
		err := repo.Delete(context.Background(), id)
		if !errors.Is(err, source.ErrNotFound) {
			t.Fatalf("expected source.ErrNotFound, got: %v", err)
		}
	})
}

func TestRepositoryList(t *testing.T) {
	repo := newMockRepo(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	firstID := uuid.New()
	secondID := uuid.New()

	t.Run("success_without_filter", func(t *testing.T) {
		mocket.Catcher.Reset().
			NewMock().
			WithQuery(`count(*)`).
			WithReply([]map[string]any{{"count": int64(2)}})
		mocket.Catcher.NewMock().WithQuery(`FROM "sources"`).WithReply([]map[string]any{
			{
				"id":         firstID.String(),
				"type":       "rss",
				"name":       "first",
				"url":        "https://first",
				"config":     []byte(`{"feed_url":"https://feed/first"}`),
				"created_at": now,
				"updated_at": now,
			},
			{
				"id":         secondID.String(),
				"type":       "telegram_group",
				"name":       "second",
				"url":        "https://second",
				"config":     []byte(`{"group_id":"g-2"}`),
				"created_at": now,
				"updated_at": now,
			},
		})

		out, total, err := repo.List(context.Background(), source.ListFilter{PageSize: 50, Page: 1})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if total != 2 || len(out) != 2 {
			t.Fatalf("unexpected list result: total=%d len=%d", total, len(out))
		}
	})

	t.Run("success_with_type_filter", func(t *testing.T) {
		mocket.Catcher.Reset().
			NewMock().
			WithQuery(`count(*)`).
			WithReply([]map[string]any{{"count": int64(1)}})
		mocket.Catcher.NewMock().WithQuery(`WHERE type =`).WithReply([]map[string]any{
			{
				"id":         firstID.String(),
				"type":       "rss",
				"name":       "first",
				"url":        "https://first",
				"config":     []byte(`{"feed_url":"https://feed/first"}`),
				"created_at": now,
				"updated_at": now,
			},
		})

		out, total, err := repo.List(context.Background(), source.ListFilter{
			Type:     source.TypeRSS,
			PageSize: 10,
			Page:     1,
		})
		if err != nil {
			t.Fatalf("List with filter failed: %v", err)
		}
		if total != 1 || len(out) != 1 || out[0].ID != firstID {
			t.Fatalf("unexpected filtered list: total=%d out=%+v", total, out)
		}
	})

	t.Run("count_error", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`count(*)`).WithQueryException()
		_, _, err := repo.List(context.Background(), source.ListFilter{PageSize: 50, Page: 1})
		if err == nil {
			t.Fatal("expected count error")
		}
	})

	t.Run("find_error", func(t *testing.T) {
		mocket.Catcher.Reset().
			NewMock().
			WithQuery(`count(*)`).
			WithReply([]map[string]any{{"count": int64(1)}})
		mocket.Catcher.NewMock().WithQuery(`FROM "sources"`).WithQueryException()
		_, _, err := repo.List(context.Background(), source.ListFilter{PageSize: 50, Page: 1})
		if err == nil {
			t.Fatal("expected find error")
		}
	})
}

func newMockRepo(t *testing.T) *Repository {
	t.Helper()

	mocket.Catcher.Register()
	mocket.Catcher.Logging = false
	mocket.Catcher.Reset()

	sqlDB, err := sql.Open(mocket.DriverName, "connection_string")
	if err != nil {
		t.Fatalf("sql.Open mock: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open mock: %v", err)
	}

	return New(db)
}
