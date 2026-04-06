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

	"gorm.io/gorm"
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

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	db, err := postgres.Open(dsn)
	if err != nil {
		return err
	}

	// Step 3: Create ServeMux, register health endpoint and all service handlers
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", HealthHandler)
	worker, scheduler := registerHandlers(mux, db, log)

	// Step 4: Create HTTP server
	server := &http.Server{
		Addr:              ":" + portStr,
		Handler:           mux,
		ReadHeaderTimeout: shutdownTimeout,
	}

	// Step 5: Start scheduler and worker in goroutines
	scheduler.Start(ctx)
	workerErrCh := make(chan error, 1)
	go startWorker(ctx, worker, log, workerErrCh)

	// Step 6: Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		serveErr := server.ListenAndServe()
		if serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()

	log.InfoContext(ctx, "listening", "port", port)

	// Step 7: Wait for context cancellation or error
	select {
	case <-ctx.Done():
	case serveErr := <-errCh:
		return serveErr
	case workerErr := <-workerErrCh:
		return workerErr
	}

	// Step 8: Shutdown
	log.InfoContext(ctx, "shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}

// registerHandlers sets up all repositories, services, and HTTP handlers on mux.
// Returns the summary worker and scheduler for the caller to start.
func registerHandlers(mux *http.ServeMux, db *gorm.DB, log *slog.Logger) (*summarysvc.Worker, *summarysvc.Scheduler) {
	outboxEventRepo := summarypg.NewOutboxEventRepository(db)
	summaryRepo := summarypg.NewSummaryRepository(db)
	postQueryRepo := summarypg.NewPostQueryRepository(db)
	sourceQueryRepo := summarypg.NewSourceQueryRepository(db)

	sourceRepo := sourcepg.New(db)
	sourceSvc := sourcesvc.NewService(sourceRepo, log)
	sourceHandler := sourceconnect.NewWithProcessing(sourceSvc, log, outboxEventRepo, sourceQueryRepo)
	sourcePath, sourceH := sourcev1connect.NewSourceServiceHandler(sourceHandler)
	mux.Handle(sourcePath, sourceH)

	postRepo := postpg.New(db)
	postSvc := postsvc.NewService(postRepo, log)
	postSvc.SetOutboxBuilder(summarysvc.NewOutboxBuilder(sourceQueryRepo))
	postHandler := postconnect.New(postSvc, log)
	postPath, postH := postv1connect.NewPostServiceHandler(postHandler)
	mux.Handle(postPath, postH)

	worker := summarysvc.NewWorker(outboxEventRepo, summaryRepo, postQueryRepo, sourceQueryRepo, &stubProcessor{}, log)
	scheduler := summarysvc.NewScheduler(outboxEventRepo, log)
	return worker, scheduler
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
			if _, err := worker.ProcessNext(ctx); err != nil {
				log.ErrorContext(ctx, "worker error", "error", err)
				errCh <- err
				return
			}
		}
	}
}

// stubProcessor is a temporary processor implementation.
type stubProcessor struct{}

func (s *stubProcessor) Process(_ context.Context, _ []postsvc.Post) (string, error) {
	return "stub: processing not implemented", nil
}
