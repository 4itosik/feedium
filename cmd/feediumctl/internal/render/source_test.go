package render_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/render"
)

// ── WriteDelete snapshot (AC-S4, SR-05, SR-10) ───────────────────────────

const snapshotID = "00000000-0000-4000-8000-000000000001"

func TestWriteDelete_Table(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, render.WriteDelete(&buf, render.FormatTable, snapshotID))
	assert.Equal(t, "deleted: "+snapshotID+"\n", buf.String())
}

func TestWriteDelete_JSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, render.WriteDelete(&buf, render.FormatJSON, snapshotID))
	assert.Equal(t, `{"deleted":true,"id":"`+snapshotID+`"}`+"\n", buf.String())
}

func TestWriteDelete_YAML(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, render.WriteDelete(&buf, render.FormatYAML, snapshotID))
	got := buf.String()
	assert.Equal(t, "deleted: true\nid: "+snapshotID+"\n", got)
	// SR-10: UUID must not be quoted in YAML output
	assert.NotContains(t, got, `"`+snapshotID+`"`, "UUID must not be quoted in YAML")
}

func TestWriteDelete_Deterministic(t *testing.T) {
	var a, b bytes.Buffer
	require.NoError(t, render.WriteDelete(&a, render.FormatTable, snapshotID))
	require.NoError(t, render.WriteDelete(&b, render.FormatTable, snapshotID))
	assert.Equal(t, a.Bytes(), b.Bytes(), "INV-06: same input → same output")
}

// ── Source list table (EC-C, SR-08) ──────────────────────────────────────

func TestWriteSourceList_Table_Empty_EC_C(t *testing.T) {
	var buf bytes.Buffer
	resp := &feediumapi.V1ListSourcesResponse{}
	require.NoError(t, render.Write(&buf, render.FormatTable, resp))
	got := buf.String()
	assert.Equal(t, "ID | TYPE | MODE | CONFIG | CREATED_AT\n", got)
}

func TestWriteSourceList_Table_OneItem(t *testing.T) {
	ts := timestamppb.New(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	resp := &feediumapi.V1ListSourcesResponse{
		Items: []*feediumapi.Source{
			{
				Id:             "abc-123",
				Type:           feediumapi.SourceType_SOURCE_TYPE_RSS,
				ProcessingMode: feediumapi.ProcessingMode_PROCESSING_MODE_SELF_CONTAINED,
				Config: &feediumapi.SourceConfig{
					Config: &feediumapi.SourceConfig_Rss{
						Rss: &feediumapi.RSSConfig{FeedUrl: "https://example.com/feed"},
					},
				},
				CreatedAt: ts,
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, render.Write(&buf, render.FormatTable, resp))
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "ID | TYPE | MODE | CONFIG | CREATED_AT", lines[0])
	assert.Contains(t, lines[1], "abc-123")
	assert.Contains(t, lines[1], "RSS")
	assert.Contains(t, lines[1], "SELF_CONTAINED")
	assert.Contains(t, lines[1], "feed_url=https://example.com/feed")
	assert.Contains(t, lines[1], "2024-01-02T03:04:05Z")
}

func TestWriteSourceList_Table_OrderPreserved_R2(t *testing.T) {
	resp := &feediumapi.V1ListSourcesResponse{
		Items: []*feediumapi.Source{
			{Id: "first"},
			{Id: "second"},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, render.Write(&buf, render.FormatTable, resp))
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[1], "first")
	assert.Contains(t, lines[2], "second")
}

// ── Source list JSON / YAML (EC-C, AC-S1) ────────────────────────────────

func TestWriteSourceList_JSON_Empty_EC_C(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, render.Write(&buf, render.FormatJSON, &feediumapi.V1ListSourcesResponse{}))
	assert.Equal(t, "[]\n", buf.String())
}

func TestWriteSourceList_YAML_Empty_EC_C(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, render.Write(&buf, render.FormatYAML, &feediumapi.V1ListSourcesResponse{}))
	assert.Equal(t, "[]\n", buf.String())
}

