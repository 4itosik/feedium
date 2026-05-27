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

func runSummary(
	t *testing.T,
	client feediumapi.SummaryServiceClient,
	args ...string,
) (string, error) {
	t.Helper()
	cmd := app.NewRootCommandWithSummary(app.FactoryFromSummary(client))
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return outBuf.String(), err
}

// ── summary list ───────────────────────────────────────────────────────────

func TestSummaryList_ByPostID(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	var captured *feediumapi.V1ListPostSummariesRequest
	client.EXPECT().
		V1ListPostSummaries(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1ListPostSummariesRequest, _ ...grpc.CallOption) (*feediumapi.V1ListPostSummariesResponse, error) {
			captured = req
			return &feediumapi.V1ListPostSummariesResponse{}, nil
		})

	// Act
	stdout, err := runSummary(t, client, "summary", "list", "--post-id=post-123")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "post-123", captured.GetPostId())
	assert.Contains(t, stdout, "ID | POST_ID")
}

func TestSummaryList_BySourceID(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	var captured *feediumapi.V1ListSourceSummariesRequest
	client.EXPECT().
		V1ListSourceSummaries(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1ListSourceSummariesRequest, _ ...grpc.CallOption) (*feediumapi.V1ListSourceSummariesResponse, error) {
			captured = req
			return &feediumapi.V1ListSourceSummariesResponse{}, nil
		})

	// Act
	stdout, err := runSummary(t, client, "summary", "list", "--source-id=src-123", "--page-size=50")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "src-123", captured.GetSourceId())
	assert.Equal(t, int32(50), captured.GetPageSize())
	assert.Contains(t, stdout, "ID | POST_ID")
}

func TestSummaryList_NoSelector(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	// no EXPECT: RPC must not fire

	// Act
	stdout, err := runSummary(t, client, "summary", "list")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, app.FormatError(err), "exactly one of --post-id or --source-id")
}

func TestSummaryList_BothSelectors(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	// no EXPECT: RPC must not fire

	// Act
	stdout, err := runSummary(t, client, "summary", "list", "--post-id=p1", "--source-id=s1")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, app.FormatError(err), "exactly one of --post-id or --source-id")
}

func TestSummaryList_EmptyItems_PostID(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	client.EXPECT().
		V1ListPostSummaries(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListPostSummariesResponse{}, nil)

	// Act
	stdout, err := runSummary(t, client, "summary", "list", "--post-id=p1")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "ID | POST_ID | SOURCE_ID | WORD_COUNT | CREATED_AT\n", stdout)
}

func TestSummaryList_EmptyItems_JSON(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	client.EXPECT().
		V1ListPostSummaries(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListPostSummariesResponse{}, nil)

	// Act
	stdout, err := runSummary(t, client, "summary", "list", "--post-id=p1", "--output=json")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "[]\n", stdout)
}

// ── summary get ────────────────────────────────────────────────────────────

func TestSummaryGet_Request(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	var captured *feediumapi.V1GetSummaryRequest
	client.EXPECT().
		V1GetSummary(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1GetSummaryRequest, _ ...grpc.CallOption) (*feediumapi.V1GetSummaryResponse, error) {
			captured = req
			return &feediumapi.V1GetSummaryResponse{
				Summary: &feediumapi.Summary{
					Id:        req.GetId(),
					SourceId:  "src-123",
					Text:      "This is a summary",
					WordCount: 4,
				},
			}, nil
		})

	// Act
	stdout, err := runSummary(t, client, "summary", "get", "sum-123")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "sum-123", captured.GetId())
	assert.Contains(t, stdout, "sum-123")
}

func TestSummaryGet_NotFound(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	client.EXPECT().
		V1GetSummary(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.NotFound, "summary not found"))

	// Act
	stdout, err := runSummary(t, client, "summary", "get", "no-such-id")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "code=NotFound message=summary not found", app.FormatError(err))
}

func TestSummaryGet_WithoutID(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	// no EXPECT: RPC must not fire

	// Act
	stdout, err := runSummary(t, client, "summary", "get")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
}

func TestSummaryList_InvalidOutput(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	// no EXPECT: RPC must not fire

	// Act
	stdout, err := runSummary(t, client, "summary", "list", "--post-id=post-1", "--output=xml")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
}

func TestSummaryGet_InvalidOutput(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockSummaryServiceClient(ctrl)
	// no EXPECT: RPC must not fire

	// Act
	stdout, err := runSummary(t, client, "summary", "get", "sum-1", "--output=xml")

	// Assert
	require.Error(t, err)
	assert.Empty(t, stdout)
}
