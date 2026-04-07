package post

import (
	"time"

	"github.com/google/uuid"
)

type Post struct {
	ID          uuid.UUID
	SourceID    uuid.UUID
	Title       string
	Content     string
	Author      string
	PublishedAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ListFilter struct {
	PublishedAfter  *time.Time
	PublishedBefore *time.Time
	Page            int
	PageSize        int
}
