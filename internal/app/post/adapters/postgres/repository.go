package postgres

import (
	"context"
	"errors"
	"time"

	"feedium/internal/app/post"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, p *post.Post) error {
	row := fromDomain(p)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return mapError(err)
	}
	applyRow(p, &row)
	return nil
}

// CreateWithOutbox creates a post and an outbox event in a single transaction.
// createEventFn is a callback that receives the post ID after insertion.
// If createEventFn returns nil, nil, no event is created.
func (r *Repository) CreateWithOutbox(ctx context.Context, post *post.Post, createEventFn func(postID uuid.UUID) (interface{}, error)) error {
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// Create the post
	row := fromDomain(post)
	if err := tx.Create(&row).Error; err != nil {
		tx.Rollback()
		return mapError(err)
	}
	applyRow(post, &row)

	// Call the callback to create the event (if needed)
	if createEventFn != nil {
		event, err := createEventFn(post.ID)
		if err != nil {
			tx.Rollback()
			return err
		}

		// Create the event if it's not nil
		if event != nil {
			if err := tx.Create(event).Error; err != nil {
				tx.Rollback()
				return err
			}
		}
	}

	return tx.Commit().Error
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*post.Post, error) {
	var row postRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, post.ErrNotFound
		}
		return nil, err
	}
	out := toDomain(&row)
	return &out, nil
}

func (r *Repository) Update(ctx context.Context, p *post.Post) error {
	row := fromDomain(p)
	res := r.db.WithContext(ctx).Model(&postRow{}).Where("id = ?", p.ID).Updates(map[string]any{
		"source_id":    row.SourceID,
		"title":        row.Title,
		"content":      row.Content,
		"author":       row.Author,
		"published_at": row.PublishedAt,
		"updated_at":   time.Now().UTC(),
	})
	if res.Error != nil {
		return mapError(res.Error)
	}
	if res.RowsAffected == 0 {
		return post.ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&postRow{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return post.ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter post.ListFilter) ([]post.Post, int, error) {
	var rows []postRow
	q := r.db.WithContext(ctx).Model(&postRow{})

	if filter.PublishedAfter != nil {
		q = q.Where("published_at >= ?", *filter.PublishedAfter)
	}
	if filter.PublishedBefore != nil {
		q = q.Where("published_at < ?", *filter.PublishedBefore)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	if err := q.Order("published_at desc").Limit(filter.PageSize).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	out := make([]post.Post, 0, len(rows))
	for i := range rows {
		p := toDomain(&rows[i])
		out = append(out, p)
	}
	return out, int(total), nil
}

type postRow struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	SourceID    uuid.UUID `gorm:"type:uuid;not null"`
	Title       string    `gorm:"type:text;not null"`
	Content     string    `gorm:"type:text;not null"`
	Author      string    `gorm:"type:varchar(255);not null;default:''"`
	PublishedAt time.Time `gorm:"type:timestamptz;not null"`
	CreatedAt   time.Time `gorm:"type:timestamptz;not null;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"type:timestamptz;not null;autoUpdateTime"`
}

func (postRow) TableName() string {
	return "posts"
}

func fromDomain(p *post.Post) postRow {
	return postRow{
		ID:          p.ID,
		SourceID:    p.SourceID,
		Title:       p.Title,
		Content:     p.Content,
		Author:      p.Author,
		PublishedAt: p.PublishedAt,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func toDomain(row *postRow) post.Post {
	return post.Post{
		ID:          row.ID,
		SourceID:    row.SourceID,
		Title:       row.Title,
		Content:     row.Content,
		Author:      row.Author,
		PublishedAt: row.PublishedAt,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func applyRow(dst *post.Post, row *postRow) {
	dst.ID, dst.CreatedAt, dst.UpdatedAt = row.ID, row.CreatedAt, row.UpdatedAt
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	// Check for FK violation (PostgreSQL error code 23503)
	// GORM doesn't provide a clean way to check for specific PG errors,
	// so we check for the error message containing "violates foreign key constraint"
	if isFKViolation(err) {
		return post.NewValidationError("invalid source_id: source not found")
	}
	return err
}

func isFKViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "violates foreign key constraint") ||
		contains(errStr, "23503")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
