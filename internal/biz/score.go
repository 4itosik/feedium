package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/conf"
)

//go:generate mockgen -source=score.go -destination=mock/score_mock.go -package=mock

// ScoreKey identifies a scoreable entity: exactly one of PostID / SummaryID is set
// (FT-020 CTR-01). PostID keys a post score (existing path); SummaryID keys a
// cumulative-digest score (summaries.post_id IS NULL). The same discriminator keys
// the persisted embedding cache and the score row.
type ScoreKey struct {
	PostID    *string
	SummaryID *string
}

// Score is the single current relevance score of a scoreable entity (FT-010 CTR-10,
// FT-020 CTR-01, REQ-02). Exactly one of PostID / SummaryID is set.
type Score struct {
	ID        string
	PostID    *string
	SummaryID *string
	Value     float64
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PostEmbedding is a scoreable entity's persisted embedding vector, reused across
// scorings while (InputHash, Model) match the current input text and embedding model
// (FT-013 INV-02, FT-020 CTR-01). Exactly one of PostID / SummaryID is set.
type PostEmbedding struct {
	PostID    *string
	SummaryID *string
	Vector    []float32
	Model     string
	InputHash string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ErrPostEmbeddingNotFound means no embedding is stored for the entity yet.
var ErrPostEmbeddingNotFound = errors.New("post embedding not found")

// PostEmbeddingRepo persists exactly one embedding per scoreable entity (CTR-01).
type PostEmbeddingRepo interface {
	Get(ctx context.Context, key ScoreKey) (PostEmbedding, error)
	Upsert(ctx context.Context, pe PostEmbedding) (PostEmbedding, error)
}

// hashInput is the content hash that keys vector validity (INV-02).
func hashInput(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
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
		return 0, fmt.Errorf(
			"%w: embedding dimension mismatch (%d vs %d)",
			ErrScoreValidation,
			len(a),
			len(b),
		)
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

// ScoreRepo persists the single current score per scoreable entity (INV-02).
type ScoreRepo interface {
	// Upsert writes the score, replacing any existing row for the entity in place
	// (ON CONFLICT on the entity's partial unique index) so there is exactly one
	// current score per post or per digest (FT-020 CTR-01).
	Upsert(ctx context.Context, score Score) (Score, error)
}

// ScoreUsecase executes the per-post scoring business logic (REQ-02, REQ-03).
type ScoreUsecase struct {
	scoreRepo         ScoreRepo
	interestRepo      InterestRepo
	summaryRepo       SummaryRepo
	outboxRepo        ScoreOutboxRepo
	scorer            Scorer
	postEmbeddingRepo PostEmbeddingRepo
	cfg               *conf.Scoring
}

// NewScoreUsecase creates a new ScoreUsecase.
func NewScoreUsecase(
	scoreRepo ScoreRepo,
	interestRepo InterestRepo,
	summaryRepo SummaryRepo,
	outboxRepo ScoreOutboxRepo,
	scorer Scorer,
	postEmbeddingRepo PostEmbeddingRepo,
	cfg *conf.Scoring,
) *ScoreUsecase {
	return &ScoreUsecase{
		scoreRepo:         scoreRepo,
		interestRepo:      interestRepo,
		summaryRepo:       summaryRepo,
		outboxRepo:        outboxRepo,
		scorer:            scorer,
		postEmbeddingRepo: postEmbeddingRepo,
		cfg:               cfg,
	}
}

// EnqueueDue enqueues scoring events for summarized-unscored posts (F2 step 1).
// Delegated to by the score_due_scheduler each tick. Returns the number enqueued.
func (uc *ScoreUsecase) EnqueueDue(ctx context.Context) (int, error) {
	posts, err := uc.outboxRepo.EnqueueDuePosts(ctx)
	if err != nil {
		return 0, fmt.Errorf("enqueue due posts: %w", err)
	}
	// Cumulative digests (summaries.post_id IS NULL) are scored independently on the
	// same outbox/worker pool (FT-020 REQ-01). Volume is negligible vs posts (ASM-01).
	digests, err := uc.outboxRepo.EnqueueDueDigests(ctx)
	if err != nil {
		return 0, fmt.Errorf("enqueue due digests: %w", err)
	}
	return posts + digests, nil
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
		ok, termErr := uc.outboxRepo.FailExpired(
			ctx,
			ev.ID,
			maxAttempts,
			grace,
			"max attempts exceeded",
		)
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
	key, text, err := uc.resolveScoreInput(ctx, event)
	if err != nil {
		return "", err
	}

	model := uc.scorer.Model()
	interestVectors, err := uc.interestRepo.ListInterestVectors(ctx)
	if err != nil {
		return "", fmt.Errorf("list interest vectors: %w", err)
	}
	// Never cosine across embedding models (INV-01). A stale-model interest is a
	// permanent, observable failure (the worker logs the failed event via FM-08) —
	// never a silent wrong score. OQ-01 default: fail terminally; auto re-embedding
	// interests on model change is a follow-up.
	interestEmbeddings := make([][]float32, 0, len(interestVectors))
	for _, iv := range interestVectors {
		if iv.Model != model {
			return "", fmt.Errorf("%w: interest embedded with model %q, current %q",
				ErrScoreValidation, iv.Model, model)
		}
		interestEmbeddings = append(interestEmbeddings, iv.Embedding)
	}

	value := 0.0
	// No same-model interests means the lowest score; skip the paid embed (FM-09).
	if len(interestEmbeddings) > 0 {
		postEmbedding, embedErr := uc.resolveVector(ctx, key, text, model)
		if embedErr != nil {
			return "", embedErr
		}
		value, err = Relevance(postEmbedding, interestEmbeddings)
		if err != nil {
			return "", err
		}
	}

	now := time.Now()
	saved, err := uc.scoreRepo.Upsert(ctx, Score{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    key.PostID,
		SummaryID: key.SummaryID,
		Value:     value,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return "", fmt.Errorf("upsert score: %w", err)
	}
	return saved.ID, nil
}

// resolveScoreInput resolves the scoreable entity's key and input text from the
// event. A post event (SummaryID == nil) reads the post's latest summary via
// ListByPost; a digest event (SummaryID != nil) reads that summary directly (FT-020
// REQ-01). A post with no summary is a terminal ErrScoreValidation (FM-08).
func (uc *ScoreUsecase) resolveScoreInput(
	ctx context.Context,
	event ScoreEvent,
) (ScoreKey, string, error) {
	if event.SummaryID != nil {
		summary, err := uc.summaryRepo.Get(ctx, *event.SummaryID)
		if err != nil {
			return ScoreKey{}, "", fmt.Errorf("get digest summary: %w", err)
		}
		return ScoreKey{SummaryID: event.SummaryID}, summary.Text, nil
	}
	summaries, err := uc.summaryRepo.ListByPost(ctx, *event.PostID)
	if err != nil {
		return ScoreKey{}, "", fmt.Errorf("list post summaries: %w", err)
	}
	if len(summaries) == 0 {
		return ScoreKey{}, "", fmt.Errorf("%w: post has no summary", ErrScoreValidation)
	}
	return ScoreKey{PostID: event.PostID}, summaries[0].Text, nil
}

// resolveVector returns the entity's embedding, reusing the persisted vector when
// (input_hash, model) still match (INV-02) and otherwise recomputing and persisting
// it, keyed by whichever of post_id / summary_id is set (FT-020 CTR-01). The paid
// Embed call happens only on the recompute path (REQ-01, REQ-02, REQ-03).
//
// Non-atomicity is deliberate: Embed + vector Upsert (here) and the subsequent score
// Upsert in ScorePost are NOT wrapped in a single DB transaction. Embed is an
// external/paid network call, and holding a DB transaction open across it is the
// ER-01 hazard. The vector is a recoverable cache (INV-03): a crash between the
// vector Upsert and the score Upsert is safely handled by the score-event retry,
// which finds and reuses the persisted vector. Do NOT wrap this sequence in a tx.
func (uc *ScoreUsecase) resolveVector(
	ctx context.Context, key ScoreKey, text, model string,
) ([]float32, error) {
	inputHash := hashInput(text)
	stored, err := uc.postEmbeddingRepo.Get(ctx, key)
	switch {
	case err == nil && stored.Model == model && stored.InputHash == inputHash:
		return stored.Vector, nil
	case err != nil && !errors.Is(err, ErrPostEmbeddingNotFound):
		return nil, fmt.Errorf("get embedding: %w", err)
	}
	vector, embedErr := uc.scorer.Embed(ctx, text)
	if embedErr != nil {
		return nil, fmt.Errorf("embed summary: %w", embedErr)
	}
	if _, upErr := uc.postEmbeddingRepo.Upsert(ctx, PostEmbedding{
		PostID: key.PostID, SummaryID: key.SummaryID, Vector: vector, Model: model, InputHash: inputHash,
	}); upErr != nil {
		return nil, fmt.Errorf("persist embedding: %w", upErr)
	}
	return vector, nil
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
