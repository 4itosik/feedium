package render

import (
	"fmt"
	"io"
	"strings"
	"time"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const sourceTableHeader = "ID | TYPE | MODE | CONFIG | CREATED_AT"

// sourceTypeShort returns the SourceType name without the SOURCE_TYPE_ prefix
// (SR-08). Returns "UNSPECIFIED" for SOURCE_TYPE_UNSPECIFIED and unknown values.
func sourceTypeShort(t feediumapi.SourceType) string {
	const prefix = "SOURCE_TYPE_"
	s := t.String()
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return "UNSPECIFIED"
}

// processingModeShort returns the ProcessingMode name without PROCESSING_MODE_
// prefix (SR-08). Returns "UNSPECIFIED" for PROCESSING_MODE_UNSPECIFIED.
func processingModeShort(m feediumapi.ProcessingMode) string {
	const prefix = "PROCESSING_MODE_"
	s := m.String()
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return "UNSPECIFIED"
}

// formatTimestamp converts a Timestamp to RFC3339 UTC (SR-08, OI-4).
// Returns an empty string for nil timestamps.
func formatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format(time.RFC3339)
}

// formatSourceConfig returns the single-line representation of SourceConfig.config
// (SR-08). Returns an empty string when config is nil or the oneof is not set
// (EC-H table case).
func formatSourceConfig(cfg *feediumapi.SourceConfig) string {
	if cfg == nil {
		return ""
	}
	switch v := cfg.Config.(type) {
	case *feediumapi.SourceConfig_TelegramChannel:
		c := v.TelegramChannel
		return fmt.Sprintf("tg_id=%d,username=%s", c.GetTgId(), c.GetUsername())
	case *feediumapi.SourceConfig_TelegramGroup:
		c := v.TelegramGroup
		return fmt.Sprintf("tg_id=%d,username=%s", c.GetTgId(), c.GetUsername())
	case *feediumapi.SourceConfig_Rss:
		return fmt.Sprintf("feed_url=%s", v.Rss.GetFeedUrl())
	case *feediumapi.SourceConfig_Html:
		return fmt.Sprintf("url=%s", v.Html.GetUrl())
	default:
		return ""
	}
}

// writeSourceListTable prints the header followed by one row per item (SR-08).
// Empty items prints only the header (EC-C table).
func writeSourceListTable(w io.Writer, resp *feediumapi.V1ListSourcesResponse) error {
	if _, err := fmt.Fprintln(w, sourceTableHeader); err != nil {
		return err
	}
	for _, s := range resp.GetItems() {
		if err := writeSourceItemRow(w, s); err != nil {
			return err
		}
	}
	return nil
}

// writeSourceSingleTable prints the header and a single source row (SR-09).
func writeSourceSingleTable(w io.Writer, s *feediumapi.Source) error {
	if s == nil {
		// Server contract guarantees a non-nil Source in get/create/update
		// responses. Panicking here surfaces a protocol bug instead of
		// leaking a non-NFR-03 prefix.
		panic("render: unreachable nil Source in response")
	}
	if _, err := fmt.Fprintln(w, sourceTableHeader); err != nil {
		return err
	}
	return writeSourceItemRow(w, s)
}

// writeSourceItemRow writes one data row for the table format.
func writeSourceItemRow(w io.Writer, s *feediumapi.Source) error {
	_, err := fmt.Fprintf(w, "%s | %s | %s | %s | %s\n",
		s.GetId(),
		sourceTypeShort(s.GetType()),
		processingModeShort(s.GetProcessingMode()),
		formatSourceConfig(s.GetConfig()),
		formatTimestamp(s.GetCreatedAt()),
	)
	return err
}
