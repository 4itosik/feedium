package app_test

import (
	"bytes"
	"context"
	"io"
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

func runHealthCLI(t *testing.T, client feediumapi.HealthServiceClient, args ...string) (stdout string, err error) {
	t.Helper()
	cmd := app.NewRootCommandWithHealth(app.FactoryFromHealth(client))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	return buf.String(), err
}

func TestHealth_HappyPath_Table(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock.NewMockHealthServiceClient(ctrl)

	var captured *feediumapi.V1CheckRequest
	client.EXPECT().
		V1Check(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *feediumapi.V1CheckRequest, _ ...grpc.CallOption) (*feediumapi.V1CheckResponse, error) {
			captured = in
			return &feediumapi.V1CheckResponse{Status: "SERVING"}, nil
		})

	out, err := runHealthCLI(t, client, "health")
	require.NoError(t, err)
	assert.Equal(t, "FIELD | VALUE\nstatus | SERVING\n", out)
	require.NotNil(t, captured, "V1CheckRequest must be dispatched to the client")
	assert.Empty(t, captured.String(), "V1CheckRequest must be empty (FR-01)")
}

func TestHealth_HappyPath_JSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock.NewMockHealthServiceClient(ctrl)
	client.EXPECT().
		V1Check(gomock.Any(), gomock.Any()).
		Return(&feediumapi.V1CheckResponse{Status: "SERVING"}, nil)

	out, err := runHealthCLI(t, client, "health", "--output=json")
	require.NoError(t, err)
	assert.Contains(t, out, `"status"`)
	assert.Contains(t, out, `"SERVING"`)
}

func TestHealth_UnavailableRoutedThroughRPCFormatter(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock.NewMockHealthServiceClient(ctrl)
	client.EXPECT().
		V1Check(gomock.Any(), gomock.Any()).
		Return(nil, status.Error(codes.Unavailable, "connection refused"))

	out, err := runHealthCLI(t, client, "health")
	require.Error(t, err)
	assert.Empty(t, out, "stdout must be empty on error (INV-02)")
	assert.Equal(t, "code=Unavailable message=connection refused", app.FormatError(err))
}

func TestHealth_OutputValidation(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock.NewMockHealthServiceClient(ctrl) // no EXPECT: any V1Check call fails the test

	out, err := runHealthCLI(t, client, "health", "--output=xml")
	require.Error(t, err)
	assert.Empty(t, out)
	assert.Equal(t, `output: invalid value "xml" (allowed: table,json,yaml)`, app.FormatError(err))
}

func TestHealth_InvalidTimeoutFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock.NewMockHealthServiceClient(ctrl)

	out, err := runHealthCLI(t, client, "health", "--timeout=abc")
	require.Error(t, err)
	assert.Empty(t, out)
	assert.Contains(t, app.FormatError(err), `flag: invalid timeout "abc":`)
}

func TestHealth_ContextHasTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := mock.NewMockHealthServiceClient(ctrl)

	var observed context.Context
	client.EXPECT().
		V1Check(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ *feediumapi.V1CheckRequest, _ ...grpc.CallOption) (*feediumapi.V1CheckResponse, error) {
			observed = ctx
			return &feediumapi.V1CheckResponse{Status: "SERVING"}, nil
		})

	_, err := runHealthCLI(t, client, "health", "--timeout=5s")
	require.NoError(t, err)
	require.NotNil(t, observed)
	_, ok := observed.Deadline()
	assert.True(t, ok, "per-RPC context must carry a deadline (INV-04)")
}
