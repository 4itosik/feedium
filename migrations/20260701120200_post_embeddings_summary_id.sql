-- +goose Up
-- FT-020 (ADR-003, CTR-01, REQ-03): digest embeddings are persisted and reused between
-- re-scorings keyed by summary_id, mirroring the post embedding cache (ADR-002 pattern).
ALTER TABLE post_embeddings ALTER COLUMN post_id DROP NOT NULL;
ALTER TABLE post_embeddings ADD COLUMN summary_id UUID REFERENCES summaries(id) ON DELETE CASCADE;
ALTER TABLE post_embeddings ADD CONSTRAINT post_embeddings_post_or_summary
    CHECK (num_nonnulls(post_id, summary_id) = 1);

DROP INDEX idx_post_embeddings_post_id;
CREATE UNIQUE INDEX idx_post_embeddings_post_id ON post_embeddings(post_id) WHERE post_id IS NOT NULL;
CREATE UNIQUE INDEX idx_post_embeddings_summary_id ON post_embeddings(summary_id) WHERE summary_id IS NOT NULL;

-- +goose Down
DELETE FROM post_embeddings WHERE post_id IS NULL;
DROP INDEX idx_post_embeddings_summary_id;
DROP INDEX idx_post_embeddings_post_id;
ALTER TABLE post_embeddings DROP CONSTRAINT post_embeddings_post_or_summary;
ALTER TABLE post_embeddings DROP COLUMN summary_id;
ALTER TABLE post_embeddings ALTER COLUMN post_id SET NOT NULL;
CREATE UNIQUE INDEX idx_post_embeddings_post_id ON post_embeddings(post_id);
