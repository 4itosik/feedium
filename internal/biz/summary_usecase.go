package biz

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/conf"
)

const (
	defaultCronInterval       = time.Hour
	defaultEventTTL           = 24 * time.Hour
	defaultCumulativeWindow   = 72 * time.Hour
	defaultCumulativeMaxChars = 50000
	defaultLLMMaxRetries      = 3
	defaultReaperMaxAttempts  = 5
	defaultReaperGracePeriod  = 30 * time.Second
	defaultBackoffBase        = 10 * time.Second
	defaultBackoffMax         = 10 * time.Minute
	defaultListPageSize       = 500
	llmRetryBackoffBase       = 2
	schedulerListPageSize     = 1
	reaperBatchLimit          = 100
)

type SummaryUsecase struct {
	summaryRepo SummaryRepo
	outboxRepo  SummaryOutboxRepo
	sourceRepo  SourceRepo
	postRepo    PostRepo
	llmProvider LLMProvider
	txManager   TxManager
	cfg         *conf.Summary
}

func NewSummaryUsecase(
	summaryRepo SummaryRepo,
	outboxRepo SummaryOutboxRepo,
	sourceRepo SourceRepo,
	postRepo PostRepo,
	llmProvider LLMProvider,
	txManager TxManager,
	cfg *conf.Summary,
) *SummaryUsecase {
	return &SummaryUsecase{
		summaryRepo: summaryRepo,
		outboxRepo:  outboxRepo,
		sourceRepo:  sourceRepo,
		postRepo:    postRepo,
		llmProvider: llmProvider,
		txManager:   txManager,
		cfg:         cfg,
	}
}

// TriggerSourceSummarize enqueues a summarize_source event for a cumulative source and
// bumps sources.next_summary_at by cron.interval in the same transaction (OQ-02).
// A manual trigger thus behaves as an early-fired scheduled tick: the next scheduled
// summarization still lands a full cron.interval after the manual one.
func (uc *SummaryUsecase) TriggerSourceSummarize(ctx context.Context, sourceID string) (string, bool, error) {
	source, err := uc.sourceRepo.Get(ctx, sourceID)
	if err != nil {
		return "", false, err
	}

	if ProcessingModeForType(source.Type) != ProcessingModeCumulative {
		return "", false, ErrSummarizeSelfContainedSrc
	}

	found, activeEvent, err := uc.outboxRepo.HasActiveEvent(ctx, sourceID, SummaryEventTypeSummarizeSource)
	if err != nil {
		return "", false, err
	}
	if found {
		return activeEvent.ID, true, nil
	}

	nextAt := time.Now().Add(uc.cronInterval())

	var eventID string
	txErr := uc.txManager.InTx(ctx, func(txCtx context.Context) error {
		event := NewSummaryEvent(SummaryEventTypeSummarizeSource, sourceID, nil)
		saved, saveErr := uc.outboxRepo.Save(txCtx, event)
		if saveErr != nil {
			return saveErr
		}
		eventID = saved.ID
		return uc.sourceRepo.BumpNextSummaryAt(txCtx, sourceID, nextAt)
	})
	if txErr != nil {
		return "", false, txErr
	}

	return eventID, false, nil
}

