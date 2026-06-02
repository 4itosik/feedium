-- +goose Up
CREATE TABLE scores (
    id UUID PRIMARY KEY,
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    value DOUBLE PRECISION NOT NULL,
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Exactly one current score per post (INV-02, CTR-02): upsert target.
CREATE UNIQUE INDEX idx_scores_post_id ON scores(post_id);
-- Ranked feed reads value descending (REQ-04).
CREATE INDEX idx_scores_value ON scores(value DESC);

-- +goose Down
DROP TABLE IF EXISTS scores;
