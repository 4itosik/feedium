package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SummaryEventType string

const (
	SummaryEventTypeSummarizePost   SummaryEventType = "summarize_post"
	SummaryEventTypeSummarizeSource SummaryEventType = "summarize_source"
)

type SummaryEventStatus string

const (
	SummaryEventStatusPending    SummaryEventStatus = "pending"
	SummaryEventStatusProcessing SummaryEventStatus = "processing"
	SummaryEventStatusCompleted  SummaryEventStatus = "completed"
	SummaryEventStatusFailed     SummaryEventStatus = "failed"
	SummaryEventStatusExpired    SummaryEventStatus = "expired"
)

type Summary struct {
	ID        string
	PostID    *string
	SourceID  string
	Text      string
	WordCount int
	CreatedAt time.Time
}

type SummaryEvent struct {
	ID           string
	PostID       *string
	SourceID     string
	EventType    SummaryEventType
	Status       SummaryEventStatus
	SummaryID    *string
	Error        *string
	CreatedAt    time.Time
	ProcessedAt  *time.Time
	AttemptCount int
}

var (
	ErrSummaryNotFound           = errors.New("summary not found")
	ErrSummaryEventNotFound      = errors.New("summary event not found")
	ErrSummaryAlreadyProcessing  = errors.New("summary already processing")
	ErrSummarizeSelfContainedSrc = errors.New("cannot summarize self-contained source")
	ErrSummaryValidation         = errors.New("summary validation failed")
	// ErrLeaseLost indicates the worker lost ownership of the lease (finalization attempted
	// against an expired or reassigned row). The worker must abort without writing
	// any terminal state — the row will be picked up again via the expired-lease path.
	ErrLeaseLost = errors.New("lease lost")
	// ErrNoEventAvailable indicates ClaimOne found no eligible row to claim.
	ErrNoEventAvailable = errors.New("no event available")
	// ErrNoSourceDue indicates ClaimDueCumulative found no source whose
	// next_summary_at is due.
	ErrNoSourceDue = errors.New("no source due")
)

func NewSummaryEvent(eventType SummaryEventType, sourceID string, postID *string) SummaryEvent {
	return SummaryEvent{
		ID:        uuid.Must(uuid.NewV7()).String(),
		PostID:    postID,
		SourceID:  sourceID,
		EventType: eventType,
		Status:    SummaryEventStatusPending,
		CreatedAt: time.Now(),
	}
}

func ValidateSummary(text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: text is empty", ErrSummaryValidation)
	}
	return nil
}

type ListSummariesResult struct {
	Items         []Summary
	NextPageToken string
}

type TxManager interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type SummaryRepo interface {
	Save(ctx context.Context, summary Summary) (Summary, error)
	Get(ctx context.Context, id string) (Summary, error)
	ListByPost(ctx context.Context, postID string) ([]Summary, error)
	ListBySource(ctx context.Context, sourceID string, pageSize int, pageToken string) (ListSummariesResult, error)
	GetLastBySource(ctx context.Context, sourceID string) (*Summary, error)
}

type SummaryOutboxRepo interface {
	Save(ctx context.Context, event SummaryEvent) (SummaryEvent, error)
	Get(ctx context.Context, id string) (SummaryEvent, error)
	UpdateStatus(ctx context.Context, id string, status SummaryEventStatus, summaryID *string, errText *string) error
	HasActiveEvent(ctx context.Context, sourceID string, eventType SummaryEventType) (bool, *SummaryEvent, error)
	// ClaimOne atomically selects one eligible event (pending and due, or processing with
	// expired lease) and marks it processing with locked_by=workerID and locked_until=now()+leaseTTL,
	// incrementing attempt_count. Returns ErrNoEventAvailable when the queue is empty.
	ClaimOne(ctx context.Context, workerID string, leaseTTL time.Duration) (SummaryEvent, error)
	// ExtendLease refreshes locked_until=now()+leaseTTL only if the worker still owns the
	// live lease (locked_by=workerID AND status='processing' AND locked_until>now()). If
	// the lease has been lost, returns ErrLeaseLost.
	ExtendLease(ctx context.Context, eventID, workerID string, leaseTTL time.Duration) error
	// FinalizeWithLease writes a terminal status (completed/failed/expired) only if the
	// worker still owns a live lease on the event (locked_by=workerID AND
	// locked_until>now() AND status='processing'). Returns ErrLeaseLost otherwise.
	FinalizeWithLease(
		ctx context.Context,
		eventID, workerID string,
		status SummaryEventStatus,
		summaryID *string,
		errText *string,
	) error
	// MarkForRetry resets the event back to pending with next_attempt_at=retryAt, clears
	// locked_by/locked_until, and stores the given error text. The update is guarded by
	// locked_by=workerID AND locked_until>now(); if the guard fails, returns ErrLeaseLost.
	MarkForRetry(ctx context.Context, eventID, workerID string, retryAt time.Time, errText string) error
	// ListLeaseExpired returns up to `limit` events in status='processing' whose
	// locked_until < now() - grace (i.e. stuck leases ready for reaping).
	ListLeaseExpired(ctx context.Context, grace time.Duration, limit int) ([]SummaryEvent, error)
}

type LLMProvider interface {
	Summarize(ctx context.Context, text string) (string, error)
}
