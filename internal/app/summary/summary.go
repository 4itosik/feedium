package summary

import (
	"time"

	"github.com/google/uuid"
)

type Summary struct {
	ID        uuid.UUID
	SourceID  uuid.UUID
	EventID   uuid.UUID
	Content   string
	CreatedAt time.Time
}
