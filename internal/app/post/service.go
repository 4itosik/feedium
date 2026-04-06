package post

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repo           Repository
	log            *slog.Logger
	outboxBuilder  func(*Post) (any, error)
}

const (
	defaultPageSize = 50
	maxPageSize     = 100
)

func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// SetOutboxBuilder registers a callback for creating outbox events on post creation/update.
func (s *Service) SetOutboxBuilder(fn func(*Post) (any, error)) {
	s.outboxBuilder = fn
}

func (s *Service) Create(ctx context.Context, post *Post) (*Post, error) {
	if err := validatePost(post); err != nil {
		return nil, err
	}

	if s.outboxBuilder != nil {
		// Use CreateWithOutbox if outboxBuilder is registered
		if err := s.repo.CreateWithOutbox(ctx, post, func(postID uuid.UUID) (interface{}, error) {
			// Create a copy of the post with the ID for the builder
			postCopy := *post
			postCopy.ID = postID
			return s.outboxBuilder(&postCopy)
		}); err != nil {
			return nil, err
		}
	} else {
		// Use regular Create if no outboxBuilder
		if err := s.repo.Create(ctx, post); err != nil {
			return nil, err
		}
	}

	return post, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Post, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, post *Post) (*Post, error) {
	if err := validatePost(post); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByID(ctx, post.ID); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, post); err != nil {
		return nil, err
	}
	return post, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Post, int, error) {
	filter.PageSize = normalizePageSize(filter.PageSize)
	filter.Page = normalizePage(filter.Page)
	return s.repo.List(ctx, filter)
}

func normalizePageSize(v int) int {
	if v <= 0 {
		return defaultPageSize
	}
	if v > maxPageSize {
		return maxPageSize
	}
	return v
}

func normalizePage(v int) int {
	if v <= 0 {
		return 1
	}
	return v
}

func validatePost(p *Post) error {
	if p == nil {
		return validationError("post is required")
	}
	if p.SourceID == uuid.Nil {
		return validationError("source_id is required")
	}
	p.Title = strings.TrimSpace(p.Title)
	p.Content = strings.TrimSpace(p.Content)
	if p.Title == "" {
		return validationError("title is required")
	}
	if p.Content == "" {
		return validationError("content is required")
	}
	if p.PublishedAt.IsZero() {
		return validationError("published_at is required")
	}
	return nil
}
