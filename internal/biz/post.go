package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SourceInfo struct {
	ID   string
	Type SourceType
}

type Post struct {
	ID          string
	SourceID    string
	Source      SourceInfo
	ExternalID  string
	PublishedAt time.Time
	Author      *string
	Text        string
	Metadata    map[string]string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SortField int

const (
	SortByPublishedAt SortField = iota + 1
	SortByCreatedAt
)

type SortDirection int

const (
	SortDesc SortDirection = iota + 1
	SortAsc
)

type ListPostsFilter struct {
	SourceID  string
	PageSize  int
	PageToken string
	OrderBy   SortField
	OrderDir  SortDirection
}

type ListPostsResult struct {
	Items         []Post
	NextPageToken string
}

var (
	ErrPostNotFound        = errors.New("post not found")
	ErrPostInvalidArgument = errors.New("post invalid argument")
	ErrPostSourceNotFound  = errors.New("post source not found")
	ErrPostAlreadyExists   = errors.New("post already exists")
)

func ValidateCreatePost(sourceID, externalID, text string, publishedAt time.Time) error {
	var invalid []string

	if _, err := uuid.Parse(sourceID); err != nil {
		invalid = append(invalid, "source_id")
	}
	if externalID == "" {
		invalid = append(invalid, "external_id")
	}
	if strings.TrimSpace(text) == "" {
		invalid = append(invalid, "text")
	}
	if publishedAt.IsZero() {
		invalid = append(invalid, "published_at")
	}

	if len(invalid) > 0 {
		return fmt.Errorf("%w: %s", ErrPostInvalidArgument, strings.Join(invalid, ", "))
	}
	return nil
}

func ValidateUpdatePost(externalID, text string, publishedAt time.Time) error {
	var invalid []string

	if externalID == "" {
		invalid = append(invalid, "external_id")
	}
	if strings.TrimSpace(text) == "" {
		invalid = append(invalid, "text")
	}
	if publishedAt.IsZero() {
		invalid = append(invalid, "published_at")
	}

	if len(invalid) > 0 {
		return fmt.Errorf("%w: %s", ErrPostInvalidArgument, strings.Join(invalid, ", "))
	}
	return nil
}

func ValidateListPostsFilter(filter ListPostsFilter) error {
	if filter.OrderBy != SortByPublishedAt && filter.OrderBy != SortByCreatedAt {
		return fmt.Errorf("%w: invalid order_by", ErrPostInvalidArgument)
	}
	if filter.OrderDir != SortDesc && filter.OrderDir != SortAsc {
		return fmt.Errorf("%w: invalid order_dir", ErrPostInvalidArgument)
	}
	if filter.SourceID != "" {
		if _, err := uuid.Parse(filter.SourceID); err != nil {
			return fmt.Errorf("%w: invalid source_id", ErrPostInvalidArgument)
		}
	}
	return nil
}

type PostRepo interface {
	Save(ctx context.Context, post Post) (Post, error)
	Update(ctx context.Context, post Post) (Post, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (Post, error)
	List(ctx context.Context, filter ListPostsFilter) (ListPostsResult, error)
}

type PostUsecase struct {
	repo PostRepo
}

func NewPostUsecase(repo PostRepo) *PostUsecase {
	return &PostUsecase{repo: repo}
}

func (uc *PostUsecase) Create(
	ctx context.Context,
	sourceID, externalID, text string,
	publishedAt time.Time,
	author *string,
	metadata map[string]string,
) (Post, error) {
	if err := ValidateCreatePost(sourceID, externalID, text, publishedAt); err != nil {
		return Post{}, err
	}

	if metadata == nil {
		metadata = map[string]string{}
	}

	now := time.Now()
	post := Post{
		ID:          uuid.Must(uuid.NewV7()).String(),
		SourceID:    sourceID,
		ExternalID:  externalID,
		PublishedAt: publishedAt,
		Author:      author,
		Text:        text,
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return uc.repo.Save(ctx, post)
}

func (uc *PostUsecase) Update(
	ctx context.Context,
	id, externalID, text string,
	publishedAt time.Time,
	author *string,
	metadata map[string]string,
) (Post, error) {
	if err := ValidateUpdatePost(externalID, text, publishedAt); err != nil {
		return Post{}, err
	}

	existing, err := uc.repo.Get(ctx, id)
	if err != nil {
		return Post{}, err
	}

	if metadata == nil {
		metadata = map[string]string{}
	}

	existing.ExternalID = externalID
	existing.PublishedAt = publishedAt
	existing.Author = author
	existing.Text = text
	existing.Metadata = metadata
	existing.UpdatedAt = time.Now()

	return uc.repo.Update(ctx, existing)
}

func (uc *PostUsecase) Delete(ctx context.Context, id string) error {
	return uc.repo.Delete(ctx, id)
}

func (uc *PostUsecase) Get(ctx context.Context, id string) (Post, error) {
	return uc.repo.Get(ctx, id)
}

func (uc *PostUsecase) List(ctx context.Context, filter ListPostsFilter) (ListPostsResult, error) {
	if filter.PageSize < minPageSize {
		filter.PageSize = minPageSize
	}
	if filter.PageSize > maxPageSize {
		filter.PageSize = maxPageSize
	}

	if err := ValidateListPostsFilter(filter); err != nil {
		return ListPostsResult{}, err
	}

	return uc.repo.List(ctx, filter)
}
