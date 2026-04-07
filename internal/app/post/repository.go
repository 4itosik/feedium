package post

import (
	"context"

	"github.com/google/uuid"
)

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

type Repository interface {
	Create(context.Context, *Post) error
	CreateWithOutbox(ctx context.Context, post *Post, createEventFn func(postID uuid.UUID) (any, error)) error
	GetByID(context.Context, uuid.UUID) (*Post, error)
	Update(context.Context, *Post) error
	Delete(context.Context, uuid.UUID) error
	List(context.Context, ListFilter) ([]Post, int, error)
}
