package task

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

type CronWorker struct {
	outboxRepo  biz.SummaryOutboxRepo
	sourceRepo  biz.SourceRepo
	summaryRepo biz.SummaryRepo
	postRepo    biz.PostRepo
	cfg         *conf.Summary
	log         *slog.Logger
	done        chan struct{}
	wg          sync.WaitGroup
}

func NewCronWorker(
	outboxRepo biz.SummaryOutboxRepo,
	sourceRepo biz.SourceRepo,
	summaryRepo biz.SummaryRepo,
	postRepo biz.PostRepo,
	cfg *conf.Summary,
	logger *slog.Logger,
) *CronWorker {
	return &CronWorker{
		outboxRepo:  outboxRepo,
		sourceRepo:  sourceRepo,
		summaryRepo: summaryRepo,
		postRepo:    postRepo,
		cfg:         cfg,
		log:         logger,
		done:        make(chan struct{}),
	}
}

func (w *CronWorker) Start(ctx context.Context) error {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		interval := w.cfg.GetCron().GetInterval().AsDuration()
		if interval == 0 {
			interval = time.Hour
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.done:
				return
			default:
				w.tick(ctx)
				select {
				case <-time.After(interval):
				case <-ctx.Done():
					return
				case <-w.done:
					return
				}
			}
		}
	}()
	return nil
}

func (w *CronWorker) Stop(ctx context.Context) error {
	close(w.done)
	w.wg.Wait()
	return nil
}

func (w *CronWorker) tick(ctx context.Context) {
	mode := biz.ProcessingModeCumulative
	pageToken := ""
	for {
		sources, err := w.sourceRepo.List(ctx, biz.ListSourcesFilter{
			ProcessingMode: &mode,
			PageSize:       500,
			PageToken:      pageToken,
		})
		if err != nil {
			w.log.Error("cron: failed to list sources", "error", err)
			return
		}

		for _, source := range sources.Items {
			w.processSource(ctx, source)
		}

		if sources.NextPageToken == "" {
			break
		}
		pageToken = sources.NextPageToken
	}
}

func (w *CronWorker) processSource(ctx context.Context, source biz.Source) {
	logger := w.log.With("source_id", source.ID, "cron", "true")

	found, _, err := w.outboxRepo.HasActiveEvent(ctx, source.ID, biz.SummaryEventTypeSummarizeSource)
	if err != nil {
		logger.Error("failed to check active event", "error", err)
		return
	}
	if found {
		return
	}

	lastSummary, err := w.summaryRepo.GetLastBySource(ctx, source.ID)
	if err != nil {
		logger.Error("failed to get last summary", "error", err)
		return
	}

	var createdAfter *time.Time
	if lastSummary != nil {
		createdAfter = &lastSummary.CreatedAt
	}

	now := time.Now()
	posts, err := w.postRepo.List(ctx, biz.ListPostsFilter{
		SourceID:      source.ID,
		PageSize:      1,
		OrderBy:       biz.SortByCreatedAt,
		OrderDir:      biz.SortDesc,
		CreatedAfter:  createdAfter,
		CreatedBefore: &now,
	})
	if err != nil {
		logger.Error("failed to check new posts", "error", err)
		return
	}

	if len(posts.Items) == 0 {
		return
	}

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, source.ID, nil)
	if _, saveErr := w.outboxRepo.Save(ctx, event); saveErr != nil {
		logger.Error("failed to create summary event", "error", saveErr)
		return
	}

	logger.Info("created summary event for cumulative source")
}
