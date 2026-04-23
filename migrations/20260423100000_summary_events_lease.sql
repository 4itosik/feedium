-- +goose Up
ALTER TABLE summary_events
    ADD COLUMN locked_until TIMESTAMPTZ NULL,
    ADD COLUMN locked_by TEXT NULL,
    ADD COLUMN attempt_count INT NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at TIMESTAMPTZ NULL;

CREATE INDEX idx_summary_events_claim
    ON summary_events(status, COALESCE(next_attempt_at, created_at));

CREATE INDEX idx_summary_events_processing_lease
    ON summary_events(locked_until)
    WHERE status = 'processing';

-- +goose Down
DROP INDEX IF EXISTS idx_summary_events_processing_lease;
DROP INDEX IF EXISTS idx_summary_events_claim;

ALTER TABLE summary_events
    DROP COLUMN IF EXISTS next_attempt_at,
    DROP COLUMN IF EXISTS attempt_count,
    DROP COLUMN IF EXISTS locked_by,
    DROP COLUMN IF EXISTS locked_until;
