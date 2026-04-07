package summary

import (
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	EventTypeImmediate EventType = "IMMEDIATE"
	EventTypeScheduled EventType = "SCHEDULED"
	EventTypeManual    EventType = "MANUAL"
)

type EventStatus string

const (
	EventStatusPending    EventStatus = "PENDING"
	EventStatusProcessing EventStatus = "PROCESSING"
	EventStatusCompleted  EventStatus = "COMPLETED"
	EventStatusFailed     EventStatus = "FAILED"
)

const (
	// MaxRetries is the maximum number of retry attempts for transient errors.
	MaxRetries = 3
)

type OutboxEvent struct {
	ID          uuid.UUID
	SourceID    uuid.UUID
	PostID      *uuid.UUID
	EventType   EventType
	Status      EventStatus
	RetryCount  int
	ScheduledAt *time.Time
	CreatedAt   time.Time
}
