package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

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
	mux.HandleFunc("GET /healthz", healthHandler)

	// Step 4: Create HTTP server
	server := &http.Server{
		Addr:    ":" + portStr,
		Handler: mux,
	}

	// Step 5: Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Step 6: Log that we're listening
	log.Info("listening", "port", port)

	// Step 7: Wait for context cancellation or error
	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	// Step 8: Shutdown
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}
