package data_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
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

// queryCounter is a wrapper around Ent driver that counts SQL queries.
type queryCounter struct {
	dialect.Driver

	count int
}

func (c *queryCounter) Query(ctx context.Context, query string, args, v any) error {
	c.count++
	return c.Driver.Query(ctx, query, args, v)
}

func (c *queryCounter) Exec(ctx context.Context, query string, args, v any) error {
	c.count++
	return c.Driver.Exec(ctx, query, args, v)
}

// setupTestDBWithCounter creates test DB and returns both regular client and counter client.
func setupTestDBWithCounter(t *testing.T) (*entgo.Client, *entgo.Client, *queryCounter, func()) {
	t.Helper()

	// Use same pattern as setupTestDB from source_repo_test.go
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

	// We need to run migrations using regular DB
	err = goose.Up(db, "../../migrations")
	require.NoError(t, err)

	// Create regular client
	drv := entsql.OpenDB(dialect.Postgres, db)
	entClient := entgo.NewClient(entgo.Driver(drv))

	// Create counter client (shares same underlying db connection)
	counter := &queryCounter{Driver: drv}
	counterClient := entgo.NewClient(entgo.Driver(counter))

	cleanup := func() {
		if closeErr := counterClient.Close(); closeErr != nil {
			t.Logf("failed to close counter client: %v", closeErr)
		}
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

	return entClient, counterClient, counter, cleanup
}

// createTestSource creates a minimal source for FK constraint.
func createTestSource(ctx context.Context, t *testing.T, client *entgo.Client) string {
	t.Helper()
	config := &biz.RSSConfig{FeedURL: "https://test.example.com/feed"}
	now := time.Now()

	repo := data.NewSourceRepo(&data.Data{Ent: client})
	saved, err := repo.Save(ctx, biz.Source{
		Type:      biz.SourceTypeRSS,
		Config:    config,
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	return saved.ID
}

func TestIntegration_PostSave(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	postRepo := data.NewPostRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)

	t.Run("save valid post", func(t *testing.T) {
		now := time.Now()
		author := "Test Author"
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-123",
			PublishedAt: now,
			Author:      &author,
			Text:        "Test post content",
			Metadata:    map[string]string{"key": "value"},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		saved, err := postRepo.Save(ctx, post)
		require.NoError(t, err)
		assert.Equal(t, post.ID, saved.ID)
		assert.Equal(t, sourceID, saved.SourceID)
		assert.Equal(t, post.ExternalID, saved.ExternalID)
		assert.Equal(t, post.Text, saved.Text)
		assert.Equal(t, post.Metadata, saved.Metadata)
		assert.NotNil(t, saved.Source)
		assert.Equal(t, sourceID, saved.Source.ID)
		assert.Equal(t, biz.SourceTypeRSS, saved.Source.Type)
	})

	t.Run("save post without author", func(t *testing.T) {
		now := time.Now()
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-no-author",
			PublishedAt: now,
			Author:      nil,
			Text:        "Post without author",
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		saved, err := postRepo.Save(ctx, post)
		require.NoError(t, err)
		assert.Nil(t, saved.Author)
	})

	t.Run("save post with nil metadata defaults to empty map", func(t *testing.T) {
		now := time.Now()
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-nil-metadata",
			PublishedAt: now,
			Text:        "Post with nil metadata",
			Metadata:    nil,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		saved, err := postRepo.Save(ctx, post)
		require.NoError(t, err)
		assert.NotNil(t, saved.Metadata)
	})

	t.Run("save post with non-existent source", func(t *testing.T) {
		now := time.Now()
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d", // Non-existent UUID
			ExternalID:  "ext-404",
			PublishedAt: now,
			Text:        "Post with bad source",
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		_, err := postRepo.Save(ctx, post)
		assert.ErrorIs(t, err, biz.ErrPostSourceNotFound)
	})

	t.Run("idempotent save returns existing post", func(t *testing.T) {
		now := time.Now()
		externalID := "ext-idempotent"
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  externalID,
			PublishedAt: now,
			Text:        "Original text",
			Metadata:    map[string]string{"version": "1"},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		// First save
		first, err := postRepo.Save(ctx, post)
		require.NoError(t, err)

		// Second save with same (source_id, external_id) but different text
		post2 := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(), // Different ID
			SourceID:    sourceID,
			ExternalID:  externalID, // Same external_id
			PublishedAt: now,
			Text:        "Different text", // Different text
			Metadata:    map[string]string{"version": "2"},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		second, err := postRepo.Save(ctx, post2)
		require.NoError(t, err)

		// Should return the first post without modification
		assert.Equal(t, first.ID, second.ID)
		assert.Equal(t, first.Text, second.Text)
		assert.True(t, first.UpdatedAt.Equal(second.UpdatedAt))
	})
}

func TestIntegration_PostUpdate(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	postRepo := data.NewPostRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)

	t.Run("update valid post", func(t *testing.T) {
		now := time.Now().UTC()
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-update",
			PublishedAt: now,
			Text:        "Original text",
			Metadata:    map[string]string{"key": "original"},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		saved, err := postRepo.Save(ctx, post)
		require.NoError(t, err)

		// Small delay to ensure UpdatedAt changes
		time.Sleep(10 * time.Millisecond)

		newAuthor := "Updated Author"
		updatedPost := biz.Post{
			ID:          saved.ID,
			SourceID:    sourceID,
			ExternalID:  "updated-ext",
			PublishedAt: now.Add(1 * time.Hour),
			Author:      &newAuthor,
			Text:        "Updated text",
			Metadata:    map[string]string{"key": "updated"},
			CreatedAt:   saved.CreatedAt,
			UpdatedAt:   time.Now().UTC(),
		}

		updated, err := postRepo.Update(ctx, updatedPost)
		require.NoError(t, err)
		assert.Equal(t, "updated-ext", updated.ExternalID)
		assert.Equal(t, "Updated text", updated.Text)
		assert.Equal(t, "Updated Author", *updated.Author)
		assert.True(t, updated.CreatedAt.Equal(saved.CreatedAt)) // CreatedAt unchanged
		assert.True(t, updated.UpdatedAt.After(saved.UpdatedAt))
		assert.Equal(t, saved.SourceID, updated.SourceID) // SourceID unchanged
	})

	t.Run("update not found", func(t *testing.T) {
		now := time.Now()
		post := biz.Post{
			ID:          "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d",
			SourceID:    sourceID,
			ExternalID:  "ext-404",
			PublishedAt: now,
			Text:        "Text",
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		_, err := postRepo.Update(ctx, post)
		assert.ErrorIs(t, err, biz.ErrPostNotFound)
	})

	t.Run("update clears author when nil", func(t *testing.T) {
		now := time.Now()
		author := "Original Author"
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-clear-author",
			PublishedAt: now,
			Author:      &author,
			Text:        "Text",
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		saved, err := postRepo.Save(ctx, post)
		require.NoError(t, err)
		assert.NotNil(t, saved.Author)

		updatedPost := biz.Post{
			ID:          saved.ID,
			SourceID:    sourceID,
			ExternalID:  "updated",
			PublishedAt: now,
			Author:      nil, // Clear author
			Text:        "Updated text",
			Metadata:    map[string]string{},
			CreatedAt:   saved.CreatedAt,
			UpdatedAt:   time.Now(),
		}

		updated, err := postRepo.Update(ctx, updatedPost)
		require.NoError(t, err)
		assert.Nil(t, updated.Author)
	})

	t.Run("update conflict external_id same source", func(t *testing.T) {
		now := time.Now()

		// Create first post
		post1 := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-conflict-1",
			PublishedAt: now,
			Text:        "First post",
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err := postRepo.Save(ctx, post1)
		require.NoError(t, err)

		// Create second post with different external_id
		post2 := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-conflict-2",
			PublishedAt: now,
			Text:        "Second post",
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err = postRepo.Save(ctx, post2)
		require.NoError(t, err)

		// Try to update second post to use first post's external_id
		post2.ExternalID = "ext-conflict-1"
		_, err = postRepo.Update(ctx, post2)
		assert.ErrorIs(t, err, biz.ErrPostAlreadyExists)
	})
}

func TestIntegration_PostDelete(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	postRepo := data.NewPostRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)

	t.Run("delete existing post", func(t *testing.T) {
		now := time.Now()
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-delete",
			PublishedAt: now,
			Text:        "To be deleted",
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		saved, err := postRepo.Save(ctx, post)
		require.NoError(t, err)

		err = postRepo.Delete(ctx, saved.ID)
		require.NoError(t, err)

		_, err = postRepo.Get(ctx, saved.ID)
		assert.ErrorIs(t, err, biz.ErrPostNotFound)
	})

	t.Run("delete not found", func(t *testing.T) {
		err := postRepo.Delete(ctx, "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d")
		assert.ErrorIs(t, err, biz.ErrPostNotFound)
	})
}

func TestIntegration_PostGet(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	postRepo := data.NewPostRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)

	t.Run("get existing post with SourceInfo", func(t *testing.T) {
		now := time.Now()
		author := "Test Author"
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-get",
			PublishedAt: now,
			Author:      &author,
			Text:        "Test content",
			Metadata:    map[string]string{"key": "value"},
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		saved, err := postRepo.Save(ctx, post)
		require.NoError(t, err)

		fetched, err := postRepo.Get(ctx, saved.ID)
		require.NoError(t, err)
		assert.Equal(t, saved.ID, fetched.ID)
		assert.Equal(t, saved.SourceID, fetched.SourceID)
		assert.Equal(t, saved.ExternalID, fetched.ExternalID)
		assert.Equal(t, saved.Text, fetched.Text)
		assert.Equal(t, *saved.Author, *fetched.Author)
		assert.NotNil(t, fetched.Source)
		assert.Equal(t, sourceID, fetched.Source.ID)
		assert.Equal(t, biz.SourceTypeRSS, fetched.Source.Type)
	})

	t.Run("get not found", func(t *testing.T) {
		_, err := postRepo.Get(ctx, "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d")
		assert.ErrorIs(t, err, biz.ErrPostNotFound)
	})
}

