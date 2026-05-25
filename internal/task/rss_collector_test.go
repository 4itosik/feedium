package task_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/task"
)

const rssCollectorFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Feed</title>
    <item>
      <guid>https://example.com/a</guid>
      <link>https://example.com/a</link>
      <title>A</title>
      <description>Body A</description>
      <pubDate>Tue, 14 Apr 2026 12:00:00 +0000</pubDate>
    </item>
    <item>
      <guid>https://example.com/b</guid>
      <link>https://example.com/b</link>
      <title>B</title>
      <description>Body B</description>
      <pubDate>Tue, 14 Apr 2026 13:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

// recordingHandler captures error-level log records so tests can assert an
// observable error was emitted (REQ-07).
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) errorCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Level >= slog.LevelError {
			n++
		}
	}
	return n
}

func createRSSSource(ctx context.Context, t *testing.T, d *data.Data, feedURL string) string {
	t.Helper()
	repo := data.NewSourceRepo(d)
	saved, err := repo.Save(ctx, biz.Source{
		Type:           biz.SourceTypeRSS,
		ProcessingMode: biz.ProcessingModeSelfContained,
		Config:         &biz.RSSConfig{FeedURL: feedURL},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})
	require.NoError(t, err)
	return saved.ID
}

func newCollectUsecase(d *data.Data) *biz.CollectUsecase {
	return biz.NewCollectUsecase(biz.NewPostUsecase(
		data.NewPostRepo(d),
		data.NewTxManager(d),
		data.NewSummaryOutboxRepo(d),
		data.NewSourceRepo(d),
	))
}

func collectorsCfg(interval time.Duration) *conf.Collectors {
	return &conf.Collectors{
		Rss: &conf.CollectorPoll{PollInterval: durationpb.New(interval)},
		Web: &conf.CollectorPoll{PollInterval: durationpb.New(interval)},
	}
}

func countPosts(ctx context.Context, t *testing.T, d *data.Data, sourceID string) int {
	t.Helper()
	res, err := data.NewPostRepo(d).List(ctx, biz.ListPostsFilter{
		SourceID: sourceID,
		PageSize: 100,
		OrderBy:  biz.SortByCreatedAt,
		OrderDir: biz.SortDesc,
	})
	require.NoError(t, err)
	return len(res.Items)
}

func waitForPosts(ctx context.Context, t *testing.T, d *data.Data, sourceID string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if countPosts(ctx, t, d, sourceID) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.GreaterOrEqual(t, countPosts(ctx, t, d, sourceID), want, "posts did not appear in time")
}

// waitForCount polls until counter reaches want, so idempotency assertions can
// wait for several real collection cycles instead of guessing with a sleep.
func waitForCount(t *testing.T, counter *atomic.Int64, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if counter.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.GreaterOrEqual(t, counter.Load(), want, "expected collection cycles did not run in time")
}

func TestIntegration_RSSCollector_CollectsAndIsIdempotent(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(rssCollectorFixture))
	}))
	defer srv.Close()

	sourceID := createRSSSource(ctx, t, d, srv.URL)
	collector := task.NewRSSCollector(
		newCollectUsecase(d),
		data.NewRSSFetcher(),
		biz.NewSourceUsecase(data.NewSourceRepo(d)),
		collectorsCfg(30*time.Millisecond),
		testLogger(),
	)

	// Act
	require.NoError(t, collector.Start(ctx))
	waitForPosts(ctx, t, d, sourceID, 2)
	// Wait for at least three collection cycles of the unchanged feed; if
	// collection were not idempotent, duplicates would appear by now.
	waitForCount(t, &hits, 3)
	require.NoError(t, collector.Stop(ctx))

	// Assert
	assert.Equal(t, 2, countPosts(ctx, t, d, sourceID))
}

