package source

import (
	"context"

	"github.com/google/uuid"
)

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks

type Repository interface {
	Create(context.Context, *Source) error
	GetByID(context.Context, uuid.UUID) (*Source, error)
	Update(context.Context, *Source) error
	Delete(context.Context, uuid.UUID) error
	List(context.Context, ListFilter) ([]Source, int, error)
}
