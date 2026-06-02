package app_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/app"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/app/mock"
)

func runFeed(t *testing.T, client feediumapi.FeedServiceClient, args ...string) (string, error) {
	t.Helper()
	cmd := app.NewRootCommandWithFeed(app.FactoryFromFeed(client))
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return outBuf.String(), err
}

func TestFeedList(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockFeedServiceClient(ctrl)
	client.EXPECT().V1ListFeed(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListFeedResponse{
			Items: []*feediumapi.FeedItem{{PostId: "p-1", SourceId: "s-1", Score: 0.92}},
		}, nil)

	// Act
	stdout, err := runFeed(t, client, "feed", "list")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, stdout, "POST_ID | SOURCE_ID | SCORE")
	assert.Contains(t, stdout, "p-1")
	assert.Contains(t, stdout, "0.92")
}

func TestFeedList_WithFilters(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockFeedServiceClient(ctrl)
	var captured *feediumapi.V1ListFeedRequest
	client.EXPECT().V1ListFeed(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1ListFeedRequest, _ ...grpc.CallOption) (*feediumapi.V1ListFeedResponse, error) {
			captured = req
			return &feediumapi.V1ListFeedResponse{}, nil
		})

	// Act
	stdout, err := runFeed(t, client,
		"feed", "list", "--min-score=0.5", "--from=2026-01-01T00:00:00Z", "--to=2026-06-01T00:00:00Z")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.InDelta(t, 0.5, captured.GetMinScore(), 1e-9)
	require.NotNil(t, captured.GetFrom())
	require.NotNil(t, captured.GetTo())
	assert.Contains(t, stdout, "POST_ID | SOURCE_ID | SCORE")
}

func TestFeedList_InvalidFromTimestamp(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockFeedServiceClient(ctrl)
	// no EXPECT: RPC must not fire when the flag fails to parse

	// Act
	_, err := runFeed(t, client, "feed", "list", "--from=not-a-time")

	// Assert
	require.Error(t, err)
}
