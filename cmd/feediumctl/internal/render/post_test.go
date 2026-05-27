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

func TestPostList_EmptyTable(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostsResponse{}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatTable, resp)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "ID | SOURCE_ID | SOURCE_TYPE | EXTERNAL_ID | PUBLISHED_AT | AUTHOR | CREATED_AT\n", buf.String())
}

func TestPostList_EmptyJSON(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostsResponse{}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatJSON, resp)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

func TestPostList_WithItem_Table(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostsResponse{
		Items: []*feediumapi.Post{
			{
				Id: "post-1",
				Source: &feediumapi.PostSourceRef{
					Id:   "src-1",
					Type: feediumapi.SourceType_SOURCE_TYPE_RSS,
				},
				ExternalId: "ext-1",
				PublishedAt: &timestamppb.Timestamp{
					Seconds: 1609459200, // 2021-01-01 00:00:00 UTC
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
	assert.Contains(t, output, "post-1")
	assert.Contains(t, output, "src-1")
	assert.Contains(t, output, "RSS")
	assert.Contains(t, output, "ext-1")
}

func TestPostList_EmptyYAML(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostsResponse{}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatYAML, resp)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

func TestPostList_WithItem_YAML(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1ListPostsResponse{
		Items: []*feediumapi.Post{
			{
				Id: "post-1",
				Source: &feediumapi.PostSourceRef{
					Id:   "src-1",
					Type: feediumapi.SourceType_SOURCE_TYPE_RSS,
				},
				ExternalId: "ext-1",
			},
		},
	}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatYAML, resp)

	// Assert
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "post-1")
	assert.Contains(t, output, "src-1")
	assert.Contains(t, output, "ext-1")
	assert.Equal(t, byte('\n'), output[len(output)-1], "output must end with single newline")
}

func TestPostGet_JSON(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1GetPostResponse{
		Post: &feediumapi.Post{
			Id: "post-1",
			Source: &feediumapi.PostSourceRef{
				Id:   "src-1",
				Type: feediumapi.SourceType_SOURCE_TYPE_RSS,
			},
		},
	}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatJSON, resp)

	// Assert
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "post-1")
	assert.Contains(t, output, "src-1")
}

func TestPostGet_YAML(t *testing.T) {
	// Arrange
	resp := &feediumapi.V1GetPostResponse{
		Post: &feediumapi.Post{
			Id: "post-1",
			Source: &feediumapi.PostSourceRef{
				Id:   "src-1",
				Type: feediumapi.SourceType_SOURCE_TYPE_RSS,
			},
		},
	}

	// Act
	var buf bytes.Buffer
	err := render.Write(&buf, render.FormatYAML, resp)

	// Assert
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "post-1")
	assert.Contains(t, output, "src-1")
	assert.Equal(t, byte('\n'), output[len(output)-1], "output must end with single newline")
}
