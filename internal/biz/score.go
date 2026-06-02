package biz

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/conf"
)

//go:generate mockgen -source=score.go -destination=mock/score_mock.go -package=mock

// Score is the single current relevance score of a post (FT-010 CTR-10, REQ-02).
type Score struct {
	ID        string
	PostID    string
	Value     float64
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Sentinel errors for scoring.
var (
	// ErrScoreValidation marks a permanent, non-retryable scoring failure: a
	// missing summary, a malformed embedding, or a vector dimension mismatch
	// (FM-08). The worker terminates the event as failed without retrying.
	ErrScoreValidation = errors.New("score validation failed")
)

// Cosine returns the cosine similarity of two equal-length vectors. A zero-length
// or mismatched pair is a permanent ErrScoreValidation (FM-08). A zero-magnitude
// vector yields 0 (orthogonal-by-convention) rather than NaN.
func Cosine(a, b []float32) (float64, error) {
	if len(a) == 0 || len(b) == 0 {
		return 0, fmt.Errorf("%w: empty embedding vector", ErrScoreValidation)
	}
	if len(a) != len(b) {
		return 0, fmt.Errorf("%w: embedding dimension mismatch (%d vs %d)", ErrScoreValidation, len(a), len(b))
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb)), nil
}

// Relevance computes a post's relevance as the maximum cosine similarity over the
// interest set, clamped to [0,1] (DEC-03, DEC-04). An empty interest set yields
// the lowest score 0 (FM-09). It is a pure function: it neither mutates inputs
// nor reads global state (INV-05).
func Relevance(postEmbedding []float32, interestEmbeddings [][]float32) (float64, error) {
	if len(interestEmbeddings) == 0 {
		return 0, nil
	}
	best := 0.0
	for _, interest := range interestEmbeddings {
		c, err := Cosine(postEmbedding, interest)
		if err != nil {
			return 0, err
		}
		if c > best {
			best = c
		}
	}
	if best < 0 {
		best = 0
	}
	if best > 1 {
		best = 1
	}
	return best, nil
}

// IsTransientScoreError classifies scoring errors. Permanent: validation/dimension
// failures (FM-08) and context cancellation/deadline. Everything else (provider
// timeouts, 5xx) is transient and retryable (FM-01).
func IsTransientScoreError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrScoreValidation) {
		return false
	}
	return true
}

// ScoreRepo persists the single current score per post (INV-02).
type ScoreRepo interface {
	// Upsert writes the score, replacing any existing row for the post in place
	// (ON CONFLICT (post_id) DO UPDATE) so there is exactly one current score.
	Upsert(ctx context.Context, score Score) (Score, error)
}

// ScoreUsecase executes the per-post scoring business logic (REQ-02, REQ-03).
type ScoreUsecase struct {
	scoreRepo    ScoreRepo
	interestRepo InterestRepo
	summaryRepo  SummaryRepo
	outboxRepo   ScoreOutboxRepo
	scorer       Scorer
	cfg          *conf.Scoring
}

// NewScoreUsecase creates a new ScoreUsecase.
func NewScoreUsecase(
	scoreRepo ScoreRepo,
	interestRepo InterestRepo,
	summaryRepo SummaryRepo,
	outboxRepo ScoreOutboxRepo,
	scorer Scorer,
	cfg *conf.Scoring,
) *ScoreUsecase {
	return &ScoreUsecase{
		scoreRepo:    scoreRepo,
		interestRepo: interestRepo,
		summaryRepo:  summaryRepo,
		outboxRepo:   outboxRepo,
		scorer:       scorer,
		cfg:          cfg,
	}
}

// EnqueueDue enqueues scoring events for summarized-unscored posts (F2 step 1).
// Delegated to by the score_due_scheduler each tick. Returns the number enqueued.
func (uc *ScoreUsecase) EnqueueDue(ctx context.Context) (int, error) {
	n, err := uc.outboxRepo.EnqueueDuePosts(ctx)
	if err != nil {
		return 0, fmt.Errorf("enqueue due posts: %w", err)
	}
	return n, nil
}

// ReapStuckScoreEvents terminates events whose lease expired past grace AND whose
// attempt_count reached the retry budget (INV-06). The guarded FailExpired UPDATE
// avoids stomping a concurrent fresh claim. Returns the number terminated.
func (uc *ScoreUsecase) ReapStuckScoreEvents(ctx context.Context) (int, error) {
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

// ScorePost reads the post's summary, computes relevance against the current
// interest set, and upserts the single current score (F2). Terminal domain
// errors: ErrScoreValidation (no summary, malformed embedding). Provider failures
// surface as transient errors handled by the worker (FM-01).
func (uc *ScoreUsecase) ScorePost(ctx context.Context, event ScoreEvent) (string, error) {
	summaries, err := uc.summaryRepo.ListByPost(ctx, event.PostID)
	if err != nil {
		return "", fmt.Errorf("list post summaries: %w", err)
	}
	if len(summaries) == 0 {
		return "", fmt.Errorf("%w: post has no summary", ErrScoreValidation)
	}

	interestEmbeddings, err := uc.interestRepo.ListEmbeddings(ctx)
	if err != nil {
		return "", fmt.Errorf("list interest embeddings: %w", err)
	}

	value := 0.0
	// No interests means the lowest score; skip the paid embed call entirely (FM-09).
	if len(interestEmbeddings) > 0 {
		postEmbedding, embedErr := uc.scorer.Embed(ctx, summaries[0].Text)
		if embedErr != nil {
			return "", fmt.Errorf("embed summary: %w", embedErr)
		}
		value, err = Relevance(postEmbedding, interestEmbeddings)
		if err != nil {
			return "", err
		}
	}

	now := time.Now()
	saved, err := uc.scoreRepo.Upsert(ctx, Score{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    event.PostID,
		Value:     value,
		Model:     uc.scorer.Model(),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return "", fmt.Errorf("upsert score: %w", err)
	}
	return saved.ID, nil
}

// RetryPolicy returns the scoring worker retry policy from config with code-level
// fallbacks for unset values (INV-08).
func (uc *ScoreUsecase) RetryPolicy() RetryPolicy {
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