func TestIntegration_PostList(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	postRepo := data.NewPostRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)
	secondSourceID := createTestSource(ctx, t, client)

	now := time.Now()

	// Create 5 posts for first source, 2 for second source
	for i := range 5 {
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-list-" + string(rune('a'+i)),
			PublishedAt: now.Add(time.Duration(i) * time.Hour),
			Text:        "Post " + string(rune('a'+i)),
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err := postRepo.Save(ctx, post)
		require.NoError(t, err)
	}

	// Use different time range for second source to avoid overlap with first source
	// First source: now + 0,1,2,3,4 hours
	// Second source: now - 2, -1 hours (all before first source)
	for i := range 2 {
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    secondSourceID,
			ExternalID:  "ext-second-" + string(rune('a'+i)),
			PublishedAt: now.Add(-time.Duration(2-i) * time.Hour),
			Text:        "Second source post",
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err := postRepo.Save(ctx, post)
		require.NoError(t, err)
	}

	t.Run("list all posts with pagination", func(t *testing.T) {
		result, err := postRepo.List(ctx, biz.ListPostsFilter{
			PageSize: 3,
			OrderBy:  biz.SortByPublishedAt,
			OrderDir: biz.SortDesc,
		})
		require.NoError(t, err)
		assert.Len(t, result.Items, 3)
		assert.NotEmpty(t, result.NextPageToken)

		// Get next page - should have 3 items (not 4) because page_size limits max items per page
		page2, err := postRepo.List(ctx, biz.ListPostsFilter{
			PageSize:  3,
			PageToken: result.NextPageToken,
			OrderBy:   biz.SortByPublishedAt,
			OrderDir:  biz.SortDesc,
		})
		require.NoError(t, err)
		assert.Len(t, page2.Items, 3)           // page_size=3 limits to 3 items per page
		assert.NotEmpty(t, page2.NextPageToken) // there are more items

		// Get third page - should have remaining 1 item
		page3, err := postRepo.List(ctx, biz.ListPostsFilter{
			PageSize:  3,
			PageToken: page2.NextPageToken,
			OrderBy:   biz.SortByPublishedAt,
			OrderDir:  biz.SortDesc,
		})
		require.NoError(t, err)
		assert.Len(t, page3.Items, 1) // 7 - 3 - 3 = 1 remaining
		assert.Empty(t, page3.NextPageToken)
	})

	t.Run("list filter by source_id", func(t *testing.T) {
		result, err := postRepo.List(ctx, biz.ListPostsFilter{
			SourceID: sourceID,
			PageSize: 100,
			OrderBy:  biz.SortByPublishedAt,
			OrderDir: biz.SortDesc,
		})
		require.NoError(t, err)
		assert.Len(t, result.Items, 5)
		for _, item := range result.Items {
			assert.Equal(t, sourceID, item.SourceID)
		}
	})

	t.Run("list source with no posts", func(t *testing.T) {
		newSourceID := createTestSource(ctx, t, client)
		result, err := postRepo.List(ctx, biz.ListPostsFilter{
			SourceID: newSourceID,
			PageSize: 100,
			OrderBy:  biz.SortByPublishedAt,
			OrderDir: biz.SortDesc,
		})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
		assert.Empty(t, result.NextPageToken)
	})

	t.Run("list empty table", func(t *testing.T) {
		// Create new DB with new source but no posts
		freshClient, freshCleanup := setupTestDB(t)
		defer freshCleanup()
		freshRepo := data.NewPostRepo(&data.Data{Ent: freshClient})

		result, err := freshRepo.List(ctx, biz.ListPostsFilter{
			PageSize: 100,
			OrderBy:  biz.SortByPublishedAt,
			OrderDir: biz.SortDesc,
		})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
		assert.Empty(t, result.NextPageToken)
	})

	t.Run("list sort by created_at ASC", func(t *testing.T) {
		result, err := postRepo.List(ctx, biz.ListPostsFilter{
			PageSize: 100,
			OrderBy:  biz.SortByCreatedAt,
			OrderDir: biz.SortAsc,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, result.Items)
	})

	t.Run("list with page_size=1 walks all pages", func(t *testing.T) {
		var allItems []biz.Post
		token := ""

		for {
			result, err := postRepo.List(ctx, biz.ListPostsFilter{
				PageSize:  1,
				PageToken: token,
				OrderBy:   biz.SortByPublishedAt,
				OrderDir:  biz.SortDesc,
			})
			require.NoError(t, err)
			allItems = append(allItems, result.Items...)
			if result.NextPageToken == "" {
				break
			}
			token = result.NextPageToken
		}

		assert.Len(t, allItems, 7) // All posts
	})
}

