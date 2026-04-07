-- +goose Up
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    post_id UUID REFERENCES posts(id) ON DELETE SET NULL,
    event_type VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    retry_count INTEGER NOT NULL DEFAULT 0,
    scheduled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_outbox_events_status_scheduled ON outbox_events(status, scheduled_at, created_at);

CREATE TABLE summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    event_id UUID NOT NULL REFERENCES outbox_events(id) ON DELETE RESTRICT,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_summaries_source_id ON summaries(source_id);
CREATE INDEX idx_summaries_event_id ON summaries(event_id);

CREATE TABLE summary_posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    summary_id UUID NOT NULL REFERENCES summaries(id) ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    UNIQUE(summary_id, post_id)
);

CREATE INDEX idx_summary_posts_summary_id ON summary_posts(summary_id);
CREATE INDEX idx_summary_posts_post_id ON summary_posts(post_id);

-- +goose Down
DROP TABLE IF EXISTS summary_posts;
DROP TABLE IF EXISTS summaries;
DROP TABLE IF EXISTS outbox_events;
