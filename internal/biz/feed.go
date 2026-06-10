package biz

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/4itosik/feedium/internal/conf"
)

//go:generate mockgen -source=feed.go -destination=mock/feed_mock.go -package=mock

// FeedItem is a post paired with its current relevance score (CTR-04).
type FeedItem struct {
	Post     Post
	Score    float64
	URL      string // best-effort link to the original source material; empty when unavailable
	Reaction string // "like", "dislike", or "" if no reaction
}

// FeedQuery filters and paginates the ranked feed (REQ-04).
type FeedQuery struct {
	From      *time.Time
	To        *time.Time
	MinScore  float64
	PageSize  int
	PageToken string
}

// FeedResult holds a page of ranked feed items, ordered by score descending.
type FeedResult struct {
	Items         []FeedItem
	NextPageToken string
}

// ErrInvalidThreshold is returned when the min-score filter is outside [0,1].
var ErrInvalidThreshold = errors.New("invalid score threshold")

// FeedQueryRepo serves the ranked feed read model (posts ⋈ scores).
type FeedQueryRepo interface {
	ListRanked(ctx context.Context, query FeedQuery) (FeedResult, error)
}

// FeedUsecase serves the ranked feed (REQ-04).
type FeedUsecase struct {
	repo FeedQueryRepo
}

// NewFeedUsecase creates a new FeedUsecase. The cfg argument is accepted for
// symmetry with other scoring usecases and to keep the default-threshold owner
// (conf.Scoring, DEC-07) in one place even though RankedFeed reads the threshold
// from the request.
func NewFeedUsecase(repo FeedQueryRepo, _ *conf.Scoring) *FeedUsecase {
	return &FeedUsecase{repo: repo}
}

// RankedFeed validates the threshold, clamps pagination and returns the ranked
// feed ordered by score descending (F3).
func (uc *FeedUsecase) RankedFeed(ctx context.Context, query FeedQuery) (FeedResult, error) {
	if query.MinScore < 0 || query.MinScore > 1 {
		return FeedResult{}, fmt.Errorf("%w: min_score must be in [0,1]", ErrInvalidThreshold)
	}
	if query.From != nil && query.To != nil && query.From.After(*query.To) {
		return FeedResult{}, fmt.Errorf("%w: from must be before to", ErrInvalidThreshold)
	}
	if query.PageSize < minPageSize {
		query.PageSize = minPageSize
	}
	if query.PageSize > maxPageSize {
		query.PageSize = maxPageSize
	}
	return uc.repo.ListRanked(ctx, query)
}
