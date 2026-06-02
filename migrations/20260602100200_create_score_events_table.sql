-- +goose Up
CREATE TABLE score_events (
    id UUID PRIMARY KEY,
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    error TEXT NULL,
    attempt_count INT NOT NULL DEFAULT 0,
    locked_by TEXT NULL,
    locked_until TIMESTAMPTZ NULL,
    next_attempt_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ NULL
);

-- At most one active scoring event per post (INV-04): enqueue is idempotent via
-- INSERT ... ON CONFLICT DO NOTHING against this partial unique index.
CREATE UNIQUE INDEX idx_score_events_unique_active_post
    ON score_events(post_id) WHERE status IN ('pending', 'processing');
-- Claim ordering: pending/due first, then expired-lease processing rows.
CREATE INDEX idx_score_events_claim
    ON score_events(status, COALESCE(next_attempt_at, created_at));
CREATE INDEX idx_score_events_processing_lease
    ON score_events(locked_until) WHERE status = 'processing';

-- +goose Down
DROP TABLE IF EXISTS score_events;
