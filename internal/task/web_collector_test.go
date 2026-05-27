package task_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/task"
)

const webCollectorArticle = `<!DOCTYPE html>
<html><head><title>News</title></head>
<body><article><p>An article body worth collecting.</p></article></body></html>`

const webCollectorBroken = `<!DOCTYPE html>
<html><head><title>Broken</title></head><body><div></div></body></html>`

func createHTMLSource(ctx context.Context, t *testing.T, d *data.Data, pageURL string) string {
	t.Helper()
	repo := data.NewSourceRepo(d)
	saved, err := repo.Save(ctx, biz.Source{
		Type:           biz.SourceTypeHTML,
		ProcessingMode: biz.ProcessingModeSelfContained,
		Config:         &biz.HTMLConfig{URL: pageURL},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})
	require.NoError(t, err)
	return saved.ID
}

func TestIntegration_WebCollector_CollectsArticle(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(webCollectorArticle))
	}))
	defer srv.Close()

	sourceID := createHTMLSource(ctx, t, d, srv.URL)
	collector := task.NewWebCollector(
		newCollectUsecase(d),
		data.NewWebFetcher(),
		biz.NewSourceUsecase(data.NewSourceRepo(d)),
		collectorsCfg(),
		testLogger(),
	)

	// Act
	require.NoError(t, collector.Start(ctx))
	waitForPosts(ctx, t, d, sourceID, 1)
	waitForCount(t, &hits, 3) // unchanged page over several cycles → no duplicates
	require.NoError(t, collector.Stop(ctx))

	// Assert
	assert.Equal(t, 1, countPosts(ctx, t, d, sourceID))
}

func TestIntegration_WebCollector_SkipsExtractionFailure(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	// Arrange
	ctx := context.Background()
	d, cleanup := setupTestStack(t)
	defer cleanup()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(webCollectorArticle))
	}))
	defer good.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(webCollectorBroken))
	}))
	defer broken.Close()

	goodID := createHTMLSource(ctx, t, d, good.URL)
	brokenID := createHTMLSource(ctx, t, d, broken.URL)

	rec := &recordingHandler{}
	collector := task.NewWebCollector(
		newCollectUsecase(d),
		data.NewWebFetcher(),
		biz.NewSourceUsecase(data.NewSourceRepo(d)),
		collectorsCfg(),
		slog.New(rec),
	)

	// Act
	require.NoError(t, collector.Start(ctx))
	waitForPosts(ctx, t, d, goodID, 1)
	require.NoError(t, collector.Stop(ctx))

	// Assert
	assert.Equal(t, 1, countPosts(ctx, t, d, goodID), "healthy source collected")
	assert.Equal(t, 0, countPosts(ctx, t, d, brokenID), "no half-empty post for broken markup")
	assert.Positive(t, rec.errorCount(), "extraction failure is observable")
}
