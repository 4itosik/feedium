package biz

import (
	"context"
	"errors"
	"time"
)

//go:generate mockgen -source=score_outbox.go -destination=mock/score_outbox_mock.go -package=mock

// ScoreEventStatus is the lifecycle state of a scoring outbox row (CTR-11).
type ScoreEventStatus string

const (
	ScoreEventStatusPending    ScoreEventStatus = "pending"
	ScoreEventStatusProcessing ScoreEventStatus = "processing"
	ScoreEventStatusCompleted  ScoreEventStatus = "completed"
	ScoreEventStatusFailed     ScoreEventStatus = "failed"
	ScoreEventStatusExpired    ScoreEventStatus = "expired"
)

// ScoreEvent is a unit of scoring work for one post (CTR-11), a structural copy
// of SummaryEvent restricted to the single scoring event type.
type ScoreEvent struct {
	ID           string
	PostID       string
	Status       ScoreEventStatus
	Error        *string
	AttemptCount int
	CreatedAt    time.Time
	ProcessedAt  *time.Time
}

// Sentinel errors for the scoring outbox.
var (
	ErrScoreEventNotFound    = errors.New("score event not found")
	ErrNoScoreEventAvailable = errors.New("no score event available")
)

// ScoreOutboxRepo is the scoring outbox: scheduler enqueues, worker pool claims
// and finalizes under a lease. Mirrors SummaryOutboxRepo's lease/retry contract.
type ScoreOutboxRepo interface {
	// EnqueueDuePosts inserts a pending score_event for every post that has a
	// summary, has no score, and has no active score_event. Idempotent via
	// INSERT ... ON CONFLICT DO NOTHING on the partial unique index (INV-04).
	// Returns the number of events enqueued.
	EnqueueDuePosts(ctx context.Context) (int, error)
	// EnqueueScoredPosts inserts a pending score_event for every post that already
	// has a score and no active event — the re-scoring path on interest change
	// (FT-013 CTR-03). Idempotent via ON CONFLICT DO NOTHING on the partial unique
	// index (INV-04). Returns the number of events enqueued.
	EnqueueScoredPosts(ctx context.Context) (int, error)
	// ClaimOne atomically claims one eligible event (pending+due, or processing
	// with expired lease) under FOR UPDATE SKIP LOCKED, marking it processing
	// with the caller's lease and incrementing attempt_count. Returns
	// ErrNoScoreEventAvailable when none is eligible.
	ClaimOne(ctx context.Context, workerID string, leaseTTL time.Duration) (ScoreEvent, error)
	// ExtendLease refreshes locked_until for a live lease owned by workerID;
	// returns ErrLeaseLost if the lease was lost.
	ExtendLease(ctx context.Context, eventID, workerID string, leaseTTL time.Duration) error
	// FinalizeWithLease writes a terminal status under a live lease guard;
	// returns ErrLeaseLost otherwise.
	FinalizeWithLease(ctx context.Context, eventID, workerID string, status ScoreEventStatus, errText *string) error
	// MarkForRetry resets the event to pending with next_attempt_at=retryAt under
	// a live lease guard; returns ErrLeaseLost otherwise.
	MarkForRetry(ctx context.Context, eventID, workerID string, retryAt time.Time, errText string) error
	// ListLeaseExpired returns processing events whose lease expired past grace.
	ListLeaseExpired(ctx context.Context, grace time.Duration, limit int) ([]ScoreEvent, error)
	// FailExpired terminally fails a stuck event only when its lease is past grace
	// and attempt_count has reached maxAttempts. Returns true when terminated.
	FailExpired(ctx context.Context, eventID string, maxAttempts int, grace time.Duration, errText string) (bool, error)
}
