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

const rssFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Example Feed</title>
    <item>
      <guid>https://example.com/posts/1</guid>
      <link>https://example.com/posts/1</link>
      <title>First Post</title>
      <description>Body of the first post.</description>
      <dc:creator>Alice</dc:creator>
      <pubDate>Tue, 14 Apr 2026 12:00:00 +0000</pubDate>
    </item>
    <item>
      <guid>https://example.com/posts/2</guid>
      <link>https://example.com/posts/2</link>
      <title>Second Post</title>
      <description>Body of the second post.</description>
      <pubDate>Wed, 15 Apr 2026 09:30:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

const atomFixture = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Example</title>
  <entry>
    <id>urn:uuid:1</id>
    <link href="https://example.com/atom/1"/>
    <title>Atom Entry</title>
    <published>2026-04-14T12:00:00Z</published>
    <author><name>Bob</name></author>
    <content>Atom content body.</content>
  </entry>
</feed>`

func rssSource(t *testing.T, feedURL string) biz.Source {
	t.Helper()
	return biz.Source{
		ID:     "01961d9c-4f78-7e2e-8c3a-5e7d9a1b2c3d",
		Type:   biz.SourceTypeRSS,
		Config: &biz.RSSConfig{FeedURL: feedURL},
	}
}

func TestRSSFetcher_Fetch(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("maps rss 2.0 items to collected items", func(t *testing.T) {
		// Arrange
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rssFixture))
		}))
		defer srv.Close()

		fetcher := data.NewRSSFetcher()

		// Act
		items, err := fetcher.Fetch(context.Background(), rssSource(t, srv.URL))

		// Assert
		require.NoError(t, err)
		require.Len(t, items, 2)
		assert.Equal(t, "https://example.com/posts/1", items[0].ExternalID)
		assert.Equal(t, "Body of the first post.", items[0].Text)
		require.NotNil(t, items[0].Author)
		assert.Equal(t, "Alice", *items[0].Author)
		assert.Equal(t, 2026, items[0].PublishedAt.Year())
		assert.Equal(t, "First Post", items[0].Metadata["title"])
		assert.Nil(t, items[1].Author)
	})

	t.Run("maps atom entries to collected items", func(t *testing.T) {
		// Arrange
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(atomFixture))
		}))
		defer srv.Close()

		fetcher := data.NewRSSFetcher()

		// Act
		items, err := fetcher.Fetch(context.Background(), rssSource(t, srv.URL))

		// Assert
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "urn:uuid:1", items[0].ExternalID)
		assert.Equal(t, "Atom content body.", items[0].Text)
		require.NotNil(t, items[0].Author)
		assert.Equal(t, "Bob", *items[0].Author)
	})

	t.Run("item without guid/link/title is skipped", func(t *testing.T) {
		// Arrange — an item with no stable identity cannot be deduped (REQ-06),
		// so it must be dropped rather than collected.
		const noIDFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Feed</title>
  <item><description>Body but no id</description></item>
  <item><guid>https://example.com/ok</guid><description>Kept</description></item>
</channel></rss>`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(noIDFixture))
		}))
		defer srv.Close()

		fetcher := data.NewRSSFetcher()

		// Act
		items, err := fetcher.Fetch(context.Background(), rssSource(t, srv.URL))

		// Assert
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "https://example.com/ok", items[0].ExternalID)
	})

	t.Run("missing pubDate falls back to a non-zero time", func(t *testing.T) {
		// Arrange — PostUsecase.Create requires a non-zero published_at, so a
		// feed without a date must still yield a usable timestamp (REQ-05).
		const noDateFixture = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Feed</title>
  <item><guid>https://example.com/a</guid><description>No date here</description></item>
</channel></rss>`
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(noDateFixture))
		}))
		defer srv.Close()

		fetcher := data.NewRSSFetcher()

		// Act
		items, err := fetcher.Fetch(context.Background(), rssSource(t, srv.URL))

		// Assert
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.False(t, items[0].PublishedAt.IsZero())
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		// Arrange
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		fetcher := data.NewRSSFetcher()

		// Act
		_, err := fetcher.Fetch(context.Background(), rssSource(t, srv.URL))

		// Assert
		require.Error(t, err)
	})

	t.Run("wrong config type returns error", func(t *testing.T) {
		// Arrange
		fetcher := data.NewRSSFetcher()
		source := biz.Source{ID: "s1", Type: biz.SourceTypeRSS, Config: &biz.HTMLConfig{URL: "https://x"}}

		// Act
		_, err := fetcher.Fetch(context.Background(), source)

		// Assert
		require.Error(t, err)
	})
}
