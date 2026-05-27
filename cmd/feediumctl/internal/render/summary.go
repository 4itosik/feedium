package render

import (
	"fmt"
	"io"
	"strings"

	feediumapi "github.com/4itosik/feedium/api/feedium"
)

const summaryTableHeader = "ID | POST_ID | SOURCE_ID | WORD_COUNT | CREATED_AT"

// writeSummaryListTable prints the header followed by one row per item.
// Empty items prints only the header.
func writeSummaryListTable(w io.Writer, resp *feediumapi.V1ListPostSummariesResponse) error {
	if _, err := fmt.Fprintln(w, summaryTableHeader); err != nil {
		return err
	}
	for _, s := range resp.GetSummaries() {
		if err := writeSummaryItemRow(w, s); err != nil {
			return err
		}
	}
	return nil
}

// writeSummaryListSourceTable prints the header followed by one row per item for source summaries.
func writeSummaryListSourceTable(w io.Writer, resp *feediumapi.V1ListSourceSummariesResponse) error {
	if _, err := fmt.Fprintln(w, summaryTableHeader); err != nil {
		return err
	}
	for _, s := range resp.GetSummaries() {
		if err := writeSummaryItemRow(w, s); err != nil {
			return err
		}
	}
	return nil
}

// writeSummarySingleTable prints the header and a single summary row.
func writeSummarySingleTable(w io.Writer, s *feediumapi.Summary) error {
	if s == nil {
		panic("render: unreachable nil Summary in response")
	}
	if _, err := fmt.Fprintln(w, summaryTableHeader); err != nil {
		return err
	}
	return writeSummaryItemRow(w, s)
}

// writeSummaryItemRow writes one data row for the table format.
func writeSummaryItemRow(w io.Writer, s *feediumapi.Summary) error {
	postID := s.GetPostId()
	if postID == "" {
		postID = "-"
	}

	_, err := fmt.Fprintf(w, "%s | %s | %s | %d | %s\n",
		s.GetId(),
		postID,
		s.GetSourceId(),
		s.GetWordCount(),
		formatTimestamp(s.GetCreatedAt()),
	)
	return err
}

// writeSummaryListJSON serialises the items slice as a JSON array for post summaries.
func writeSummaryListJSON(w io.Writer, resp *feediumapi.V1ListPostSummariesResponse) error {
	items := resp.GetSummaries()
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

// writeSummaryListSourceJSON serialises the items slice as a JSON array for source summaries.
func writeSummaryListSourceJSON(w io.Writer, resp *feediumapi.V1ListSourceSummariesResponse) error {
	items := resp.GetSummaries()
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
