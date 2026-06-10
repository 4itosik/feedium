package data

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
)

type reactionRepo struct {
	data *Data
}

var _ biz.ReactionRepo = (*reactionRepo)(nil)

// NewReactionRepo creates a new reaction repository.
//
//nolint:revive // unexported return type for wire DI
func NewReactionRepo(data *Data) *reactionRepo {
	return &reactionRepo{data: data}
}

// Upsert inserts or replaces the reaction signal for a post (last-write-wins).
func (r *reactionRepo) Upsert(ctx context.Context, postID, signal string) error {
	if _, err := uuid.Parse(postID); err != nil {
		return fmt.Errorf("invalid post id: %w", err)
	}
	const query = `
INSERT INTO post_reactions (post_id, signal, created_at, updated_at)
VALUES ($1, $2, now(), now())
ON CONFLICT (post_id) DO UPDATE
SET signal = EXCLUDED.signal, updated_at = now()
`
	if _, err := r.data.DB.ExecContext(ctx, query, postID, signal); err != nil {
		return fmt.Errorf("upsert reaction: %w", err)
	}
	return nil
}
