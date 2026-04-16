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
	ID          string
	PostID      *string
	SourceID    string
	EventType   SummaryEventType
	Status      SummaryEventStatus
	SummaryID   *string
	Error       *string
	CreatedAt   time.Time
	ProcessedAt *time.Time
}

var (
	ErrSummaryNotFound           = errors.New("summary not found")
	ErrSummaryEventNotFound      = errors.New("summary event not found")
	ErrSummaryAlreadyProcessing  = errors.New("summary already processing")
	ErrSummarizeSelfContainedSrc = errors.New("cannot summarize self-contained source")
	ErrSummaryValidation         = errors.New("summary validation failed")
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
	ListPending(ctx context.Context, limit int) ([]SummaryEvent, error)
	UpdateStatus(ctx context.Context, id string, status SummaryEventStatus, summaryID *string, errText *string) error
	HasActiveEvent(ctx context.Context, sourceID string, eventType SummaryEventType) (bool, *SummaryEvent, error)
}

type LLMProvider interface {
	Summarize(ctx context.Context, text string) (string, error)
}
