//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

package summary

import (
	"context"
	"time"

	"github.com/google/uuid"

	"feedium/internal/app/post"
	"feedium/internal/app/source"
)

// OutboxEventRepository defines operations for outbox events.
type OutboxEventRepository interface {
	// FetchAndLockPending fetches the next pending event and locks it for processing.
	// Uses SELECT FOR UPDATE SKIP LOCKED to prevent concurrent processing.
	// Returns nil if no pending events are available.
	FetchAndLockPending(ctx context.Context) (*OutboxEvent, time.Time, error)

	// UpdateStatus updates the status of an event and optionally increments retry count.
	UpdateStatus(ctx context.Context, id uuid.UUID, status EventStatus, incrementRetry bool) error

	// Create creates a single outbox event and returns the populated event with ID.
	Create(ctx context.Context, event *OutboxEvent) error

	// CreateScheduledForType creates scheduled outbox events for all sources of a given type.
	// Returns the number of events created.
	CreateScheduledForType(ctx context.Context, sourceType source.Type, scheduledAt time.Time) (int, error)
}

// Repository defines operations for summaries.
type Repository interface {
	// Create creates a new summary and associates it with the given posts in a single transaction.
	Create(ctx context.Context, summary *Summary, postIDs []uuid.UUID) error
}

// PostQueryRepository defines read operations for posts.
type PostQueryRepository interface {
	// GetByID retrieves a post by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*post.Post, error)

	// FindUnprocessedBySource finds all unprocessed posts for a source since a given time.
	// A post is considered unprocessed if it has no entry in summary_posts.
	FindUnprocessedBySource(ctx context.Context, sourceID uuid.UUID, since time.Time) ([]post.Post, error)
}

// SourceQueryRepository defines read operations for sources.
type SourceQueryRepository interface {
	// GetByID retrieves a source by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*source.Source, error)
}
