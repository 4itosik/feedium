package render

import (
	"fmt"
	"io"
	"strings"

	feediumapi "github.com/4itosik/feedium/api/feedium"
)

const postTableHeader = "ID | SOURCE_ID | SOURCE_TYPE | EXTERNAL_ID | PUBLISHED_AT | AUTHOR | CREATED_AT"

// writePostListTable prints the header followed by one row per item.
// Empty items prints only the header.
func writePostListTable(w io.Writer, resp *feediumapi.V1ListPostsResponse) error {
	if _, err := fmt.Fprintln(w, postTableHeader); err != nil {
		return err
	}
	for _, p := range resp.GetItems() {
		if err := writePostItemRow(w, p); err != nil {
			return err
		}
	}
	return nil
}

// writePostSingleTable prints the header and a single post row.
func writePostSingleTable(w io.Writer, p *feediumapi.Post) error {
	if p == nil {
		panic("render: unreachable nil Post in response")
	}
	if _, err := fmt.Fprintln(w, postTableHeader); err != nil {
		return err
	}
	return writePostItemRow(w, p)
}

// writePostItemRow writes one data row for the table format.
func writePostItemRow(w io.Writer, p *feediumapi.Post) error {
	sourceType := ""
	sourceID := ""
	if src := p.GetSource(); src != nil {
		sourceID = src.GetId()
		sourceType = sourceTypeShort(src.GetType())
	}

	author := p.GetAuthor()
	if author == "" {
		author = "-"
	}

	_, err := fmt.Fprintf(w, "%s | %s | %s | %s | %s | %s | %s\n",
		p.GetId(),
		sourceID,
		sourceType,
		p.GetExternalId(),
		formatTimestamp(p.GetPublishedAt()),
		author,
		formatTimestamp(p.GetCreatedAt()),
	)
	return err
}

// writePostListJSON serialises the items slice as a JSON array.
// Each item uses the compact listItemMarshaler.
func writePostListJSON(w io.Writer, resp *feediumapi.V1ListPostsResponse) error {
	items := resp.GetItems()
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, "[]")
		return err
	}
	parts := make([]string, len(items))
	for i, item := range items {
		data, err := listItemMarshaler().Marshal(item)
		if err != nil {
			return err
		}
		parts[i] = string(data)
	}
	_, err := fmt.Fprintf(w, "[%s]\n", strings.Join(parts, ","))
	return err
}