func TestWriteSourceList_JSON_IsArray(t *testing.T) {
	resp := &feediumapi.V1ListSourcesResponse{
		Items: []*feediumapi.Source{{Id: "x", Type: feediumapi.SourceType_SOURCE_TYPE_HTML}},
	}
	var buf bytes.Buffer
	require.NoError(t, render.Write(&buf, render.FormatJSON, resp))
	got := buf.String()
	assert.True(t, strings.HasPrefix(got, "["), "JSON must start with [")
	assert.True(t, strings.HasSuffix(got, "]\n"), "JSON must end with ]\\n")
	assert.Contains(t, got, `"id"`)
}

// ── Enum short names (SR-08, AC-S5) ──────────────────────────────────────

func TestWriteSourceGet_Table_EnumShortNames(t *testing.T) {
	cases := []struct {
		srcType  feediumapi.SourceType
		mode     feediumapi.ProcessingMode
		wantType string
		wantMode string
	}{
		{feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL, feediumapi.ProcessingMode_PROCESSING_MODE_SELF_CONTAINED, "TELEGRAM_CHANNEL", "SELF_CONTAINED"},
		{feediumapi.SourceType_SOURCE_TYPE_TELEGRAM_GROUP, feediumapi.ProcessingMode_PROCESSING_MODE_CUMULATIVE, "TELEGRAM_GROUP", "CUMULATIVE"},
		{feediumapi.SourceType_SOURCE_TYPE_RSS, feediumapi.ProcessingMode_PROCESSING_MODE_UNSPECIFIED, "RSS", "UNSPECIFIED"},
		{feediumapi.SourceType_SOURCE_TYPE_HTML, feediumapi.ProcessingMode_PROCESSING_MODE_UNSPECIFIED, "HTML", "UNSPECIFIED"},
		{feediumapi.SourceType_SOURCE_TYPE_UNSPECIFIED, feediumapi.ProcessingMode_PROCESSING_MODE_UNSPECIFIED, "UNSPECIFIED", "UNSPECIFIED"},
	}
	for _, tc := range cases {
		t.Run(tc.wantType, func(t *testing.T) {
			resp := &feediumapi.V1GetSourceResponse{
				Source: &feediumapi.Source{Id: "id-1", Type: tc.srcType, ProcessingMode: tc.mode},
			}
			var buf bytes.Buffer
			require.NoError(t, render.Write(&buf, render.FormatTable, resp))
			assert.Contains(t, buf.String(), tc.wantType)
			assert.Contains(t, buf.String(), tc.wantMode)
		})
	}
}

// ── EC-H: empty SourceConfig.config ──────────────────────────────────────

func TestWriteSource_Table_EmptyConfig_EC_H(t *testing.T) {
	resp := &feediumapi.V1GetSourceResponse{
		Source: &feediumapi.Source{
			Id:     "id-h",
			Type:   feediumapi.SourceType_SOURCE_TYPE_RSS,
			Config: &feediumapi.SourceConfig{}, // oneof not set
		},
	}
	var buf bytes.Buffer
	require.NoError(t, render.Write(&buf, render.FormatTable, resp))
	// CONFIG column must be empty: four pipes separate five columns
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	parts := strings.Split(lines[1], " | ")
	require.Len(t, parts, 5)
	assert.Empty(t, parts[3], "CONFIG column must be empty when oneof unset")
}

func TestWriteSource_JSON_NilConfig_Omitted_EC_H(t *testing.T) {
	// When the server sends a Source with no config (nil pointer),
	// EmitUnpopulated=false means the config field is omitted entirely.
	resp := &feediumapi.V1GetSourceResponse{
		Source: &feediumapi.Source{
			Id:     "id-h",
			Type:   feediumapi.SourceType_SOURCE_TYPE_RSS,
			Config: nil,
		},
	}
	var buf bytes.Buffer
	require.NoError(t, render.Write(&buf, render.FormatJSON, resp))
	assert.NotContains(t, buf.String(), `"config"`)
}
