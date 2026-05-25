package data

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/4itosik/feedium/internal/biz"
)

// webFetcher implements biz.Fetcher for plain web pages. It extracts the
// article text from the page's paragraphs using golang.org/x/net/html (already
// a project dependency), keeping FT-009 free of new dependencies. The strategy
// lives behind the port and can be replaced by a readability library later
// (DEC-02 alternative — reversible). When no text can be extracted — e.g. the
// page changed its markup (RISK-04) — Fetch returns an error so the task layer
// can apply single-retry-then-skip (FM-T02); no half-empty Post is produced.
type webFetcher struct {
	client *http.Client
}

var (
	_ biz.Fetcher    = (*webFetcher)(nil)
	_ biz.WebFetcher = (*webFetcher)(nil)
)

// NewWebFetcher creates a web page fetcher.
//
//nolint:revive // unexported return type is intentional for Wire injection
func NewWebFetcher() *webFetcher {
	return &webFetcher{client: &http.Client{Timeout: defaultFetchTimeout}}
}

// Fetch downloads the configured page and returns a single CollectedItem with
// the extracted article text. The page URL is the external identifier, so
// re-collecting the same page is idempotent (REQ-06).
func (f *webFetcher) Fetch(ctx context.Context, source biz.Source) ([]biz.CollectedItem, error) {
	cfg, ok := source.Config.(*biz.HTMLConfig)
	if !ok {
		return nil, fmt.Errorf("web fetcher: unexpected config type %T for source %s", source.Config, source.ID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("web fetcher: new request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web fetcher: get page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("web fetcher: page returned status %d", resp.StatusCode)
	}

	root, err := html.Parse(io.LimitReader(resp.Body, maxFeedBytes))
	if err != nil {
		return nil, fmt.Errorf("web fetcher: parse html for source %s: %w", source.ID, err)
	}

	article := extractArticle(root)
	if strings.TrimSpace(article.text) == "" {
		return nil, fmt.Errorf("web fetcher: no extractable text for source %s (markup may have changed)", source.ID)
	}

	metadata := map[string]string{"url": cfg.URL}
	if article.title != "" {
		metadata["title"] = article.title
	}

	return []biz.CollectedItem{{
		ExternalID:  cfg.URL,
		PublishedAt: article.publishedAt,
		Author:      optionalAuthor(article.author),
		Text:        article.text,
		Metadata:    metadata,
	}}, nil
}

type extractedArticle struct {
	title       string
	author      string
	publishedAt time.Time
	text        string
}

// extractArticle walks the parsed document collecting the title, paragraph
// text and the author/published-time hints from common meta tags.
func extractArticle(root *html.Node) extractedArticle {
	result := extractedArticle{publishedAt: time.Now().UTC()}
	var paragraphs []string

	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Title:
				if result.title == "" {
					result.title = strings.TrimSpace(nodeText(n))
				}
			case atom.P:
				if t := strings.TrimSpace(nodeText(n)); t != "" {
					paragraphs = append(paragraphs, t)
				}
				return // text already collected; do not recurse into the paragraph
			case atom.Meta:
				applyMeta(n, &result)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)

	result.text = strings.Join(paragraphs, "\n\n")
	return result
}

// applyMeta reads author and published-time hints from a <meta> element.
func applyMeta(n *html.Node, result *extractedArticle) {
	var name, property, content string
	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "name":
			name = strings.ToLower(attr.Val)
		case "property":
			property = strings.ToLower(attr.Val)
		case "content":
			content = attr.Val
		}
	}

	if content == "" {
		return
	}
	if name == "author" && result.author == "" {
		result.author = content
	}
	if property == "article:published_time" {
		if t, err := time.Parse(time.RFC3339, content); err == nil {
			result.publishedAt = t.UTC()
		}
	}
}

// nodeText concatenates all descendant text nodes of n.
func nodeText(n *html.Node) string {
	var sb strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return strings.TrimSpace(sb.String())
}
