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

func runInterest(t *testing.T, client feediumapi.InterestServiceClient, args ...string) (string, error) {
	t.Helper()
	cmd := app.NewRootCommandWithInterest(app.FactoryFromInterest(client))
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return outBuf.String(), err
}

func TestInterestCreate(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockInterestServiceClient(ctrl)
	var captured *feediumapi.V1CreateInterestRequest
	client.EXPECT().V1CreateInterest(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1CreateInterestRequest, _ ...grpc.CallOption) (*feediumapi.V1CreateInterestResponse, error) {
			captured = req
			return &feediumapi.V1CreateInterestResponse{
				Interest: &feediumapi.Interest{Id: "i-1", Topic: req.GetTopic()},
			}, nil
		})

	// Act
	stdout, err := runInterest(t, client, "interest", "create", "go programming")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "go programming", captured.GetTopic())
	assert.Contains(t, stdout, "i-1")
}

func TestInterestList(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockInterestServiceClient(ctrl)
	client.EXPECT().V1ListInterests(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1ListInterestsResponse{
			Items: []*feediumapi.Interest{{Id: "i-1", Topic: "ai"}},
		}, nil)

	// Act
	stdout, err := runInterest(t, client, "interest", "list")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, stdout, "ID | TOPIC")
	assert.Contains(t, stdout, "ai")
}

func TestInterestGet(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockInterestServiceClient(ctrl)
	client.EXPECT().V1GetInterest(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1GetInterestRequest, _ ...grpc.CallOption) (*feediumapi.V1GetInterestResponse, error) {
			assert.Equal(t, "i-1", req.GetId())
			return &feediumapi.V1GetInterestResponse{Interest: &feediumapi.Interest{Id: "i-1", Topic: "ai"}}, nil
		})

	// Act
	stdout, err := runInterest(t, client, "interest", "get", "i-1")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, stdout, "ai")
}

func TestInterestUpdate(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockInterestServiceClient(ctrl)
	var captured *feediumapi.V1UpdateInterestRequest
	client.EXPECT().V1UpdateInterest(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *feediumapi.V1UpdateInterestRequest, _ ...grpc.CallOption) (*feediumapi.V1UpdateInterestResponse, error) {
			captured = req
			return &feediumapi.V1UpdateInterestResponse{
				Interest: &feediumapi.Interest{Id: req.GetId(), Topic: req.GetTopic()},
			}, nil
		})

	// Act
	stdout, err := runInterest(t, client, "interest", "update", "i-1", "--topic=devops")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "devops", captured.GetTopic())
	assert.Contains(t, stdout, "devops")
}

func TestInterestDelete(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockInterestServiceClient(ctrl)
	client.EXPECT().V1DeleteInterest(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1DeleteInterestResponse{}, nil)

	// Act
	stdout, err := runInterest(t, client, "interest", "delete", "i-1")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, stdout, "i-1")
}

func TestInterestGet_NotFoundError(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	client := mock.NewMockInterestServiceClient(ctrl)
	client.EXPECT().V1GetInterest(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.NotFound, "ERROR_REASON_INTEREST_NOT_FOUND"))

	// Act
	_, err := runInterest(t, client, "interest", "get", "missing")

	// Assert
	require.Error(t, err)
}
