package health_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/goleak"

	feediumv1 "github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/data"
	"github.com/4itosik/feedium/internal/service/health"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// Ignore testcontainers reaper goroutines
		goleak.IgnoreTopFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect.func1"),
		goleak.IgnoreTopFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect"),
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
		goleak.IgnoreTopFunction("net.(*conn).Read"),
		goleak.IgnoreTopFunction("net.(*conn).Write"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
	)
}

func setupPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:18.3-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
	)
	require.NoError(t, err)

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)

	// Wait for database to be actually ready to accept connections
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		pingErr := db.PingContext(ctx)
		if pingErr == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("database did not become ready in time: %v", pingErr)
		case <-time.After(100 * time.Millisecond):
			// Retry
		}
	}

	cleanup := func() {
		db.Close()
		container.Terminate(ctx)
	}

	return db, cleanup
}

func TestHealthService_Check_Success(t *testing.T) {
	db, cleanup := setupPostgres(t)
	defer cleanup()

	d := &data.Data{DB: db}
	healthRepo := data.NewHealthRepo(d, slog.Default())
	hs := health.NewHealthService(healthRepo, slog.Default())

	ctx := context.Background()
	status, ok := hs.Check(ctx)
	assert.True(t, ok)
	assert.Equal(t, "ok", status)
}

func TestHealthService_V1Check_Success(t *testing.T) {
	db, cleanup := setupPostgres(t)
	defer cleanup()

	d := &data.Data{DB: db}
	healthRepo := data.NewHealthRepo(d, slog.Default())
	hs := health.NewHealthService(healthRepo, slog.Default())

	ctx := context.Background()
	resp, err := hs.V1Check(ctx, &feediumv1.V1CheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.GetStatus())
}

