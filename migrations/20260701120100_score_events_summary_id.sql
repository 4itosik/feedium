-- +goose Up
-- FT-020 (ADR-003, CTR-01/CTR-04): score_events can target a digest (summary_id) as well as a
-- post. post_id nullable; exactly one of (post_id, summary_id). The post active-event partial
-- unique index is rebuilt as post-scoped, plus a mirror for summary_id so EnqueueDueDigests /
-- EnqueueScoredDigests stay idempotent via INSERT ... ON CONFLICT DO NOTHING (INV-04).
ALTER TABLE score_events ALTER COLUMN post_id DROP NOT NULL;
ALTER TABLE score_events ADD COLUMN summary_id UUID REFERENCES summaries(id) ON DELETE CASCADE;
ALTER TABLE score_events ADD CONSTRAINT score_events_post_or_summary
    CHECK (num_nonnulls(post_id, summary_id) = 1);

DROP INDEX idx_score_events_unique_active_post;
CREATE UNIQUE INDEX idx_score_events_unique_active_post
    ON score_events(post_id) WHERE post_id IS NOT NULL AND status IN ('pending', 'processing');
CREATE UNIQUE INDEX idx_score_events_unique_active_summary
    ON score_events(summary_id) WHERE summary_id IS NOT NULL AND status IN ('pending', 'processing');

-- +goose Down
DELETE FROM score_events WHERE post_id IS NULL;
DROP INDEX idx_score_events_unique_active_summary;
DROP INDEX idx_score_events_unique_active_post;
ALTER TABLE score_events DROP CONSTRAINT score_events_post_or_summary;
ALTER TABLE score_events DROP COLUMN summary_id;
ALTER TABLE score_events ALTER COLUMN post_id SET NOT NULL;
CREATE UNIQUE INDEX idx_score_events_unique_active_post
    ON score_events(post_id) WHERE status IN ('pending', 'processing');
