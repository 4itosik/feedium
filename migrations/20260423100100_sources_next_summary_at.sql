-- +goose Up
ALTER TABLE sources
    ADD COLUMN next_summary_at TIMESTAMPTZ NULL;

CREATE INDEX idx_sources_next_summary_at
    ON sources(next_summary_at)
    WHERE next_summary_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_sources_next_summary_at;
ALTER TABLE sources DROP COLUMN IF EXISTS next_summary_at;
