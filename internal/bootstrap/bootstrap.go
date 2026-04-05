package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	sourcev1connect "feedium/api/source/v1/sourcev1connect"
	sourcesvc "feedium/internal/app/source"
	sourceconnect "feedium/internal/app/source/adapters/connect"
	sourcepg "feedium/internal/app/source/adapters/postgres"
	"feedium/internal/platform/postgres"
)

const shutdownTimeout = 5 * time.Second

func Run(ctx context.Context, log *slog.Logger) error {
	// Step 1: Read PORT from environment, default to "8080"
	portStr := os.Getenv("PORT")
	if portStr == "" {
		portStr = "8080"
	}

	// Step 2: Validate port
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid PORT: %w", err)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("PORT out of range: %d", port)
	}

	// Step 3: Create ServeMux and register health endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", HealthHandler)

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	db, err := postgres.Open(dsn)
	if err != nil {
		return err
	}
	repo := sourcepg.New(db)
	service := sourcesvc.NewService(repo, log)
	handler := sourceconnect.New(service, log)
	path, h := sourcev1connect.NewSourceServiceHandler(handler)
	mux.Handle(path, h)

	// Step 4: Create HTTP server
	server := &http.Server{
		Addr:              ":" + portStr,
		Handler:           mux,
		ReadHeaderTimeout: shutdownTimeout,
	}

	// Step 5: Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		serveErr := server.ListenAndServe()
		if serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()

	// Step 6: Log that we're listening
	log.InfoContext(ctx, "listening", "port", port)

	// Step 7: Wait for context cancellation or error
	select {
	case <-ctx.Done():
	case serveErr := <-errCh:
		return serveErr
	}

	// Step 8: Shutdown
	log.InfoContext(ctx, "shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}
