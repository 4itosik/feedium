package biz

import (
	"context"
	"errors"
)

// ErrInvalidSignal is returned when the reaction signal is not "like" or "dislike".
var ErrInvalidSignal = errors.New("invalid reaction signal")

// ReactionRepo persists post reactions.
type ReactionRepo interface {
	Upsert(ctx context.Context, postID, signal string) error
}

// ReactionUsecase handles like/dislike signals on posts.
type ReactionUsecase struct {
	repo ReactionRepo
}

// NewReactionUsecase creates a new ReactionUsecase.
func NewReactionUsecase(repo ReactionRepo) *ReactionUsecase {
	return &ReactionUsecase{repo: repo}
}

// React stores a like or dislike for the given post. Calling React again on
// the same post replaces the previous signal (last-write-wins).
func (uc *ReactionUsecase) React(ctx context.Context, postID, signal string) error {
	if signal != "like" && signal != "dislike" {
		return ErrInvalidSignal
	}
	return uc.repo.Upsert(ctx, postID, signal)
}