func TestHealthService_HTTPHandler_Success(t *testing.T) {
	db, cleanup := setupPostgres(t)
	defer cleanup()

	d := &data.Data{DB: db}
	healthRepo := data.NewHealthRepo(d, slog.Default())
	hs := health.NewHealthService(healthRepo, slog.Default())

	req := httptest.NewRequest(stdhttp.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	health.HTTPHandler(hs).ServeHTTP(w, req)

	assert.Equal(t, stdhttp.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
}

func TestHealthService_HTTPHandler_MethodNotAllowed(t *testing.T) {
	mockPinger := &mockPinger{}
	hs := health.NewHealthService(mockPinger, slog.Default())

	req := httptest.NewRequest(stdhttp.MethodPost, "/healthz", nil)
	w := httptest.NewRecorder()

	health.HTTPHandler(hs).ServeHTTP(w, req)

	assert.Equal(t, stdhttp.StatusMethodNotAllowed, w.Code)
}

type slowPinger struct {
	delay time.Duration
}

func (s *slowPinger) Ping(ctx context.Context) error {
	select {
	case <-time.After(s.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestHealthService_Check_SlowPing(t *testing.T) {
	slowPinger := &slowPinger{delay: 2 * time.Second}
	hs := health.NewHealthService(slowPinger, slog.Default())

	start := time.Now()
	status, ok := hs.Check(context.Background())
	duration := time.Since(start)

	assert.False(t, ok)
	assert.Equal(t, "unavailable", status)
	assert.Less(t, duration.Seconds(), 1.2)
}

type failingPinger struct{}

func (f *failingPinger) Ping(_ context.Context) error {
	return sql.ErrConnDone
}

func TestHealthService_V1Check_Failure(t *testing.T) {
	failingPinger := &failingPinger{}
	hs := health.NewHealthService(failingPinger, slog.Default())

	resp, err := hs.V1Check(context.Background(), &feediumv1.V1CheckRequest{})
	assert.Nil(t, resp)
	assert.Error(t, err)
}

func TestHealthService_HTTPHandler_Failure(t *testing.T) {
	failingPinger := &failingPinger{}
	hs := health.NewHealthService(failingPinger, slog.Default())

	req := httptest.NewRequest(stdhttp.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	health.HTTPHandler(hs).ServeHTTP(w, req)

	assert.Equal(t, stdhttp.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "unavailable", body["status"])
}

func TestThrottledLogger(t *testing.T) {
	var logBuf bytes.Buffer
	handler := slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelError})
	logger := slog.New(handler)
	hs := health.NewHealthService(&failingPinger{}, logger)

	for range 10 {
		hs.Check(context.Background())
	}

	time.Sleep(1 * time.Second)
	hs.Check(context.Background())

	var logs []map[string]any
	decoder := json.NewDecoder(&logBuf)
	for decoder.More() {
		var log map[string]any
		if err := decoder.Decode(&log); err != nil {
			break
		}
		logs = append(logs, log)
	}

	assert.GreaterOrEqual(t, len(logs), 2)
}

func TestThrottledLogger_SuccessNotLogged(t *testing.T) {
	var logBuf bytes.Buffer
	handler := slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelError})
	logger := slog.New(handler)
	hs := health.NewHealthService(&mockPinger{}, logger)

	ctx := context.Background()
	for range 10 {
		hs.Check(ctx)
	}

	assert.Equal(t, 0, logBuf.Len())
}

func TestHealthService_CommonPool(t *testing.T) {
	db, cleanup := setupPostgres(t)
	defer cleanup()

	dataInstance := &data.Data{DB: db}
	healthRepo1 := data.NewHealthRepo(dataInstance, slog.Default())
	healthRepo2 := data.NewHealthRepo(dataInstance, slog.Default())

	hs1 := health.NewHealthService(healthRepo1, slog.Default())
	hs2 := health.NewHealthService(healthRepo2, slog.Default())

	ctx := context.Background()

	status1, ok1 := hs1.Check(ctx)
	status2, ok2 := hs2.Check(ctx)

	assert.True(t, ok1)
	assert.True(t, ok2)
	assert.Equal(t, "ok", status1)
	assert.Equal(t, "ok", status2)

	assert.Equal(t, healthRepo1.GetDB(), healthRepo2.GetDB(), "both repos should use the same DB pool")
	assert.Equal(t, dataInstance.DB, healthRepo1.GetDB(), "health repo should use the same DB pool as Data")
}

func TestHealthService_SamePoolAsWireGraph(t *testing.T) {
	db, cleanup := setupPostgres(t)
	defer cleanup()

	dataInstance := &data.Data{DB: db}
	healthRepo := data.NewHealthRepo(dataInstance, slog.Default())

	hs := health.NewHealthService(healthRepo, slog.Default())

	status, ok := hs.Check(context.Background())

	assert.True(t, ok)
	assert.Equal(t, "ok", status)
	assert.Equal(t, dataInstance.DB, healthRepo.GetDB(), "health repo should use the same DB pool as Data")
}

func TestHealthService_AnonymousAccess(t *testing.T) {
	db, cleanup := setupPostgres(t)
	defer cleanup()

	d := &data.Data{DB: db}
	healthRepo := data.NewHealthRepo(d, slog.Default())
	hs := health.NewHealthService(healthRepo, slog.Default())

	req := httptest.NewRequest(stdhttp.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	health.HTTPHandler(hs).ServeHTTP(w, req)

	assert.Equal(t, stdhttp.StatusOK, w.Code)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestHealthService_WithKratosHTTPServer(t *testing.T) {
	db, cleanup := setupPostgres(t)
	defer cleanup()

	d := &data.Data{DB: db}
	healthRepo := data.NewHealthRepo(d, slog.Default())
	hs := health.NewHealthService(healthRepo, slog.Default())

	ctx := context.Background()

	srv := kratoshttp.NewServer(kratoshttp.Address(":0"))
	srv.Handle("/healthz", health.HTTPHandler(hs))

	// Resolve endpoint on the main goroutine before Start() to avoid a data race:
	// both Endpoint() and Start() lazily init s.lis/s.endpoint without synchronization.
	u, err := srv.Endpoint()
	require.NoError(t, err)

	go srv.Start(ctx)
	defer srv.Stop(ctx)

	resp, err := stdhttp.Get(fmt.Sprintf("http://%s/healthz", u.Host))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, stdhttp.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]string
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)
	assert.Equal(t, "ok", result["status"])
}

func TestHealthService_WithKratosGRPCServer(t *testing.T) {
	db, cleanup := setupPostgres(t)
	defer cleanup()

	d := &data.Data{DB: db}
	healthRepo := data.NewHealthRepo(d, slog.Default())
	hs := health.NewHealthService(healthRepo, slog.Default())

	ctx := context.Background()

	srv := kratosgrpc.NewServer(kratosgrpc.Address(":0"))
	feediumv1.RegisterHealthServiceServer(srv, hs)

	// Resolve endpoint on the main goroutine before Start() to avoid a data race:
	// both Endpoint() and Start() lazily init s.lis/s.endpoint without synchronization.
	u, err := srv.Endpoint()
	require.NoError(t, err)

	go srv.Start(ctx)
	defer srv.Stop(ctx)

	conn, err := kratosgrpc.DialInsecure(ctx, kratosgrpc.WithEndpoint(u.Host))
	require.NoError(t, err)
	defer conn.Close()

	client := feediumv1.NewHealthServiceClient(conn)
	resp, err := client.V1Check(ctx, &feediumv1.V1CheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.GetStatus())
}

type mockPinger struct{}

func (m *mockPinger) Ping(_ context.Context) error {
	return nil
}
