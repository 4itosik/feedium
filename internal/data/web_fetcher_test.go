package data_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/data"
)

const htmlArticleFixture = `<!DOCTYPE html>
<html>
<head>
  <title>Article Title</title>
  <meta name="author" content="Carol">
  <meta property="article:published_time" content="2026-04-14T12:00:00Z">
</head>
<body>
  <article>
    <h1>Article Title</h1>
    <p>First paragraph of the article.</p>
    <p>Second paragraph with more detail.</p>
  </article>
</body>
</html>`

// htmlBrokenFixture has no paragraph content — simulates a markup change that
// breaks text extraction (RISK-04 / SC-07).
const htmlBrokenFixture = `<!DOCTYPE html>
<html><head><title>Broken</title></head><body><div></div></body></html>`

func htmlSource(t *testing.T, pageURL string) biz.Source {
	t.Helper()
	return biz.Source{
		ID:     "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3e",
		Type:   biz.SourceTypeHTML,
		Config: &biz.HTMLConfig{URL: pageURL},
	}
}

func TestWebFetcher_Fetch(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("extracts article text and metadata", func(t *testing.T) {
		// Arrange
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(htmlArticleFixture))
		}))
		defer srv.Close()

		fetcher := data.NewWebFetcher()

		// Act
		items, err := fetcher.Fetch(context.Background(), htmlSource(t, srv.URL))

		// Assert
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, srv.URL, items[0].ExternalID)
		assert.Contains(t, items[0].Text, "First paragraph of the article.")
		assert.Contains(t, items[0].Text, "Second paragraph with more detail.")
		assert.Equal(t, "Article Title", items[0].Metadata["title"])
		require.NotNil(t, items[0].Author)
		assert.Equal(t, "Carol", *items[0].Author)
		assert.Equal(t, 2026, items[0].PublishedAt.Year())
	})

	t.Run("missing author meta yields nil author and a non-zero published time", func(t *testing.T) {
		// Arrange — a page with article text but no author meta and a malformed
		// published_time: author must be nil and the time must fall back to a
		// non-zero value (REQ-05).
		const noMetaFixture = `<!DOCTYPE html>
<html><head><title>No Meta</title>
<meta property="article:published_time" content="not-a-date"></head>
<body><article><p>Body without author metadata.</p></article></body></html>`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(noMetaFixture))
		}))
		defer srv.Close()

		fetcher := data.NewWebFetcher()

		// Act
		items, err := fetcher.Fetch(context.Background(), htmlSource(t, srv.URL))

		// Assert
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Nil(t, items[0].Author)
		assert.False(t, items[0].PublishedAt.IsZero())
	})

	t.Run("broken markup yields an error and no item", func(t *testing.T) {
		// Arrange
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(htmlBrokenFixture))
		}))
		defer srv.Close()

		fetcher := data.NewWebFetcher()

		// Act
		items, err := fetcher.Fetch(context.Background(), htmlSource(t, srv.URL))

		// Assert
		require.Error(t, err)
		assert.Nil(t, items)
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		// Arrange
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		fetcher := data.NewWebFetcher()

		// Act
		_, err := fetcher.Fetch(context.Background(), htmlSource(t, srv.URL))

		// Assert
		require.Error(t, err)
	})
}
