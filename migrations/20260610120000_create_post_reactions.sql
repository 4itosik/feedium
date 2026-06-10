-- +goose Up
CREATE TABLE post_reactions (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id    UUID        NOT NULL UNIQUE REFERENCES posts(id) ON DELETE CASCADE,
    signal     TEXT        NOT NULL CHECK (signal IN ('like', 'dislike')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS post_reactions;
