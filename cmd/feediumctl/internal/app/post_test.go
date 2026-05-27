package app_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/app"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/app/mock"
)

// ── helpers ───────────────────────────────────────────────────────────────

func runPost(
	t *testing.T,
	client feediumapi.PostServiceClient,
	args ...string,
) (string, error) {
	t.Helper()
	cmd := app.NewRootCommandWithPost(app.FactoryFromPost(client))
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return outBuf.String(), err
}

// ── post list ──────────────────────────────────────────────────────────────

func TestPostList_Request_PageSize(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockPostServiceClient(ctrl)
	var captured *feediumapi.V1ListPostsRequest
	client.EXPECT().
		V1ListPosts(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1ListPostsRequest, _ ...grpc.CallOption) (*feediumapi.V1ListPostsResponse, error) {
			captured = req
			return &feediumapi.V1ListPostsResponse{}, nil
		})

	// Act
	stdout, err := runPost(t, client, "post", "list", "--page-size=25")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, int32(25), captured.GetPageSize())
	assert.Empty(t, captured.GetPageToken())
	assert.Nil(t, captured.SourceId)
	assert.Contains(t, stdout, "ID | SOURCE_ID")
}

func TestPostList_SourceIDFilter(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockPostServiceClient(ctrl)
	var captured *feediumapi.V1ListPostsRequest
	client.EXPECT().
		V1ListPosts(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1ListPostsRequest, _ ...grpc.CallOption) (*feediumapi.V1ListPostsResponse, error) {
			captured = req
			return &feediumapi.V1ListPostsResponse{}, nil
		})

	// Act
	stdout, err := runPost(t, client, "post", "list", "--source-id=src-123")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.NotNil(t, captured.SourceId)
	assert.Equal(t, "src-123", captured.GetSourceId())
	assert.Contains(t, stdout, "ID | SOURCE_ID")
}

func TestPostList_EmptyItems_Table(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockPostServiceClient(ctrl)
	client.EXPECT().
		V1ListPosts(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListPostsResponse{}, nil)

	// Act
	stdout, err := runPost(t, client, "post", "list")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "ID | SOURCE_ID | SOURCE_TYPE | EXTERNAL_ID | PUBLISHED_AT | AUTHOR | CREATED_AT\n", stdout)
}

func TestPostList_EmptyItems_JSON(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockPostServiceClient(ctrl)
	client.EXPECT().
		V1ListPosts(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListPostsResponse{}, nil)

	// Act
	stdout, err := runPost(t, client, "post", "list", "--output=json")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "[]\n", stdout)
}

// ── post get ───────────────────────────────────────────────────────────────

func TestPostGet_Request(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockPostServiceClient(ctrl)
	var captured *feediumapi.V1GetPostRequest
	client.EXPECT().
		V1GetPost(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1GetPostRequest, _ ...grpc.CallOption) (*feediumapi.V1GetPostResponse, error) {
			captured = req
			return &feediumapi.V1GetPostResponse{
				Post: &feediumapi.Post{
					Id: req.GetId(),
					Source: &feediumapi.PostSourceRef{
						Id:   "src-123",
						Type: feediumapi.SourceType_SOURCE_TYPE_RSS,
					},
					ExternalId: "ext-123",
				},
			}, nil
		})

	// Act
	stdout, err := runPost(t, client, "post", "get", "post-123")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "post-123", captured.GetId())
	assert.Contains(t, stdout, "post-123")
	assert.Contains(t, stdout, "src-123")
}

func TestPostGet_NotFound(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockPostServiceClient(ctrl)
	client.EXPECT().
		V1GetPost(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.NotFound, "post not found"))

	// Act
	stdout, err := runPost(t, client, "post", "get", "no-such-id")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "code=NotFound message=post not found", app.FormatError(err))
}

func TestPostGet_WithoutID(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockPostServiceClient(ctrl)
	// no EXPECT: RPC must not fire

	// Act
	stdout, err := runPost(t, client, "post", "get")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
}

func TestPostList_InvalidOutput(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockPostServiceClient(ctrl)
	// no EXPECT: RPC must not fire

	// Act
	stdout, err := runPost(t, client, "post", "list", "--output=xml")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
}

func TestPostGet_InvalidOutput(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockPostServiceClient(ctrl)
	// no EXPECT: RPC must not fire

	// Act
	stdout, err := runPost(t, client, "post", "get", "post-1", "--output=xml")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
}
