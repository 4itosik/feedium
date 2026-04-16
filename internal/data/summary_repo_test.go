package data_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
	entgo "github.com/4itosik/feedium/internal/ent"
)

func createTestPost(ctx context.Context, t *testing.T, client *entgo.Client, sourceID string) string {
	t.Helper()
	now := time.Now()
	post := biz.Post{
		ID:          uuid.Must(uuid.NewV7()).String(),
		SourceID:    sourceID,
		ExternalID:  "ext-summary-test-" + uuid.Must(uuid.NewV7()).String(),
		PublishedAt: now,
		Text:        "Test post for summary",
		Metadata:    map[string]string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	saved, _, err := data.NewPostRepo(&data.Data{Ent: client}).Save(ctx, post)
	require.NoError(t, err)
	return saved.ID
}

func TestIntegration_SummaryRepo_SaveAndGet(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSummaryRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)

	id := uuid.Must(uuid.NewV7()).String()
	summary := biz.Summary{
		ID:        id,
		SourceID:  sourceID,
		Text:      "This is a test summary.",
		WordCount: 5,
		CreatedAt: time.Now(),
	}

	saved, err := repo.Save(ctx, summary)
	require.NoError(t, err)
	assert.Equal(t, id, saved.ID)
	assert.Equal(t, sourceID, saved.SourceID)
	assert.Equal(t, "This is a test summary.", saved.Text)
	assert.Equal(t, 5, saved.WordCount)
	assert.Nil(t, saved.PostID)

	fetched, err := repo.Get(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, fetched.ID)
	assert.Equal(t, saved.SourceID, fetched.SourceID)
	assert.Equal(t, saved.Text, fetched.Text)
	assert.Equal(t, saved.WordCount, fetched.WordCount)
	assert.WithinDuration(t, saved.CreatedAt, fetched.CreatedAt, time.Second)
}

func TestIntegration_SummaryRepo_Save_SelfContained(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSummaryRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)
	postID := createTestPost(ctx, t, client, sourceID)

	id := uuid.Must(uuid.NewV7()).String()
	summary := biz.Summary{
		ID:        id,
		PostID:    &postID,
		SourceID:  sourceID,
		Text:      "Self-contained summary.",
		WordCount: 3,
		CreatedAt: time.Now(),
	}

	saved, err := repo.Save(ctx, summary)
	require.NoError(t, err)
	assert.Equal(t, id, saved.ID)
	assert.NotNil(t, saved.PostID)
	assert.Equal(t, postID, *saved.PostID)
	assert.Equal(t, sourceID, saved.SourceID)
}

func TestIntegration_SummaryRepo_Save_Cumulative(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSummaryRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)

	id := uuid.Must(uuid.NewV7()).String()
	summary := biz.Summary{
		ID:        id,
		PostID:    nil,
		SourceID:  sourceID,
		Text:      "Cumulative summary.",
		WordCount: 2,
		CreatedAt: time.Now(),
	}

	saved, err := repo.Save(ctx, summary)
	require.NoError(t, err)
	assert.Equal(t, id, saved.ID)
	assert.Nil(t, saved.PostID)
	assert.Equal(t, sourceID, saved.SourceID)
}

func TestIntegration_SummaryRepo_ListByPost(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSummaryRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)
	postID := createTestPost(ctx, t, client, sourceID)

	baseTime := time.Now()
	var savedIDs []string
	for i := range 3 {
		id := uuid.Must(uuid.NewV7()).String()
		s, err := repo.Save(ctx, biz.Summary{
			ID:        id,
			PostID:    &postID,
			SourceID:  sourceID,
			Text:      "Summary " + string(rune('A'+i)),
			WordCount: i + 1,
			CreatedAt: baseTime.Add(time.Duration(i) * time.Second),
		})
		require.NoError(t, err)
		savedIDs = append(savedIDs, s.ID)
	}

	result, err := repo.ListByPost(ctx, postID)
	require.NoError(t, err)
	assert.Len(t, result, 3)

	assert.Equal(t, savedIDs[2], result[0].ID)
	assert.Equal(t, savedIDs[1], result[1].ID)
	assert.Equal(t, savedIDs[0], result[2].ID)

	for _, s := range result {
		assert.Equal(t, postID, *s.PostID)
		assert.Equal(t, sourceID, s.SourceID)
	}
}

func TestIntegration_SummaryRepo_ListBySource_Pagination(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSummaryRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)

	baseTime := time.Now()
	for i := range 5 {
		_, err := repo.Save(ctx, biz.Summary{
			ID:        uuid.Must(uuid.NewV7()).String(),
			SourceID:  sourceID,
			Text:      "Summary " + string(rune('A'+i)),
			WordCount: i + 1,
			CreatedAt: baseTime.Add(time.Duration(i) * time.Second),
		})
		require.NoError(t, err)
	}

	page1, err := repo.ListBySource(ctx, sourceID, 2, "")
	require.NoError(t, err)
	assert.Len(t, page1.Items, 2)
	assert.NotEmpty(t, page1.NextPageToken)

	page2, err := repo.ListBySource(ctx, sourceID, 2, page1.NextPageToken)
	require.NoError(t, err)
	assert.Len(t, page2.Items, 2)
	assert.NotEmpty(t, page2.NextPageToken)

	page3, err := repo.ListBySource(ctx, sourceID, 2, page2.NextPageToken)
	require.NoError(t, err)
	assert.Len(t, page3.Items, 1)
	assert.Empty(t, page3.NextPageToken)

	var totalCount int
	for _, p := range []biz.ListSummariesResult{page1, page2, page3} {
		totalCount += len(p.Items)
	}
	assert.Equal(t, 5, totalCount)
}

func TestIntegration_SummaryRepo_GetLastBySource(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSummaryRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)

	baseTime := time.Now()
	_, err := repo.Save(ctx, biz.Summary{
		ID:        uuid.Must(uuid.NewV7()).String(),
		SourceID:  sourceID,
		Text:      "Older summary",
		WordCount: 2,
		CreatedAt: baseTime,
	})
	require.NoError(t, err)

	lastID := uuid.Must(uuid.NewV7()).String()
	_, err = repo.Save(ctx, biz.Summary{
		ID:        lastID,
		SourceID:  sourceID,
		Text:      "Newer summary",
		WordCount: 2,
		CreatedAt: baseTime.Add(1 * time.Hour),
	})
	require.NoError(t, err)

	last, err := repo.GetLastBySource(ctx, sourceID)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, lastID, last.ID)
	assert.Equal(t, "Newer summary", last.Text)
}

func TestIntegration_SummaryRepo_GetLastBySource_NoSummary(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSummaryRepo(&data.Data{Ent: client})
	sourceID := createTestSource(ctx, t, client)

	last, err := repo.GetLastBySource(ctx, sourceID)
	require.NoError(t, err)
	assert.Nil(t, last)
}

func TestIntegration_SummaryRepo_Get_NotFound(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := data.NewSummaryRepo(&data.Data{Ent: client})

	_, err := repo.Get(ctx, "00000000-0000-0000-0000-000000000001")
	assert.ErrorIs(t, err, biz.ErrSummaryNotFound)
}
