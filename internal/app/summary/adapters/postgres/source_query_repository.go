package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"feedium/internal/app/source"
	"feedium/internal/app/summary"
)

type SourceQueryRepository struct {
	db *gorm.DB
}

func NewSourceQueryRepository(db *gorm.DB) summary.SourceQueryRepository {
	return &SourceQueryRepository{db: db}
}

// GetByID retrieves a source by its ID.
func (r *SourceQueryRepository) GetByID(ctx context.Context, id uuid.UUID) (*source.Source, error) {
	var s source.Source
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&s)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, summary.ErrSourceNotFound
		}
		return nil, result.Error
	}
	return &s, nil
}
