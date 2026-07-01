-- +goose Up
-- FT-020 (ADR-003, CTR-01): cumulative digests (summaries.post_id IS NULL) become
-- independently scoreable, keyed by summary_id. post_id becomes nullable; exactly one
-- of (post_id, summary_id) is set per row; the single UNIQUE(post_id) is replaced by two
-- partial unique indexes so each entity keeps "exactly one score" (INV-02, symmetric REQ-01).
ALTER TABLE scores ALTER COLUMN post_id DROP NOT NULL;
ALTER TABLE scores ADD COLUMN summary_id UUID REFERENCES summaries(id) ON DELETE CASCADE;
ALTER TABLE scores ADD CONSTRAINT scores_post_or_summary
    CHECK (num_nonnulls(post_id, summary_id) = 1);

DROP INDEX idx_scores_post_id;
CREATE UNIQUE INDEX idx_scores_post_id ON scores(post_id) WHERE post_id IS NOT NULL;
CREATE UNIQUE INDEX idx_scores_summary_id ON scores(summary_id) WHERE summary_id IS NOT NULL;

-- +goose Down
-- Reversible to pre-FT-020 (posts-only). Digest-scored rows (post_id IS NULL) cannot satisfy
-- post_id NOT NULL, so they are dropped first (OQ-03): the safe fallback is "only posts scored".
DELETE FROM scores WHERE post_id IS NULL;
DROP INDEX idx_scores_summary_id;
DROP INDEX idx_scores_post_id;
ALTER TABLE scores DROP CONSTRAINT scores_post_or_summary;
ALTER TABLE scores DROP COLUMN summary_id;
ALTER TABLE scores ALTER COLUMN post_id SET NOT NULL;
CREATE UNIQUE INDEX idx_scores_post_id ON scores(post_id);
