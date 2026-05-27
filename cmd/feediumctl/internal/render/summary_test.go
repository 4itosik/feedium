package render_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/render"
)

func TestSummaryListPost_EmptyTable(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostSummariesResponse{}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatTable, resp)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "ID | POST_ID | SOURCE_ID | WORD_COUNT | CREATED_AT\n", buf.String())
}

func TestSummaryListPost_EmptyJSON(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostSummariesResponse{}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatJSON, resp)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

func TestSummaryListPost_WithItem_Table(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostSummariesResponse{
		Summaries: []*feediumapi.Summary{
			{
				Id:        "sum-1",
				SourceId:  "src-1",
				Text:      "Summary text",
				WordCount: 2,
				CreatedAt: &timestamppb.Timestamp{
					Seconds: 1609459200,
				},
			},
		},
	}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatTable, resp)

	// Assert
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "sum-1")
	assert.Contains(t, output, "src-1")
	assert.Contains(t, output, "2")
}

func TestSummaryListSource_EmptyTable(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListSourceSummariesResponse{}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatTable, resp)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "ID | POST_ID | SOURCE_ID | WORD_COUNT | CREATED_AT\n", buf.String())
}

func TestSummaryListPost_EmptyYAML(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostSummariesResponse{}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatYAML, resp)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

func TestSummaryListPost_WithItem_YAML(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostSummariesResponse{
		Summaries: []*feediumapi.Summary{
			{
				Id:        "sum-1",
				SourceId:  "src-1",
				WordCount: 2,
			},
		},
	}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatYAML, resp)

	// Assert
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "sum-1")
	assert.Contains(t, output, "src-1")
	assert.Equal(t, byte('\n'), output[len(output)-1], "output must end with single newline")
}

func TestSummaryListSource_EmptyYAML(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListSourceSummariesResponse{}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatYAML, resp)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

func TestSummaryListSource_WithItem_YAML(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListSourceSummariesResponse{
		Summaries: []*feediumapi.Summary{
			{
				Id:        "sum-1",
				SourceId:  "src-1",
				WordCount: 3,
			},
		},
	}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatYAML, resp)

	// Assert
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "sum-1")
	assert.Contains(t, output, "src-1")
	assert.Equal(t, byte('\n'), output[len(output)-1], "output must end with single newline")
}

func TestSummaryGet_JSON(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1GetSummaryResponse{
		Summary: &feediumapi.Summary{
			Id:        "sum-1",
			SourceId:  "src-1",
			Text:      "Summary text",
			WordCount: 2,
		},
	}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatJSON, resp)

	// Assert
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "sum-1")
	assert.Contains(t, output, "src-1")
}

func TestSummaryGet_YAML(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1GetSummaryResponse{
		Summary: &feediumapi.Summary{
			Id:        "sum-1",
			SourceId:  "src-1",
			Text:      "Summary text",
			WordCount: 2,
		},
	}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatYAML, resp)

	// Assert
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "sum-1")
	assert.Contains(t, output, "src-1")
	assert.Equal(t, byte('\n'), output[len(output)-1], "output must end with single newline")
}
