package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"feedium/internal/app/source"
	"feedium/internal/app/summary"
)

type OutboxEventRepository struct {
	db *gorm.DB
}

func NewOutboxEventRepository(db *gorm.DB) summary.OutboxEventRepository {
	return &OutboxEventRepository{db: db}
}

// FetchAndLockPending fetches the next pending event using SELECT FOR UPDATE SKIP LOCKED.
// Returns the event, lock acquisition timestamp, and any error.
// Returns nil if no pending events are available.
func (r *OutboxEventRepository) FetchAndLockPending(ctx context.Context) (*summary.OutboxEvent, time.Time, error) {
	var event summary.OutboxEvent
	lockTime := time.Now()

	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, time.Time{}, tx.Error
	}

	// Raw SELECT FOR UPDATE SKIP LOCKED query
	const query = `
		SELECT id, source_id, post_id, event_type, status, retry_count, scheduled_at, created_at
		FROM outbox_events
		WHERE status = $1 AND (scheduled_at IS NULL OR scheduled_at <= $2)
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`

	result := tx.Raw(query, summary.EventStatusPending, time.Now()).Scan(&event)
	if result.Error != nil {
		tx.Rollback()
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, time.Time{}, nil
		}
		return nil, time.Time{}, result.Error
	}

	// Check if event was found
	if result.RowsAffected == 0 {
		tx.Rollback()
		return nil, time.Time{}, nil
	}

	// Update status to PROCESSING in the same transaction
	updateResult := tx.Model(&event).Update("status", summary.EventStatusProcessing)
	if updateResult.Error != nil {
		tx.Rollback()
		return nil, time.Time{}, updateResult.Error
	}

	if err := tx.Commit().Error; err != nil {
		return nil, time.Time{}, err
	}

	return &event, lockTime, nil
}

// UpdateStatus updates the event status and optionally increments retry count.
func (r *OutboxEventRepository) UpdateStatus(
	ctx context.Context,
	id uuid.UUID,
	status summary.EventStatus,
	incrementRetry bool,
) error {
	if incrementRetry {
		// Use raw SQL to increment retry_count atomically
		return r.db.WithContext(ctx).
			Model(&summary.OutboxEvent{}).
			Where("id = ?", id).
			Updates(map[string]any{
				"status":      status,
				"retry_count": gorm.Expr("retry_count + ?", 1),
			}).Error
	}

	return r.db.WithContext(ctx).
		Model(&summary.OutboxEvent{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// Create creates a single outbox event.
func (r *OutboxEventRepository) Create(ctx context.Context, event *summary.OutboxEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(event).Error
}

// CreateScheduledForType creates scheduled outbox events for all sources of a given type.
func (r *OutboxEventRepository) CreateScheduledForType(
	ctx context.Context,
	sourceType source.Type,
	scheduledAt time.Time,
) (int, error) {
	const query = `
		INSERT INTO outbox_events (id, source_id, post_id, event_type, status, scheduled_at, created_at)
		SELECT gen_random_uuid(), id, NULL, $1, $2, $3, $4
		FROM sources WHERE type = $5
	`

	result := r.db.WithContext(ctx).Exec(
		query,
		summary.EventTypeScheduled,
		summary.EventStatusPending,
		scheduledAt,
		time.Now().UTC(),
		sourceType,
	)

	if result.Error != nil {
		return 0, result.Error
	}

	return int(result.RowsAffected), nil
}
