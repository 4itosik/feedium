package data_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
)

// seedDigestSummary persists a cumulative (source-level) summary with post_id NULL —
// the digest entity FT-020 scores independently — and returns the saved summary.
func seedDigestSummary(
	t *testing.T,
	d *data.Data,
	sourceID, text string,
	createdAt time.Time,
) biz.Summary {
	t.Helper()
	repo := data.NewSummaryRepo(d)
	saved, err := repo.Save(context.Background(), biz.Summary{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    nil, // cumulative digest: no owning post
		SourceID:  sourceID,
		Text:      text,
		WordCount: 3,
		CreatedAt: createdAt,
	})
	require.NoError(t, err)
	return saved
}

// newDigestScore builds a Score keyed by summary_id (the digest scoring path).
func newDigestScore(summaryID string, value float64) biz.Score {
	now := time.Now()
	return biz.Score{
		ID:        uuid.Must(uuid.NewV7()).String(),
		SummaryID: new(summaryID),
		Value:     value,
		Model:     "test-model",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// TestIntegration_ScoreRepo_UpsertBySummaryID covers CTR-01: a digest score is keyed
// by summary_id, with post_id NULL, and the single-row upsert works via the summary
// partial unique index.
func TestIntegration_ScoreRepo_UpsertBySummaryID(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreRepo(d)
	sourceID := seedSource(t, d)
	digest := seedDigestSummary(t, d, sourceID, "digest body", time.Now())

	saved, err := repo.Upsert(ctx, newDigestScore(digest.ID, 0.42))
	require.NoError(t, err)
	require.NotNil(t, saved.SummaryID)
	assert.Equal(t, digest.ID, *saved.SummaryID)
	assert.Nil(t, saved.PostID)
	assert.InDelta(t, 0.42, saved.Value, 1e-9)

	// Idempotent overwrite via the summary partial unique index.
	updated, err := repo.Upsert(ctx, newDigestScore(digest.ID, 0.77))
	require.NoError(t, err)
	assert.InDelta(t, 0.77, updated.Value, 1e-9)

	var count int
	require.NoError(t, d.DB.QueryRowContext(ctx,
		"SELECT count(*) FROM scores WHERE summary_id = $1", digest.ID).Scan(&count))
	assert.Equal(t, 1, count, "exactly one current score per digest")
}

// TestIntegration_ScoreRepo_ListRanked_IncludesDigest covers SC-01, EC-02, CTR-03:
// a digest score appears in the ranked feed with kind=digest, the summary text and
// created_at, the summary id as identity, and empty author/url/reaction.
func TestIntegration_ScoreRepo_ListRanked_IncludesDigest(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreRepo(d)
	sourceID := seedSource(t, d)
	createdAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	digest := seedDigestSummary(t, d, sourceID, "cumulative digest text", createdAt)
	_, err := repo.Upsert(ctx, newDigestScore(digest.ID, 0.6))
	require.NoError(t, err)

	feed, err := repo.ListRanked(ctx, biz.FeedQuery{PageSize: 10})
	require.NoError(t, err)
	require.Len(t, feed.Items, 1)

	item := feed.Items[0]
	assert.Equal(t, biz.FeedItemKindDigest, item.Kind)
	assert.Equal(t, digest.ID, item.Post.ID, "digest identity is the summary id (CTR-03)")
	assert.Equal(t, sourceID, item.Post.SourceID)
	assert.Equal(t, "cumulative digest text", item.Post.Text)
	assert.InDelta(t, 0.6, item.Score, 1e-9)
	assert.True(t, createdAt.Equal(item.Post.PublishedAt), "publishedAt is the summary created_at")
	assert.Nil(t, item.Post.Author, "digest has no author")
	assert.Empty(t, item.URL, "digest has no url")
	assert.Empty(t, item.Reaction, "digest has no reaction")
}

// TestIntegration_ScoreRepo_ListRanked_MixedKeysetPagination covers SC-02: posts and
// digests interleave by score under a single keyset cursor with no duplicates or gaps.
// No reactions are used so effective_value == value (keyset/order consistency).
func TestIntegration_ScoreRepo_ListRanked_MixedKeysetPagination(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreRepo(d)
	sourceID := seedSource(t, d)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Interleaved distinct scores: posts 0.9/0.7/0.5, digests 0.8/0.6/0.4.
	type want struct {
		id    string
		value float64
		kind  biz.FeedItemKind
	}
	var expected []want
	for i, v := range []float64{0.9, 0.7, 0.5} {
		pid := seedPost(
			t,
			d,
			sourceID,
			"ext-p"+string(rune('a'+i)),
			base.Add(time.Duration(i)*time.Hour),
		)
		_, err := repo.Upsert(ctx, newScore(pid, v))
		require.NoError(t, err)
		expected = append(expected, want{pid, v, biz.FeedItemKindPost})
	}
	for i, v := range []float64{0.8, 0.6, 0.4} {
		digest := seedDigestSummary(
			t,
			d,
			sourceID,
			"digest "+string(rune('a'+i)),
			base.Add(time.Duration(i)*time.Hour),
		)
		_, err := repo.Upsert(ctx, newDigestScore(digest.ID, v))
		require.NoError(t, err)
		expected = append(expected, want{digest.ID, v, biz.FeedItemKindDigest})
	}
	sort.Slice(expected, func(i, j int) bool { return expected[i].value > expected[j].value })

	// Page through with PageSize=2, collecting every item across pages.
	var got []biz.FeedItem
	token := ""
	for range 10 {
		page, err := repo.ListRanked(ctx, biz.FeedQuery{PageSize: 2, PageToken: token})
		require.NoError(t, err)
		got = append(got, page.Items...)
		token = page.NextPageToken
		if token == "" {
			break
		}
	}

	require.Len(t, got, len(expected), "no duplicates and no omissions across the union")
	seen := map[string]bool{}
	sawPost, sawDigest := false, false
	for i, item := range got {
		assert.False(t, seen[item.Post.ID], "no duplicate id %s", item.Post.ID)
		seen[item.Post.ID] = true
		assert.Equal(t, expected[i].id, item.Post.ID, "position %d id", i)
		assert.InDelta(t, expected[i].value, item.Score, 1e-9, "position %d score", i)
		assert.Equal(t, expected[i].kind, item.Kind, "position %d kind", i)
		switch item.Kind {
		case biz.FeedItemKindPost:
			sawPost = true
		case biz.FeedItemKindDigest:
			sawDigest = true
		}
	}
	assert.True(t, sawPost && sawDigest, "both posts and digests appear in the feed")
}

// TestIntegration_PostEmbeddingRepo_DigestKeyRoundTrip covers REQ-03/CTR-01: a digest
// embedding is persisted and read back by summary_id.
func TestIntegration_PostEmbeddingRepo_DigestKeyRoundTrip(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewPostEmbeddingRepo(d)
	sourceID := seedSource(t, d)
	digest := seedDigestSummary(t, d, sourceID, "digest for embedding", time.Now())

	_, err := repo.Upsert(ctx, biz.PostEmbedding{
		SummaryID: new(digest.ID),
		Vector:    []float32{0.5, -0.5, 0.25},
		Model:     "test-embed-model",
		InputHash: "digest-hash",
	})
	require.NoError(t, err)

	got, err := repo.Get(ctx, biz.ScoreKey{SummaryID: new(digest.ID)})
	require.NoError(t, err)
	require.NotNil(t, got.SummaryID)
	assert.Equal(t, digest.ID, *got.SummaryID)
	assert.Nil(t, got.PostID)
	assert.Equal(t, []float32{0.5, -0.5, 0.25}, got.Vector)
	assert.Equal(t, "digest-hash", got.InputHash)
}

// TestIntegration_ScoreOutbox_EnqueueDueDigests covers CTR-04/REQ-01: an unscored
// cumulative digest is enqueued exactly once (idempotent), a post-linked summary is
// not enqueued by the digest path, and the claimed event carries summary_id.
func TestIntegration_ScoreOutbox_EnqueueDueDigests(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	ctx := context.Background()
	d, cleanup := setupTestData(t)
	defer cleanup()
	repo := data.NewScoreOutboxRepo(d)
	sourceID := seedSource(t, d)
	digest := seedDigestSummary(t, d, sourceID, "due digest", time.Now())
	// A post-linked summary must NOT be picked up by the digest enqueue path.
	postID := seedPost(t, d, sourceID, "ext-1", time.Now())
	seedSummary(t, d, sourceID, postID)

	first, err := repo.EnqueueDueDigests(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, first, "the unscored digest is enqueued")

	second, err := repo.EnqueueDueDigests(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, second, "an active event blocks re-enqueue (INV-04)")

	var digestEvents, postEvents int
	require.NoError(t, d.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM score_events WHERE summary_id = $1`, digest.ID).Scan(&digestEvents))
	assert.Equal(t, 1, digestEvents, "exactly one active digest event")
	require.NoError(t, d.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM score_events WHERE post_id = $1`, postID).Scan(&postEvents))
	assert.Equal(t, 0, postEvents, "digest enqueue does not touch post-linked summaries")

	claimed, err := repo.ClaimOne(ctx, "worker-1", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, claimed.SummaryID)
	assert.Equal(t, digest.ID, *claimed.SummaryID)
	assert.Nil(t, claimed.PostID)
}
