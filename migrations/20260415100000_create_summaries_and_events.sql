-- +goose Up
CREATE TABLE summaries (
    id UUID PRIMARY KEY,
    post_id UUID NULL REFERENCES posts(id) ON DELETE CASCADE,
    source_id UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    word_count INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_summaries_post_id ON summaries(post_id, created_at DESC);
CREATE INDEX idx_summaries_source_created ON summaries(source_id, created_at DESC, id DESC);

CREATE TABLE summary_events (
    id UUID PRIMARY KEY,
    post_id UUID NULL,
    source_id UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    summary_id UUID NULL REFERENCES summaries(id),
    error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX idx_summary_events_unique_active_post ON summary_events(post_id, event_type) WHERE status IN ('pending', 'processing');
CREATE UNIQUE INDEX idx_summary_events_unique_active_source ON summary_events(source_id) WHERE event_type = 'summarize_source' AND status IN ('pending', 'processing');
CREATE INDEX idx_summary_events_pending ON summary_events(status, created_at) WHERE status = 'pending';
CREATE INDEX idx_summary_events_source_type_status ON summary_events(source_id, event_type, status);

-- +goose Down
DROP TABLE IF EXISTS summary_events;
DROP TABLE IF EXISTS summaries;
