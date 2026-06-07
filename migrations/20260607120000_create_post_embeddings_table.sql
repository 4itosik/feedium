-- +goose Up
CREATE TABLE post_embeddings (
    id UUID PRIMARY KEY,
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    vector JSONB NOT NULL,
    model TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Exactly one persisted embedding per post (CTR-01): upsert target.
CREATE UNIQUE INDEX idx_post_embeddings_post_id ON post_embeddings(post_id);

-- +goose Down
DROP TABLE IF EXISTS post_embeddings;
