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
