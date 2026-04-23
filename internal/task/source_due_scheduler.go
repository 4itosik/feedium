package task

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/conf"
)

const defaultSourceSchedulerPoll = 5 * time.Second

// SourceDueScheduler implements REQ-06: per-source due-time planning for cumulative
// sources. On each tick it claims one due source under FOR UPDATE SKIP LOCKED,
// checks HasActiveEvent / presence of new posts, enqueues a summarize_source event
// and bumps next_summary_at by cron.interval. Replaces the in-process CronWorker
// ticker from FT-005.
type SourceDueScheduler struct {
	outboxRepo  biz.SummaryOutboxRepo
	sourceRepo  biz.SourceRepo
	summaryRepo biz.SummaryRepo
	postRepo    biz.PostRepo
	txManager   biz.TxManager
	cfg         *conf.Summary
	log         *slog.Logger

	done     chan struct{}
	doneOnce sync.Once
}

func NewSourceDueScheduler(
	outboxRepo biz.SummaryOutboxRepo,
	sourceRepo biz.SourceRepo,
	summaryRepo biz.SummaryRepo,
	postRepo biz.PostRepo,
	txManager biz.TxManager,
	cfg *conf.Summary,
	logger *slog.Logger,
) *SourceDueScheduler {
	return &SourceDueScheduler{
		outboxRepo:  outboxRepo,
		sourceRepo:  sourceRepo,
		summaryRepo: summaryRepo,
		postRepo:    postRepo,
		txManager:   txManager,
		cfg:         cfg,
		log:         logger,
		done:        make(chan struct{}),
	}
}

func (s *SourceDueScheduler) Start(ctx context.Context) error {
	pollInterval := s.cfg.GetSourceScheduler().GetPollInterval().AsDuration()
	if pollInterval <= 0 {
		pollInterval = defaultSourceSchedulerPoll
	}

	go s.run(ctx, pollInterval)
	return nil
}

func (s *SourceDueScheduler) Stop(_ context.Context) error {
	s.doneOnce.Do(func() { close(s.done) })
	return nil
}

func (s *SourceDueScheduler) run(ctx context.Context, pollInterval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		s.tick(ctx)

		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return
		case <-s.done:
			return
		}
	}
}

func (s *SourceDueScheduler) tick(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		txErr := s.txManager.InTx(ctx, func(txCtx context.Context) error {
			source, claimErr := s.sourceRepo.ClaimDueCumulative(txCtx)
			if claimErr != nil {
				return claimErr
			}
			return s.processClaimedSource(txCtx, source)
		})
		if txErr != nil {
			if errors.Is(txErr, biz.ErrNoSourceDue) {
				return
			}
			s.log.ErrorContext(ctx, "source scheduler tick failed", "error", txErr)
			return
		}
	}
}

func (s *SourceDueScheduler) processClaimedSource(ctx context.Context, source biz.Source) error {
	cronInterval := s.cfg.GetCron().GetInterval().AsDuration()
	if cronInterval <= 0 {
		cronInterval = time.Hour
	}
	next := time.Now().Add(cronInterval)

	if err := s.sourceRepo.BumpNextSummaryAt(ctx, source.ID, next); err != nil {
		return err
	}

	active, _, err := s.outboxRepo.HasActiveEvent(ctx, source.ID, biz.SummaryEventTypeSummarizeSource)
	if err != nil {
		return err
	}
	if active {
		return nil
	}

	lastSummary, err := s.summaryRepo.GetLastBySource(ctx, source.ID)
	if err != nil {
		return err
	}

	var createdAfter *time.Time
	if lastSummary != nil {
		createdAfter = &lastSummary.CreatedAt
	}
	now := time.Now()
	posts, err := s.postRepo.List(ctx, biz.ListPostsFilter{
		SourceID:      source.ID,
		PageSize:      1,
		OrderBy:       biz.SortByCreatedAt,
		OrderDir:      biz.SortDesc,
		CreatedAfter:  createdAfter,
		CreatedBefore: &now,
	})
	if err != nil {
		return err
	}
	if len(posts.Items) == 0 {
		return nil
	}

	event := biz.NewSummaryEvent(biz.SummaryEventTypeSummarizeSource, source.ID, nil)
	if _, saveErr := s.outboxRepo.Save(ctx, event); saveErr != nil {
		if errors.Is(saveErr, biz.ErrSummaryAlreadyProcessing) {
			return nil
		}
		return saveErr
	}
	s.log.InfoContext(ctx, "source scheduled for summarization",
		"source_id", source.ID, "next_summary_at", next)
	return nil
}
