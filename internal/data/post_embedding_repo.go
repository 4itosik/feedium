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

// Get returns the persisted embedding for the entity identified by key (post_id or
// summary_id — exactly one set), or ErrPostEmbeddingNotFound (FT-020 CTR-01).
func (r *postEmbeddingRepo) Get(ctx context.Context, key biz.ScoreKey) (biz.PostEmbedding, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	column, idStr, err := embeddingKeyColumn(key)
	if err != nil {
		return biz.PostEmbedding{}, err
	}
	kid, err := uuid.Parse(idStr)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("invalid %s: %w", column, err)
	}
	query := `
SELECT post_id, summary_id, vector, model, input_hash, created_at, updated_at
FROM post_embeddings WHERE ` + column + ` = $1`
	var (
		outPostID  uuid.NullUUID
		outSumID   uuid.NullUUID
		vectorJSON []byte
		model      string
		inputHash  string
		createdAt  time.Time
		updatedAt  time.Time
	)
	err = ex.QueryRowContext(ctx, query, kid).
		Scan(&outPostID, &outSumID, &vectorJSON, &model, &inputHash, &createdAt, &updatedAt)
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
	pe := biz.PostEmbedding{
		Vector: vector, Model: model,
		InputHash: inputHash, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	if outPostID.Valid {
		s := outPostID.UUID.String()
		pe.PostID = &s
	}
	if outSumID.Valid {
		s := outSumID.UUID.String()
		pe.SummaryID = &s
	}
	return pe, nil
}

// embeddingKeyColumn resolves which key column (post_id / summary_id) a ScoreKey
// targets, returning the column name and the id. Exactly one must be set (FT-020 CTR-01).
func embeddingKeyColumn(key biz.ScoreKey) (string, string, error) {
	switch {
	case key.PostID != nil:
		return "post_id", *key.PostID, nil
	case key.SummaryID != nil:
		return "summary_id", *key.SummaryID, nil
	default:
		return "", "", errors.New("score key has neither post_id nor summary_id")
	}
}

// embeddingUpsertPost persists a post embedding, matching the partial unique index
// idx_post_embeddings_post_id (WHERE post_id IS NOT NULL).
const embeddingUpsertPost = `
INSERT INTO post_embeddings (id, post_id, vector, model, input_hash, created_at, updated_at)
VALUES ($1, $2, $3::jsonb, $4, $5, now(), now())
ON CONFLICT (post_id) WHERE post_id IS NOT NULL DO UPDATE
SET vector = EXCLUDED.vector, model = EXCLUDED.model,
    input_hash = EXCLUDED.input_hash, updated_at = now()
RETURNING post_id, summary_id, vector, model, input_hash, created_at, updated_at`

// embeddingUpsertDigest persists a digest embedding, matching the partial unique index
// idx_post_embeddings_summary_id (WHERE summary_id IS NOT NULL).
const embeddingUpsertDigest = `
INSERT INTO post_embeddings (id, summary_id, vector, model, input_hash, created_at, updated_at)
VALUES ($1, $2, $3::jsonb, $4, $5, now(), now())
ON CONFLICT (summary_id) WHERE summary_id IS NOT NULL DO UPDATE
SET vector = EXCLUDED.vector, model = EXCLUDED.model,
    input_hash = EXCLUDED.input_hash, updated_at = now()
RETURNING post_id, summary_id, vector, model, input_hash, created_at, updated_at`

// Upsert persists the single embedding for a post or a digest in place (FT-020
// CTR-01). Exactly one of PostID / SummaryID is set on the PostEmbedding.
func (r *postEmbeddingRepo) Upsert(
	ctx context.Context,
	pe biz.PostEmbedding,
) (biz.PostEmbedding, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	column, idStr, err := embeddingKeyColumn(
		biz.ScoreKey{PostID: pe.PostID, SummaryID: pe.SummaryID},
	)
	if err != nil {
		return biz.PostEmbedding{}, err
	}
	kid, err := uuid.Parse(idStr)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("invalid %s: %w", column, err)
	}
	vectorJSON, err := json.Marshal(pe.Vector)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("encode vector: %w", err)
	}
	query := embeddingUpsertPost
	if pe.SummaryID != nil {
		query = embeddingUpsertDigest
	}
	var (
		outPostID uuid.NullUUID
		outSumID  uuid.NullUUID
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
		newID, kid, string(vectorJSON), pe.Model, pe.InputHash).
		Scan(&outPostID, &outSumID, &outVector, &model, &inputHash, &createdAt, &updatedAt)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("upsert post embedding: %w", err)
	}
	var vector []float32
	if uerr := json.Unmarshal(outVector, &vector); uerr != nil {
		return biz.PostEmbedding{}, fmt.Errorf("decode vector: %w", uerr)
	}
	out := biz.PostEmbedding{
		Vector: vector, Model: model,
		InputHash: inputHash, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
	if outPostID.Valid {
		s := outPostID.UUID.String()
		out.PostID = &s
	}
	if outSumID.Valid {
		s := outSumID.UUID.String()
		out.SummaryID = &s
	}
	return out, nil
}
