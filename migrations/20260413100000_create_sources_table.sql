-- +goose Up
-- UUID v7 PK — deliberate deviation from database.md convention; justified by stable cursor-based pagination.
CREATE TABLE sources (
    id UUID PRIMARY KEY,
    type TEXT NOT NULL,
    config JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sources_type_created_at_id ON sources(type, created_at, id);

-- +goose Down
DROP TABLE IF EXISTS sources;
