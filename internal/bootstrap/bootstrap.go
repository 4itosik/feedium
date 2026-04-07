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
	summaryv1connect "feedium/api/summary/v1/summaryv1connect"
	postsvc "feedium/internal/app/post"
	postconnect "feedium/internal/app/post/adapters/connect"
	postpg "feedium/internal/app/post/adapters/postgres"
	sourcesvc "feedium/internal/app/source"
	sourceconnect "feedium/internal/app/source/adapters/connect"
	sourcepg "feedium/internal/app/source/adapters/postgres"
	summarysvc "feedium/internal/app/summary"
	summaryconnect "feedium/internal/app/summary/adapters/connect"
	openrouter "feedium/internal/app/summary/adapters/openrouter"
	summarypg "feedium/internal/app/summary/adapters/postgres"
	componentsummary "feedium/internal/components/summary"
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

	// Step 3: Read OpenRouter configuration
	openRouterAPIKey := os.Getenv("OPENROUTER_API_KEY")
	if openRouterAPIKey == "" {
		return errors.New("OPENROUTER_API_KEY is required")
	}
	openRouterModel := os.Getenv("OPENROUTER_MODEL")
	if openRouterModel == "" {
		openRouterModel = openrouter.DefaultModel
	}

	// Step 4: Create ServeMux, register health endpoint and all service handlers
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", HealthHandler)
	_, scheduler, runner := registerHandlers(mux, db, log, openRouterAPIKey, openRouterModel)

	// Step 5: Create HTTP server
	server := &http.Server{
		Addr:              ":" + portStr,
		Handler:           mux,
		ReadHeaderTimeout: shutdownTimeout,
	}

	// Step 6: Start scheduler and worker runner in goroutines
	scheduler.Start(ctx)
	go runner.Start(ctx)

	// Step 7: Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		serveErr := server.ListenAndServe()
		if serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()

	log.InfoContext(ctx, "listening", "port", port)

	// Step 8: Wait for context cancellation or error
	select {
	case <-ctx.Done():
	case serveErr := <-errCh:
		return serveErr
	}

	// Step 9: Shutdown
	log.InfoContext(ctx, "shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}

// registerHandlers sets up all repositories, services, and HTTP handlers on mux.
// Returns the summary worker, scheduler, and worker runner for the caller to start.
func registerHandlers(
	mux *http.ServeMux,
	db *gorm.DB,
	log *slog.Logger,
	openRouterAPIKey string,
	openRouterModel string,
) (*summarysvc.Worker, *summarysvc.Scheduler, *componentsummary.WorkerRunner) {
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

	// Summary Connect handler
	summaryHandler := summaryconnect.New(summaryRepo, log)
	summaryPath, summaryH := summaryv1connect.NewSummaryServiceHandler(summaryHandler)
	mux.Handle(summaryPath, summaryH)

	// Create OpenRouter processor
	processor := openrouter.NewProcessor(openRouterAPIKey, openRouterModel)

	// Create worker and runner
	worker := summarysvc.NewWorker(outboxEventRepo, summaryRepo, postQueryRepo, sourceQueryRepo, processor, log)
	scheduler := summarysvc.NewScheduler(outboxEventRepo, log)
	runner := componentsummary.NewWorkerRunner(worker, log)

	return worker, scheduler, runner
}
