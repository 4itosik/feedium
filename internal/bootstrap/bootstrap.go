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

	postv1connect "feedium/api/post/v1/postv1connect"
	sourcev1connect "feedium/api/source/v1/sourcev1connect"
	postsvc "feedium/internal/app/post"
	postconnect "feedium/internal/app/post/adapters/connect"
	postpg "feedium/internal/app/post/adapters/postgres"
	sourcesvc "feedium/internal/app/source"
	sourceconnect "feedium/internal/app/source/adapters/connect"
	sourcepg "feedium/internal/app/source/adapters/postgres"
	summarysvc "feedium/internal/app/summary"
	summarypg "feedium/internal/app/summary/adapters/postgres"
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

	// Setup summary repositories first (needed for outbox builder and source handler)
	outboxEventRepo := summarypg.NewOutboxEventRepository(db)
	summaryRepo := summarypg.NewSummaryRepository(db)
	postQueryRepo := summarypg.NewPostQueryRepository(db)
	sourceQueryRepo := summarypg.NewSourceQueryRepository(db)

	// Setup source service and handler
	repo := sourcepg.New(db)
	service := sourcesvc.NewService(repo, log)
	handler := sourceconnect.NewWithProcessing(service, log, outboxEventRepo, sourceQueryRepo)
	path, h := sourcev1connect.NewSourceServiceHandler(handler)
	mux.Handle(path, h)

	// Setup post service with outbox builder
	postRepo := postpg.New(db)
	postService := postsvc.NewService(postRepo, log)
	outboxBuilder := summarysvc.NewOutboxBuilder(sourceQueryRepo)
	postService.SetOutboxBuilder(outboxBuilder)
	postHandler := postconnect.New(postService, log)
	postPath, postH := postv1connect.NewPostServiceHandler(postHandler)
	mux.Handle(postPath, postH)

	// Setup summary processing pipeline
	processor := &stubProcessor{}
	summaryWorker := summarysvc.NewWorker(outboxEventRepo, summaryRepo, postQueryRepo, sourceQueryRepo, processor, log)

	// Setup scheduler for TELEGRAM_GROUP sources
	scheduler := summarysvc.NewScheduler(outboxEventRepo, log)

	// Step 4: Create HTTP server
	server := &http.Server{
		Addr:              ":" + portStr,
		Handler:           mux,
		ReadHeaderTimeout: shutdownTimeout,
	}

	// Step 5: Start scheduler in goroutine
	scheduler.Start(ctx)

	// Step 5b: Start worker in goroutine
	workerErrCh := make(chan error, 1)
	go startWorker(ctx, summaryWorker, log, workerErrCh)

	// Step 6: Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		serveErr := server.ListenAndServe()
		if serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()

	// Step 7: Log that we're listening
	log.InfoContext(ctx, "listening", "port", port)

	// Step 8: Wait for context cancellation or error
	select {
	case <-ctx.Done():
	case serveErr := <-errCh:
		return serveErr
	case workerErr := <-workerErrCh:
		return workerErr
	}

	// Step 9: Shutdown
	log.InfoContext(ctx, "shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}

// startWorker runs the summary processing worker with polling.
func startWorker(ctx context.Context, worker *summarysvc.Worker, log *slog.Logger, errCh chan<- error) {
	const pollInterval = 1 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processed, err := worker.ProcessNext(ctx)
			if err != nil {
				log.ErrorContext(ctx, "worker error", "error", err)
				errCh <- err
				return
			}
			// If no event was processed, wait before next poll
			if !processed {
				continue
			}
		}
	}
}

// stubProcessor is a temporary processor implementation.
type stubProcessor struct{}

func (s *stubProcessor) Process(ctx context.Context, posts []postsvc.Post) (string, error) {
	return "stub: processing not implemented", nil
}
