package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
)

type postEmbeddingRepo struct {
	data *Data
}

var _ biz.PostEmbeddingRepo = (*postEmbeddingRepo)(nil)

// NewPostEmbeddingRepo creates the persisted post-embedding repository (CTR-01).
//
//nolint:revive // unexported return type for wire DI
func NewPostEmbeddingRepo(data *Data) *postEmbeddingRepo {
	return &postEmbeddingRepo{data: data}
}

func (r *postEmbeddingRepo) Get(ctx context.Context, postID string) (biz.PostEmbedding, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)
	pid, err := uuid.Parse(postID)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("invalid post id: %w", err)
	}
	const query = `
SELECT post_id, vector, model, input_hash, created_at, updated_at
FROM post_embeddings WHERE post_id = $1`
	var (
		outPostID  uuid.UUID
		vectorJSON []byte
		model      string
		inputHash  string
		createdAt  time.Time
		updatedAt  time.Time
	)
	err = ex.QueryRowContext(ctx, query, pid).
		Scan(&outPostID, &vectorJSON, &model, &inputHash, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return biz.PostEmbedding{}, biz.ErrPostEmbeddingNotFound
	}
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("get post embedding: %w", err)
	}
	var vector []float32
	if uerr := json.Unmarshal(vectorJSON, &vector); uerr != nil {
		return biz.PostEmbedding{}, fmt.Errorf("decode vector: %w", uerr)
	}
	return biz.PostEmbedding{
		PostID: outPostID.String(), Vector: vector, Model: model,
		InputHash: inputHash, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}

func (r *postEmbeddingRepo) Upsert(ctx context.Context, pe biz.PostEmbedding) (biz.PostEmbedding, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)
	pid, err := uuid.Parse(pe.PostID)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("invalid post id: %w", err)
	}
	vectorJSON, err := json.Marshal(pe.Vector)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("encode vector: %w", err)
	}
	const query = `
INSERT INTO post_embeddings (id, post_id, vector, model, input_hash, created_at, updated_at)
VALUES ($1, $2, $3::jsonb, $4, $5, now(), now())
ON CONFLICT (post_id) DO UPDATE
SET vector = EXCLUDED.vector, model = EXCLUDED.model,
    input_hash = EXCLUDED.input_hash, updated_at = now()
RETURNING post_id, vector, model, input_hash, created_at, updated_at`
	var (
		outPostID uuid.UUID
		outVector []byte
		model     string
		inputHash string
		createdAt time.Time
		updatedAt time.Time
	)
	// newID is only used on INSERT; on ON CONFLICT DO UPDATE it is ignored (id is
	// not in the SET clause).
	newID := uuid.Must(uuid.NewV7())
	err = ex.QueryRowContext(ctx, query,
		newID, pid, string(vectorJSON), pe.Model, pe.InputHash).
		Scan(&outPostID, &outVector, &model, &inputHash, &createdAt, &updatedAt)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("upsert post embedding: %w", err)
	}
	var vector []float32
	if uerr := json.Unmarshal(outVector, &vector); uerr != nil {
		return biz.PostEmbedding{}, fmt.Errorf("decode vector: %w", uerr)
	}
	return biz.PostEmbedding{
		PostID: outPostID.String(), Vector: vector, Model: model,
		InputHash: inputHash, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}
