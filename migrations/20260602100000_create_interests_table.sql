-- +goose Up
CREATE TABLE interests (
    id UUID PRIMARY KEY,
    topic TEXT NOT NULL,
    embedding JSONB NOT NULL,
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_interests_created ON interests(created_at, id);

-- +goose Down
DROP TABLE IF EXISTS interests;
