package app_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	feediumapi "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/cmd/feediumctl/internal/app"
)

func runCLI(t *testing.T, factory app.HealthClientFactory, args ...string) (stdout string, err error) {
	t.Helper()
	cmd := app.NewRootCommandWithHealth(factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	return buf.String(), err
}

func TestHealth_HappyPath_Table(t *testing.T) {
	var captured *feediumapi.V1CheckRequest
	factory := app.StubHealthFactory(func(_ context.Context, in *feediumapi.V1CheckRequest) (*feediumapi.V1CheckResponse, error) {
		captured = in
		return &feediumapi.V1CheckResponse{Status: "SERVING"}, nil
	})
	out, err := runCLI(t, factory, "health")
	require.NoError(t, err)
	assert.Equal(t, "FIELD | VALUE\nstatus | SERVING\n", out)
	require.NotNil(t, captured, "V1CheckRequest must be dispatched to the client")
	assert.Empty(t, captured.String(), "V1CheckRequest must be empty (FR-01)")
}

func TestHealth_HappyPath_JSON(t *testing.T) {
	factory := app.StubHealthFactory(func(_ context.Context, _ *feediumapi.V1CheckRequest) (*feediumapi.V1CheckResponse, error) {
		return &feediumapi.V1CheckResponse{Status: "SERVING"}, nil
	})
	out, err := runCLI(t, factory, "health", "--output=json")
	require.NoError(t, err)
	assert.Contains(t, out, `"status"`)
	assert.Contains(t, out, `"SERVING"`)
}

func TestHealth_UnavailableRoutedThroughRPCFormatter(t *testing.T) {
	factory := app.StubHealthFactory(func(_ context.Context, _ *feediumapi.V1CheckRequest) (*feediumapi.V1CheckResponse, error) {
		return nil, status.Error(codes.Unavailable, "connection refused")
	})
	out, err := runCLI(t, factory, "health")
	require.Error(t, err)
	assert.Empty(t, out, "stdout must be empty on error (INV-02)")
	assert.Equal(t, "code=Unavailable message=connection refused", err.Error())
}

func TestHealth_OutputValidation(t *testing.T) {
	factory := app.StubHealthFactory(func(_ context.Context, _ *feediumapi.V1CheckRequest) (*feediumapi.V1CheckResponse, error) {
		t.Fatal("V1Check must not be called when --output is invalid")
		return nil, nil
	})
	out, err := runCLI(t, factory, "health", "--output=xml")
	require.Error(t, err)
	assert.Empty(t, out)
	assert.Equal(t, `output: invalid value "xml" (allowed: table,json,yaml)`, err.Error())
}

func TestHealth_InvalidTimeoutFlag(t *testing.T) {
	factory := app.StubHealthFactory(func(_ context.Context, _ *feediumapi.V1CheckRequest) (*feediumapi.V1CheckResponse, error) {
		t.Fatal("V1Check must not be called when --timeout is invalid")
		return nil, nil
	})
	out, err := runCLI(t, factory, "health", "--timeout=abc")
	require.Error(t, err)
	assert.Empty(t, out)
	assert.Contains(t, err.Error(), `flag: invalid timeout "abc":`)
}

func TestHealth_ContextHasTimeout(t *testing.T) {
	var observed context.Context
	factory := app.StubHealthFactory(func(ctx context.Context, _ *feediumapi.V1CheckRequest) (*feediumapi.V1CheckResponse, error) {
		observed = ctx
		return &feediumapi.V1CheckResponse{Status: "SERVING"}, nil
	})
	_, err := runCLI(t, factory, "health", "--timeout=5s")
	require.NoError(t, err)
	require.NotNil(t, observed)
	_, ok := observed.Deadline()
	assert.True(t, ok, "per-RPC context must carry a deadline (INV-04)")
}
