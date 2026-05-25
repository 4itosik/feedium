package data

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/4itosik/feedium/internal/biz"
)

const (
	// defaultFetchTimeout bounds a single feed/page HTTP request.
	defaultFetchTimeout = 30 * time.Second
	// maxFeedBytes caps how much of a feed body we read, to bound memory under
	// the 10k posts/day target (CON-03) and protect against hostile feeds.
	maxFeedBytes = 8 << 20 // 8 MiB
)

// rssFetcher implements biz.Fetcher for RSS/Atom feeds. It uses the standard
// library encoding/xml rather than a third-party feed parser: that keeps
// FT-009 free of new dependencies (see implement.md), and the parser is
// isolated behind the port so it can be swapped without touching biz/ or task/
// (DEC-01 alternative — reversible).
type rssFetcher struct {
	client *http.Client
}

// Compile-time assertions.
var (
	_ biz.Fetcher    = (*rssFetcher)(nil)
	_ biz.RSSFetcher = (*rssFetcher)(nil)
)

// NewRSSFetcher creates an RSS feed fetcher.
//
//nolint:revive // unexported return type is intentional for Wire injection
func NewRSSFetcher() *rssFetcher {
	return &rssFetcher{client: &http.Client{Timeout: defaultFetchTimeout}}
}

// Fetch downloads the source's feed and maps its entries to CollectedItem.
// Errors are returned, never logged (CON-02). Item published-at falls back to
// now when the feed omits or garbles the date, so the post still satisfies the
// non-zero published_at invariant of PostUsecase.Create (REQ-05).
func (f *rssFetcher) Fetch(ctx context.Context, source biz.Source) ([]biz.CollectedItem, error) {
	cfg, ok := source.Config.(*biz.RSSConfig)
	if !ok {
		return nil, fmt.Errorf("rss fetcher: unexpected config type %T for source %s", source.Config, source.ID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.FeedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("rss fetcher: new request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rss fetcher: get feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rss fetcher: feed returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes))
	if err != nil {
		return nil, fmt.Errorf("rss fetcher: read feed: %w", err)
	}

	items, err := parseFeed(body)
	if err != nil {
		return nil, fmt.Errorf("rss fetcher: parse feed for source %s: %w", source.ID, err)
	}
	return items, nil
}

type rssRoot struct {
	XMLName xml.Name  `xml:"rss"`
	Items   []rssItem `xml:"channel>item"`
}

type rssItem struct {
	GUID        string `xml:"guid"`
	Link        string `xml:"link"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Author      string `xml:"author"`
	Creator     string `xml:"creator"` // dc:creator (namespace prefix ignored by encoding/xml)
	PubDate     string `xml:"pubDate"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID   string `xml:"id"`
	Link struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Title     string `xml:"title"`
	Summary   string `xml:"summary"`
	Content   string `xml:"content"`
	Updated   string `xml:"updated"`
	Published string `xml:"published"`
	Author    struct {
		Name string `xml:"name"`
	} `xml:"author"`
}

// parseFeed detects RSS 2.0 vs Atom by attempting each root element.
func parseFeed(data []byte) ([]biz.CollectedItem, error) {
	var rss rssRoot
	if err := xml.Unmarshal(data, &rss); err == nil {
		return mapRSSItems(rss.Items), nil
	}

	var atom atomFeed
	if err := xml.Unmarshal(data, &atom); err == nil {
		return mapAtomEntries(atom.Entries), nil
	}

	return nil, errors.New("unrecognized feed format")
}

func mapRSSItems(items []rssItem) []biz.CollectedItem {
	out := make([]biz.CollectedItem, 0, len(items))
	for _, it := range items {
		externalID := firstNonEmpty(it.GUID, it.Link, it.Title)
		if externalID == "" {
			continue // no stable identity → cannot dedupe (REQ-06)
		}
		out = append(out, biz.CollectedItem{
			ExternalID:  externalID,
			PublishedAt: parseFeedTime(it.PubDate),
			Author:      optionalAuthor(firstNonEmpty(it.Creator, it.Author)),
			Text:        firstNonEmpty(it.Description, it.Title),
			Metadata:    feedMetadata(it.Title, it.Link),
		})
	}
	return out
}

func mapAtomEntries(entries []atomEntry) []biz.CollectedItem {
	out := make([]biz.CollectedItem, 0, len(entries))
	for _, e := range entries {
		externalID := firstNonEmpty(e.ID, e.Link.Href, e.Title)
		if externalID == "" {
			continue
		}
		out = append(out, biz.CollectedItem{
			ExternalID:  externalID,
			PublishedAt: parseFeedTime(firstNonEmpty(e.Published, e.Updated)),
			Author:      optionalAuthor(e.Author.Name),
			Text:        firstNonEmpty(e.Content, e.Summary, e.Title),
			Metadata:    feedMetadata(e.Title, e.Link.Href),
		})
	}
	return out
}

func feedMetadata(title, link string) map[string]string {
	md := map[string]string{}
	if t := strings.TrimSpace(title); t != "" {
		md["title"] = t
	}
	if l := strings.TrimSpace(link); l != "" {
		md["link"] = l
	}
	return md
}

func optionalAuthor(name string) *string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return &name
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// parseFeedTime tries the common feed date layouts and falls back to now.
func parseFeedTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now().UTC()
	}
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		time.RFC822Z,
		time.RFC822,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}
