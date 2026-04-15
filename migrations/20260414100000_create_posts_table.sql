-- +goose Up
CREATE TABLE posts (
    id UUID PRIMARY KEY,
    source_id UUID NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    external_id TEXT NOT NULL,
    published_at TIMESTAMPTZ NOT NULL,
    author TEXT NULL,
    text TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_id, external_id)
);

CREATE INDEX idx_posts_source_published_id ON posts(source_id, published_at DESC, id DESC);
CREATE INDEX idx_posts_source_created_id ON posts(source_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS posts;
