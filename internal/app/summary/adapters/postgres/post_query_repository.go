package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"feedium/internal/app/post"
	"feedium/internal/app/summary"
)

type PostQueryRepository struct {
	db *gorm.DB
}

func NewPostQueryRepository(db *gorm.DB) summary.PostQueryRepository {
	return &PostQueryRepository{db: db}
}

// GetByID retrieves a post by its ID.
func (r *PostQueryRepository) GetByID(ctx context.Context, id uuid.UUID) (*post.Post, error) {
	var p post.Post
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&p)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, summary.ErrPostNotFound
		}
		return nil, result.Error
	}
	return &p, nil
}

// FindUnprocessedBySource finds all unprocessed posts for a source since a given time.
// A post is considered unprocessed if it has no entry in summary_posts.
func (r *PostQueryRepository) FindUnprocessedBySource(
	ctx context.Context,
	sourceID uuid.UUID,
	since time.Time,
) ([]post.Post, error) {
	var posts []post.Post

	const query = `
		SELECT p.id, p.source_id, p.title, p.content, p.author, p.published_at, p.created_at, p.updated_at
		FROM posts p
		WHERE p.source_id = $1
		  AND p.created_at >= $2
		  AND NOT EXISTS (
		    SELECT 1 FROM summary_posts sp WHERE sp.post_id = p.id
		  )
		ORDER BY p.created_at ASC
	`

	result := r.db.WithContext(ctx).Raw(query, sourceID, since).Scan(&posts)
	if result.Error != nil {
		return nil, result.Error
	}

	return posts, nil
}
