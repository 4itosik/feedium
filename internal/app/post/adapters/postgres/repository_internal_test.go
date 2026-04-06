package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"feedium/internal/app/post"

	mocket "github.com/Selvatico/go-mocket"
	"github.com/google/uuid"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRepositoryMappingHelpers(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	p := &post.Post{
		ID:          uuid.New(),
		SourceID:    uuid.New(),
		Title:       "title",
		Content:     "content",
		Author:      "author",
		PublishedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	row := fromDomain(p)
	if row.ID != p.ID || row.SourceID != p.SourceID || row.Title != p.Title || row.Content != p.Content ||
		row.Author != p.Author {
		t.Fatalf("fromDomain basic fields mismatch: %+v", row)
	}

	back := toDomain(&row)
	if back.ID != p.ID || back.SourceID != p.SourceID || back.Title != p.Title || back.Content != p.Content ||
		back.Author != p.Author {
		t.Fatalf("toDomain basic fields mismatch: %+v", back)
	}

	dst := &post.Post{}
	applyRow(dst, &row)
	if dst.ID != row.ID || dst.CreatedAt != row.CreatedAt || dst.UpdatedAt != row.UpdatedAt {
		t.Fatalf("applyRow mismatch: %+v", dst)
	}
}

func TestRepositoryCreate(t *testing.T) {
	repo := newMockRepo(t)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		p := &post.Post{
			SourceID:    uuid.New(),
			Title:       "title",
			Content:     "content",
			Author:      "author",
			PublishedAt: time.Now().UTC(),
		}
		mocket.Catcher.Reset().NewMock().WithQuery(`INSERT INTO "posts"`).WithRowsNum(1)
		err := repo.Create(ctx, p)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	})

	t.Run("fk_violation", func(t *testing.T) {
		p := &post.Post{
			SourceID:    uuid.New(),
			Title:       "title",
			Content:     "content",
			PublishedAt: time.Now().UTC(),
		}
		mocket.Catcher.Reset().NewMock().
			WithQuery(`INSERT INTO "posts"`).
			WithError(errors.New("ERROR: insert or update on table \"posts\" violates foreign key constraint \"fk_posts_source_id\" (SQLSTATE 23503)"))
		err := repo.Create(ctx, p)
		var v post.ValidationError
		if !errors.As(err, &v) {
			t.Fatalf("expected ValidationError for FK violation, got: %v", err)
		}
		if v.Msg != "invalid source_id: source not found" {
			t.Fatalf("unexpected error message: %q", v.Msg)
		}
	})
}

func TestRepositoryGetByID(t *testing.T) {
	repo := newMockRepo(t)
	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("success", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`FROM "posts"`).WithReply([]map[string]any{
			{
				"id":           id.String(),
				"source_id":    uuid.New().String(),
				"title":        "title",
				"content":      "content",
				"author":       "author",
				"published_at": now,
				"created_at":   now,
				"updated_at":   now,
			},
		})
		got, err := repo.GetByID(context.Background(), id)
		if err != nil {
			t.Fatalf("GetByID failed: %v", err)
		}
		if got.ID != id || got.Title != "title" {
			t.Fatalf("unexpected post: %+v", got)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`FROM "posts"`).WithReply([]map[string]any{})
		_, err := repo.GetByID(context.Background(), id)
		if !errors.Is(err, post.ErrNotFound) {
			t.Fatalf("expected post.ErrNotFound, got: %v", err)
		}
	})

	t.Run("query_error", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`FROM "posts"`).WithQueryException()
		_, err := repo.GetByID(context.Background(), id)
		if err == nil {
			t.Fatal("expected query error")
		}
		if errors.Is(err, post.ErrNotFound) {
			t.Fatalf("expected non-ErrNotFound error, got: %v", err)
		}
	})
}