func TestIntegration_PostGetSQLCount(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	// Use setupTestDBWithCounter to get both regular and counter clients
	regularClient, counterClient, counter, cleanup := setupTestDBWithCounter(t)
	defer cleanup()

	postRepo := data.NewPostRepo(&data.Data{Ent: regularClient})
	sourceID := createTestSource(ctx, t, regularClient)

	// Create a post using regular client
	now := time.Now()
	post := biz.Post{
		ID:          uuid.Must(uuid.NewV7()).String(),
		SourceID:    sourceID,
		ExternalID:  "ext-sqlcount",
		PublishedAt: now,
		Text:        "Test post for SQL count",
		Metadata:    map[string]string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	saved, err := postRepo.Save(ctx, post)
	require.NoError(t, err)

	// Reset counter to only count Get query
	counter.count = 0

	counterRepo := data.NewPostRepo(&data.Data{Ent: counterClient})

	// Execute Get
	_, err = counterRepo.Get(ctx, saved.ID)
	require.NoError(t, err)

	// Assert: GetPost executes 2 SQL queries (post + eager-loaded source)
	// Note: Ent eager-loading uses separate query, not JOIN
	assert.Equal(t, 2, counter.count,
		"GetPost should execute 2 SQL queries (post + eager-loaded source)")
}

func TestIntegration_PostListSQLCount(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	// Use setupTestDBWithCounter to get both regular and counter clients
	regularClient, counterClient, counter, cleanup := setupTestDBWithCounter(t)
	defer cleanup()

	postRepo := data.NewPostRepo(&data.Data{Ent: regularClient})
	sourceID := createTestSource(ctx, t, regularClient)

	// Create 5 posts using regular client
	now := time.Now()
	for i := range 5 {
		post := biz.Post{
			ID:          uuid.Must(uuid.NewV7()).String(),
			SourceID:    sourceID,
			ExternalID:  "ext-list-sqlcount-" + string(rune('a'+i)),
			PublishedAt: now.Add(time.Duration(i) * time.Hour),
			Text:        "Post " + string(rune('a'+i)),
			Metadata:    map[string]string{},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err := postRepo.Save(ctx, post)
		require.NoError(t, err)
	}

	// Reset counter to only count List query
	counter.count = 0

	counterRepo := data.NewPostRepo(&data.Data{Ent: counterClient})

	// Execute List with page_size=3
	_, err := counterRepo.List(ctx, biz.ListPostsFilter{
		PageSize: 3,
		OrderBy:  biz.SortByPublishedAt,
		OrderDir: biz.SortDesc,
	})
	require.NoError(t, err)

	// Assert: ListPosts should execute exactly 2 SQL queries:
	// 1 query for posts + 1 query for batch-loading sources
	assert.Equal(t, 2, counter.count,
		"ListPosts should execute exactly 2 SQL queries (posts + batch sources)")
}
