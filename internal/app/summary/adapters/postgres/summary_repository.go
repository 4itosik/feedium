package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"feedium/internal/app/summary"
)

type SummaryRepository struct {
	db *gorm.DB
}

func NewSummaryRepository(db *gorm.DB) summary.SummaryRepository {
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
		summaryPost := map[string]interface{}{
			"summary_id": s.ID,
			"post_id":    postID,
		}
		if err := tx.Table("summary_posts").Create(summaryPost).Error; err != nil {
			return err
		}
	}

	return tx.Commit().Error
}