func TestRepositoryUpdate(t *testing.T) {
	repo := newMockRepo(t)
	ctx := context.Background()
	p := &post.Post{
		ID:          uuid.New(),
		SourceID:    uuid.New(),
		Title:       "updated",
		Content:     "updated content",
		Author:      "author",
		PublishedAt: time.Now().UTC(),
	}

	t.Run("success", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`UPDATE "posts"`).WithRowsNum(1)
		err := repo.Update(ctx, p)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`UPDATE "posts"`).WithRowsNum(0)
		err := repo.Update(ctx, p)
		if !errors.Is(err, post.ErrNotFound) {
			t.Fatalf("expected post.ErrNotFound, got: %v", err)
		}
	})

	t.Run("fk_violation", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().
			WithQuery(`UPDATE "posts"`).
			WithError(errors.New("ERROR: update on table \"posts\" violates foreign key constraint \"fk_posts_source_id\" (SQLSTATE 23503)"))
		err := repo.Update(ctx, p)
		var v post.ValidationError
		if !errors.As(err, &v) {
			t.Fatalf("expected ValidationError for FK violation, got: %v", err)
		}
	})
}

func TestRepositoryDelete(t *testing.T) {
	repo := newMockRepo(t)
	ctx := context.Background()
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`DELETE FROM "posts"`).WithRowsNum(1)
		err := repo.Delete(ctx, id)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`DELETE FROM "posts"`).WithRowsNum(0)
		err := repo.Delete(ctx, id)
		if !errors.Is(err, post.ErrNotFound) {
			t.Fatalf("expected post.ErrNotFound, got: %v", err)
		}
	})
}