// rssFieldsFixture has a single item carrying every persisted field (including
// an author) so the end-to-end field assertions are unambiguous and exactly one
// outbox event is enqueued (HasActiveEvent uses Only()).
const rssFieldsFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Feed</title>
    <item>
      <guid>https://example.com/a</guid>
      <link>https://example.com/a</link>
      <title>A</title>
      <author>Alice</author>
      <description>Body A</description>
      <pubDate>Tue, 14 Apr 2026 12:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

// TestIntegration_RSSCollector_PersistsFieldsAndEnqueuesSummary asserts the
// end-to-end result of collecting a self-contained RSS source through the real
// PostUsecase.Create path. It closes two coverage gaps that the count-only tests
// left open: (1) the stored post carries source/external_id/published_at/author/
// metadata (REQ-05, SC-10) read back from PostgreSQL, not just argument
// forwarding; (2) a summarize_post outbox event is enqueued, so the FT-005
// self-contained summarization mode still fires after collection (REQ-09). This
// is the only implemented path that observes the REQ-09 outbox effect — the
// canonical Telegram scenarios SC-02/SC-03 are escalated (DEC-03).
func TestIntegration_RSSCollector_PersistsFieldsAndEnqueuesSummary(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFieldsFixture))
	}))
	defer srv.Close()

	sourceID := createRSSSource(ctx, t, d, srv.URL)
	collector := task.NewRSSCollector(
		newCollectUsecase(d),
		data.NewRSSFetcher(),
		biz.NewSourceUsecase(data.NewSourceRepo(d)),
		collectorsCfg(30*time.Millisecond),
		testLogger(),
	)

	// Act
	require.NoError(t, collector.Start(ctx))
	waitForPosts(ctx, t, d, sourceID, 1)
	require.NoError(t, collector.Stop(ctx))

	// Assert — persisted post fields (REQ-05 / SC-10), read back from storage.
	res, err := data.NewPostRepo(d).List(ctx, biz.ListPostsFilter{
		SourceID: sourceID,
		PageSize: 10,
		OrderBy:  biz.SortByCreatedAt,
		OrderDir: biz.SortDesc,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	post := res.Items[0]
	assert.Equal(t, sourceID, post.SourceID, "post linked to its source")
	assert.Equal(t, "https://example.com/a", post.ExternalID, "external id from feed guid")
	assert.False(t, post.PublishedAt.IsZero(), "published_at populated from the feed, not zero")
	assert.Equal(t, 2026, post.PublishedAt.UTC().Year(), "published_at parsed from pubDate")
	assert.Equal(t, "A", post.Metadata["title"], "metadata carries the feed title")
	require.NotNil(t, post.Author, "author saved when present in the feed")
	assert.Equal(t, "Alice", *post.Author)

	// Assert — self-contained collection still enqueues a summarize_post event,
	// so FT-005 mode behaviour is unchanged (REQ-09, CTR-T03). HasActiveEvent
	// matches pending/processing events, which the freshly-created event is.
	has, _, err := data.NewSummaryOutboxRepo(d).HasActiveEvent(ctx, sourceID, biz.SummaryEventTypeSummarizePost)
	require.NoError(t, err)
	assert.True(t, has, "self-contained collection enqueues a summarize_post outbox event")
}

func TestIntegration_RSSCollector_IsolatesSourceFailure(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssCollectorFixture))
	}))
	defer good.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	goodID := createRSSSource(ctx, t, d, good.URL)
	_ = createRSSSource(ctx, t, d, bad.URL)

	rec := &recordingHandler{}
	collector := task.NewRSSCollector(
		newCollectUsecase(d),
		data.NewRSSFetcher(),
		biz.NewSourceUsecase(data.NewSourceRepo(d)),
		collectorsCfg(30*time.Millisecond),
		slog.New(rec),
	)

	// Act
	require.NoError(t, collector.Start(ctx))
	waitForPosts(ctx, t, d, goodID, 2)
	require.NoError(t, collector.Stop(ctx))

	// Assert
	assert.Equal(t, 2, countPosts(ctx, t, d, goodID), "healthy source still collected")
	assert.Positive(t, rec.errorCount(), "failing source produced an observable error")
}