func (uc *SummaryUsecase) GetSummaryEvent(ctx context.Context, id string) (*SummaryEvent, error) {
	event, err := uc.outboxRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (uc *SummaryUsecase) GetSummary(ctx context.Context, id string) (Summary, error) {
	return uc.summaryRepo.Get(ctx, id)
}

func (uc *SummaryUsecase) ListPostSummaries(ctx context.Context, postID string) ([]Summary, error) {
	return uc.summaryRepo.ListByPost(ctx, postID)
}

func (uc *SummaryUsecase) ListSourceSummaries(
	ctx context.Context,
	sourceID string,
	pageSize int,
	pageToken string,
) (ListSummariesResult, error) {
	return uc.summaryRepo.ListBySource(ctx, sourceID, pageSize, pageToken)
}

// ProcessPostEvent executes the summarize_post business logic: event-TTL gate,
// idempotency check (FM-05), fetch post, summarize via LLM, persist Summary.
// Returns the saved summary id. Terminal domain errors:
//   - ErrSummaryEventExpired: event is too old per outbox.event_ttl.
//   - ErrSummaryValidation: invalid event shape (missing post id) or empty LLM reply.
//   - ErrPostNotFound: referenced post vanished.
//
// All other errors are transient per IsTransientError.
func (uc *SummaryUsecase) ProcessPostEvent(ctx context.Context, event SummaryEvent) (string, error) {
	if err := uc.ensureNotExpired(event); err != nil {
		return "", err
	}
	if event.PostID == nil {
		return "", fmt.Errorf("%w: post id is nil for summarize_post event", ErrSummaryValidation)
	}

	existing, err := uc.summaryRepo.ListByPost(ctx, *event.PostID)
	if err != nil {
		return "", fmt.Errorf("check existing summary: %w", err)
	}
	if len(existing) > 0 {
		return existing[0].ID, nil
	}

	post, err := uc.postRepo.Get(ctx, *event.PostID)
	if err != nil {
		return "", fmt.Errorf("get post: %w", err)
	}

	text, err := uc.summarizeWithLLMRetry(ctx, post.Text)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	saved, err := uc.summaryRepo.Save(ctx, Summary{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    event.PostID,
		SourceID:  event.SourceID,
		Text:      text,
		WordCount: len(strings.Fields(text)),
		CreatedAt: time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("save summary: %w", err)
	}
	return saved.ID, nil
}

// ProcessSourceEvent executes the summarize_source business logic: event-TTL gate,
// build cumulative window bounded by the last summary, aggregate posts, summarize
// via LLM, persist Summary. Terminal domain errors:
//   - ErrSummaryEventExpired: event too old.
//   - ErrSummarySourceNoPosts: no new posts in the window; task finalizes Completed.
//   - ErrSummaryValidation: aggregated text exceeds cumulative.max_input_chars.
func (uc *SummaryUsecase) ProcessSourceEvent(ctx context.Context, event SummaryEvent) (string, error) {
	if err := uc.ensureNotExpired(event); err != nil {
		return "", err
	}

	maxWindow := uc.cfg.GetCumulative().GetMaxWindow().AsDuration()
	if maxWindow <= 0 {
		maxWindow = defaultCumulativeWindow
	}
	maxInputChars := int(uc.cfg.GetCumulative().GetMaxInputChars())
	if maxInputChars <= 0 {
		maxInputChars = defaultCumulativeMaxChars
	}

	lastSummary, err := uc.summaryRepo.GetLastBySource(ctx, event.SourceID)
	if err != nil {
		return "", fmt.Errorf("get last summary: %w", err)
	}

	now := time.Now()
	windowStart := now.Add(-maxWindow)
	if lastSummary != nil && lastSummary.CreatedAt.After(windowStart) {
		windowStart = lastSummary.CreatedAt
	}

	concat, err := uc.collectWindowPosts(ctx, event.SourceID, windowStart, now, maxInputChars)
	if err != nil {
		return "", err
	}
	if concat == "" {
		return "", ErrSummarySourceNoPosts
	}

	text, err := uc.summarizeWithLLMRetry(ctx, concat)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	saved, err := uc.summaryRepo.Save(ctx, Summary{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    nil,
		SourceID:  event.SourceID,
		Text:      text,
		WordCount: len(strings.Fields(text)),
		CreatedAt: time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("save summary: %w", err)
	}
	return saved.ID, nil
}

// ScheduleNextCumulative claims one due cumulative source (FOR UPDATE SKIP LOCKED),
// bumps next_summary_at by cron.interval, and enqueues a summarize_source event
// when the source has new posts since its last summary. All steps are atomic in
// a single transaction. Returns scheduled=true when a new event was enqueued.
//
// Returns (false, nil) when nothing is due (ErrNoSourceDue is consumed internally).
func (uc *SummaryUsecase) ScheduleNextCumulative(ctx context.Context) (bool, error) {
	cronInterval := uc.cronInterval()
	scheduled := false
	txErr := uc.txManager.InTx(ctx, func(txCtx context.Context) error {
		source, claimErr := uc.sourceRepo.ClaimDueCumulative(txCtx)
		if claimErr != nil {
			return claimErr
		}
		next := time.Now().Add(cronInterval)
		if err := uc.sourceRepo.BumpNextSummaryAt(txCtx, source.ID, next); err != nil {
			return err
		}
		enqueued, err := uc.maybeEnqueueSourceEvent(txCtx, source.ID)
		if err != nil {
			return err
		}
		scheduled = enqueued
		return nil
	})
	if txErr != nil {
		if errors.Is(txErr, ErrNoSourceDue) {
			return false, nil
		}
		return false, txErr
	}
	return scheduled, nil
}

// ReapStuckEvents terminates events whose lease expired past grace AND attempt_count
// has reached the retry budget. The guarded FailExpired UPDATE prevents stomping on
// a concurrent fresh claim (see FM-08). Returns the number of events terminated.
func (uc *SummaryUsecase) ReapStuckEvents(ctx context.Context) (int, error) {
	grace := uc.cfg.GetReaper().GetGrace().AsDuration()
	if grace <= 0 {
		grace = defaultReaperGracePeriod
	}
	maxAttempts := int(uc.cfg.GetWorker().GetMaxAttempts())
	if maxAttempts <= 0 {
		maxAttempts = defaultReaperMaxAttempts
	}
	events, err := uc.outboxRepo.ListLeaseExpired(ctx, grace, reaperBatchLimit)
	if err != nil {
		return 0, fmt.Errorf("list lease expired: %w", err)
	}

	terminated := 0
	for _, ev := range events {
		if ev.AttemptCount < maxAttempts {
			continue
		}
		ok, termErr := uc.outboxRepo.FailExpired(ctx, ev.ID, maxAttempts, grace, "max attempts exceeded")
		if termErr != nil {
			return terminated, fmt.Errorf("fail expired: %w", termErr)
		}
		if ok {
			terminated++
		}
	}
	return terminated, nil
}

// RetryPolicy returns the worker retry policy derived from configuration with
// sane code-level fallbacks for unset values.
func (uc *SummaryUsecase) RetryPolicy() RetryPolicy {
	policy := RetryPolicy{
		MaxAttempts: int(uc.cfg.GetWorker().GetMaxAttempts()),
		BackoffBase: uc.cfg.GetWorker().GetBackoffBase().AsDuration(),
		BackoffMax:  uc.cfg.GetWorker().GetBackoffMax().AsDuration(),
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaultReaperMaxAttempts
	}
	if policy.BackoffBase <= 0 {
		policy.BackoffBase = defaultBackoffBase
	}
	if policy.BackoffMax <= 0 {
		policy.BackoffMax = defaultBackoffMax
	}
	return policy
}

func (uc *SummaryUsecase) ensureNotExpired(event SummaryEvent) error {
	eventTTL := uc.cfg.GetOutbox().GetEventTtl().AsDuration()
	if eventTTL <= 0 {
		eventTTL = defaultEventTTL
	}
	if time.Since(event.CreatedAt) > eventTTL {
		return ErrSummaryEventExpired
	}
	return nil
}

func (uc *SummaryUsecase) cronInterval() time.Duration {
	cronInterval := uc.cfg.GetCron().GetInterval().AsDuration()
	if cronInterval <= 0 {
		cronInterval = defaultCronInterval
	}
	return cronInterval
}

// collectWindowPosts aggregates post bodies in the [start, end) window, stopping
// at maxChars. Empty return means no posts found (ErrSummarySourceNoPosts at caller).
func (uc *SummaryUsecase) collectWindowPosts(
	ctx context.Context,
	sourceID string,
	start, end time.Time,
	maxChars int,
) (string, error) {
	var sb strings.Builder
	pageToken := ""
	for {
		page, err := uc.postRepo.List(ctx, ListPostsFilter{
			SourceID:      sourceID,
			PageSize:      defaultListPageSize,
			PageToken:     pageToken,
			OrderBy:       SortByCreatedAt,
			OrderDir:      SortAsc,
			CreatedAfter:  &start,
			CreatedBefore: &end,
		})
		if err != nil {
			return "", fmt.Errorf("list posts: %w", err)
		}
		for _, p := range page.Items {
			sb.WriteString(p.Text)
			sb.WriteString("\n\n")
			if sb.Len() > maxChars {
				return "", fmt.Errorf("%w: input text exceeds max_input_chars limit", ErrSummaryValidation)
			}
		}
		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}
	return sb.String(), nil
}

// maybeEnqueueSourceEvent creates a summarize_source event only if no active one
// exists and at least one post is available beyond the last summary. Returns
// whether an event was enqueued.
func (uc *SummaryUsecase) maybeEnqueueSourceEvent(ctx context.Context, sourceID string) (bool, error) {
	active, _, err := uc.outboxRepo.HasActiveEvent(ctx, sourceID, SummaryEventTypeSummarizeSource)
	if err != nil {
		return false, err
	}
	if active {
		return false, nil
	}

	lastSummary, err := uc.summaryRepo.GetLastBySource(ctx, sourceID)
	if err != nil {
		return false, err
	}
	var createdAfter *time.Time
	if lastSummary != nil {
		createdAfter = &lastSummary.CreatedAt
	}
	now := time.Now()
	posts, err := uc.postRepo.List(ctx, ListPostsFilter{
		SourceID:      sourceID,
		PageSize:      schedulerListPageSize,
		OrderBy:       SortByCreatedAt,
		OrderDir:      SortDesc,
		CreatedAfter:  createdAfter,
		CreatedBefore: &now,
	})
	if err != nil {
		return false, err
	}
	if len(posts.Items) == 0 {
		return false, nil
	}

	event := NewSummaryEvent(SummaryEventTypeSummarizeSource, sourceID, nil)
	if _, saveErr := uc.outboxRepo.Save(ctx, event); saveErr != nil {
		// Unique partial index lost the race; treat as scheduled-by-peer, no error.
		if errors.Is(saveErr, ErrSummaryAlreadyProcessing) {
			return false, nil
		}
		return false, saveErr
	}
	return true, nil
}

// summarizeWithLLMRetry keeps the FT-005 inner retry contract (NS-04): up to
// llm.max_retries in-call attempts with exponential backoff. The outer FT-007
// event-level retry only engages if all LLM attempts fail.
func (uc *SummaryUsecase) summarizeWithLLMRetry(ctx context.Context, text string) (string, error) {
	maxRetries := int(uc.cfg.GetLlm().GetMaxRetries())
	if maxRetries <= 0 {
		maxRetries = defaultLLMMaxRetries
	}

	var lastErr error
	for attempt := range maxRetries {
		result, err := uc.llmProvider.Summarize(ctx, text)
		if err != nil {
			lastErr = err
			backoff := time.Duration(math.Pow(llmRetryBackoffBase, float64(attempt))) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			continue
		}
		if strings.TrimSpace(result) == "" {
			lastErr = fmt.Errorf("llm returned empty summary on attempt %d", attempt+1)
			continue
		}
		return result, nil
	}
	return "", fmt.Errorf("all %d attempts failed: %w", maxRetries, lastErr)
}