func assertListResult(t *testing.T, err error, total, wantTotal int, out []post.Post, wantLen int) {
	t.Helper()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if total != wantTotal || len(out) != wantLen {
		t.Fatalf("unexpected list result: total=%d len=%d", total, len(out))
	}
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
		mocket.Catcher.NewMock().WithQuery(`FROM "posts"`).WithReply([]map[string]any{
			{
				"id":           firstID.String(),
				"source_id":    uuid.New().String(),
				"title":        "first",
				"content":      "content1",
				"author":       "author1",
				"published_at": now,
				"created_at":   now,
				"updated_at":   now,
			},
			{
				"id":           secondID.String(),
				"source_id":    uuid.New().String(),
				"title":        "second",
				"content":      "content2",
				"author":       "author2",
				"published_at": now,
				"created_at":   now,
				"updated_at":   now,
			},
		})

		out, total, err := repo.List(context.Background(), post.ListFilter{PageSize: 50, Page: 1})
		assertListResult(t, err, total, 2, out, 2)
	})

	t.Run("success_with_published_after", func(t *testing.T) {
		publishedAfter := now.Add(-24 * time.Hour)
		mocket.Catcher.Reset().
			NewMock().
			WithQuery(`count(*)`).
			WithReply([]map[string]any{{"count": int64(1)}})
		mocket.Catcher.NewMock().WithQuery(`published_at >=`).WithReply([]map[string]any{
			{
				"id":           firstID.String(),
				"source_id":    uuid.New().String(),
				"title":        "first",
				"content":      "content1",
				"author":       "author1",
				"published_at": now,
				"created_at":   now,
				"updated_at":   now,
			},
		})

		out, total, err := repo.List(context.Background(), post.ListFilter{
			PublishedAfter: &publishedAfter,
			PageSize:       50,
			Page:           1,
		})
		assertListResult(t, err, total, 1, out, 1)
	})

	t.Run("success_with_published_before", func(t *testing.T) {
		publishedBefore := now.Add(24 * time.Hour)
		mocket.Catcher.Reset().
			NewMock().
			WithQuery(`count(*)`).
			WithReply([]map[string]any{{"count": int64(1)}})
		mocket.Catcher.NewMock().WithQuery(`published_at <`).WithReply([]map[string]any{
			{
				"id":           secondID.String(),
				"source_id":    uuid.New().String(),
				"title":        "second",
				"content":      "content2",
				"author":       "author2",
				"published_at": now,
				"created_at":   now,
				"updated_at":   now,
			},
		})

		out, total, err := repo.List(context.Background(), post.ListFilter{
			PublishedBefore: &publishedBefore,
			PageSize:        50,
			Page:            1,
		})
		assertListResult(t, err, total, 1, out, 1)
	})

	t.Run("success_with_both_filters", func(t *testing.T) {
		publishedAfter := now.Add(-48 * time.Hour)
		publishedBefore := now.Add(48 * time.Hour)
		mocket.Catcher.Reset().
			NewMock().
			WithQuery(`count(*)`).
			WithReply([]map[string]any{{"count": int64(1)}})
		mocket.Catcher.NewMock().WithQuery(`published_at >=`).WithReply([]map[string]any{
			{
				"id":           firstID.String(),
				"source_id":    uuid.New().String(),
				"title":        "first",
				"content":      "content1",
				"author":       "author1",
				"published_at": now,
				"created_at":   now,
				"updated_at":   now,
			},
		})

		out, total, err := repo.List(context.Background(), post.ListFilter{
			PublishedAfter:  &publishedAfter,
			PublishedBefore: &publishedBefore,
			PageSize:        50,
			Page:            1,
		})
		assertListResult(t, err, total, 1, out, 1)
	})

	t.Run("empty_result", func(t *testing.T) {
		mocket.Catcher.Reset().
			NewMock().
			WithQuery(`count(*)`).
			WithReply([]map[string]any{{"count": int64(0)}})
		mocket.Catcher.NewMock().WithQuery(`FROM "posts"`).WithReply([]map[string]any{})

		out, total, err := repo.List(context.Background(), post.ListFilter{PageSize: 50, Page: 1})
		assertListResult(t, err, total, 0, out, 0)
	})

	t.Run("count_error", func(t *testing.T) {
		mocket.Catcher.Reset().NewMock().WithQuery(`count(*)`).WithQueryException()
		_, _, err := repo.List(context.Background(), post.ListFilter{PageSize: 50, Page: 1})
		if err == nil {
			t.Fatal("expected count error")
		}
	})

	t.Run("find_error", func(t *testing.T) {
		mocket.Catcher.Reset().
			NewMock().
			WithQuery(`count(*)`).
			WithReply([]map[string]any{{"count": int64(1)}})
		mocket.Catcher.NewMock().WithQuery(`FROM "posts"`).WithQueryException()
		_, _, err := repo.List(context.Background(), post.ListFilter{PageSize: 50, Page: 1})
		if err == nil {
			t.Fatal("expected find error")
		}
	})
}

func TestMapError(t *testing.T) {
	t.Run("nil_error", func(t *testing.T) {
		if err := mapError(nil); err != nil {
			t.Fatalf("expected nil, got: %v", err)
		}
	})

	t.Run("fk_violation", func(t *testing.T) {
		err := mapError(
			errors.New("ERROR: insert or update on table \"posts\" violates foreign key constraint (SQLSTATE 23503)"),
		)
		var v post.ValidationError
		if !errors.As(err, &v) {
			t.Fatalf("expected ValidationError, got: %v", err)
		}
	})

	t.Run("other_error", func(t *testing.T) {
		original := errors.New("some error")
		err := mapError(original)
		if !errors.Is(err, original) {
			t.Fatalf("expected original error, got: %v", err)
		}
	})
}

func TestContains(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"", "", true},
		{"hello", "ell", true},
		{"hello", "xyz", false},
		{"hello world", "world", true},
		{"hello", "hello world", false},
	}

	for _, tc := range tests {
		got := contains(tc.s, tc.substr)
		if got != tc.want {
			t.Errorf("contains(%q, %q) = %v, want %v", tc.s, tc.substr, got, tc.want)
		}
	}
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
