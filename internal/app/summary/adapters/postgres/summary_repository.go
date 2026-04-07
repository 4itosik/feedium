package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"feedium/internal/app/summary"
)

type SummaryRepository struct {
	db *gorm.DB
}

func NewSummaryRepository(db *gorm.DB) summary.Repository {
	return &SummaryRepository{db: db}
}

// Create creates a new summary and associates it with the given posts in a single transaction.
func (r *SummaryRepository) Create(ctx context.Context, s *summary.Summary, postIDs []uuid.UUID) error {
	tx := r.db.WithContext(ctx).Begin()
	defer tx.Rollback()

	// Insert the summary
	if err := tx.Create(s).Error; err != nil {
		return err
	}

	// Insert summary_posts associations for each post
	for _, postID := range postIDs {
		summaryPost := map[string]any{
			"summary_id": s.ID,
			"post_id":    postID,
		}
		if err := tx.Table("summary_posts").Create(summaryPost).Error; err != nil {
			return err
		}
	}

	return tx.Commit().Error
}

// GetByPostID retrieves a summary by its associated post ID using a JOIN with summary_posts.
// Returns the summary and all associated post IDs.
// Returns nil, nil if not found.
func (r *SummaryRepository) GetByPostID(ctx context.Context, postID uuid.UUID) (*summary.Summary, []uuid.UUID, error) {
	// First, get the summary ID from summary_posts
	var summaryID uuid.UUID
	findErr := r.db.WithContext(ctx).
		Table("summary_posts").
		Select("summary_id").
		Where("post_id = ?", postID).
		Scan(&summaryID).Error

	if errors.Is(findErr, sql.ErrNoRows) || findErr == nil && summaryID == uuid.Nil {
		return nil, nil, nil
	}
	if findErr != nil {
		return nil, nil, fmt.Errorf("failed to find summary by post: %w", findErr)
	}

	// Get the summary
	var s summary.Summary
	getErr := r.db.WithContext(ctx).
		Where("id = ?", summaryID).
		First(&s).Error
	if getErr != nil {
		if errors.Is(getErr, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to get summary: %w", getErr)
	}

	// Get all post IDs associated with this summary
	var postIDs []uuid.UUID
	pluckErr := r.db.WithContext(ctx).
		Table("summary_posts").
		Select("post_id").
		Where("summary_id = ?", summaryID).
		Pluck("post_id", &postIDs).Error
	if pluckErr != nil {
		return nil, nil, fmt.Errorf("failed to get post IDs: %w", pluckErr)
	}

	return &s, postIDs, nil
}

// ListSummaries retrieves summaries with their post IDs using aggregation to avoid N+1 queries.
// If sourceID is nil, returns summaries from all sources.
// Results are ordered by created_at DESC.
func (r *SummaryRepository) ListSummaries(
	ctx context.Context,
	sourceID *uuid.UUID,
	limit int,
) ([]summary.WithPostIDs, error) {
	type row struct {
		ID        uuid.UUID `gorm:"column:id"`
		SourceID  uuid.UUID `gorm:"column:source_id"`
		EventID   uuid.UUID `gorm:"column:event_id"`
		Content   string    `gorm:"column:content"`
		CreatedAt time.Time `gorm:"column:created_at"`
		PostIDs   string    `gorm:"column:post_ids"`
	}

	query := `
		SELECT s.id, s.source_id, s.event_id, s.content, s.created_at,
			array_to_string(array_agg(sp.post_id), ',') as post_ids
		FROM summaries s
		LEFT JOIN summary_posts sp ON s.id = sp.summary_id
		WHERE 1=1
	`
	var args []any

	if sourceID != nil {
		query += ` AND s.source_id = ?`
		args = append(args, *sourceID)
	}

	query += `
		GROUP BY s.id, s.source_id, s.event_id, s.content, s.created_at
		ORDER BY s.created_at DESC
		LIMIT ?
	`
	args = append(args, limit)

	var rows []row
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list summaries: %w", err)
	}

	result := make([]summary.WithPostIDs, 0, len(rows))
	for _, r := range rows {
		s := summary.WithPostIDs{
			Summary: summary.Summary{
				ID:        r.ID,
				SourceID:  r.SourceID,
				EventID:   r.EventID,
				Content:   r.Content,
				CreatedAt: r.CreatedAt,
			},
			PostIDs: parseUUIDList(r.PostIDs),
		}
		result = append(result, s)
	}

	return result, nil
}

func parseUUIDList(s string) []uuid.UUID {
	if s == "" {
		return []uuid.UUID{}
	}
	parts := strings.Split(s, ",")
	uuids := make([]uuid.UUID, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		id, err := uuid.Parse(p)
		if err != nil {
			continue
		}
		uuids = append(uuids, id)
	}
	return uuids
}
