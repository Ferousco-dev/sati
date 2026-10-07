-- +goose Up
CREATE TABLE media (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename     TEXT NOT NULL,
    storage_key  TEXT NOT NULL UNIQUE,
    content_type TEXT NOT NULL,
    size_bytes   INTEGER NOT NULL,
    width        INTEGER NOT NULL DEFAULT 0,
    height       INTEGER NOT NULL DEFAULT 0,
    alt_text     TEXT NOT NULL DEFAULT '',
    uploaded_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX media_created_idx ON media (created_at DESC);

CREATE TABLE content_sections (
    key              TEXT PRIMARY KEY,
    published_version INTEGER NOT NULL DEFAULT 0,
    draft            JSONB,
    draft_updated_at TIMESTAMPTZ,
    draft_updated_by UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE content_versions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    section_key  TEXT NOT NULL REFERENCES content_sections (key) ON DELETE CASCADE,
    version      INTEGER NOT NULL,
    data         JSONB NOT NULL,
    note         TEXT NOT NULL DEFAULT '',
    published_by UUID REFERENCES users (id) ON DELETE SET NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (section_key, version)
);

-- +goose Down
DROP TABLE content_versions;
DROP TABLE content_sections;
DROP TABLE media;
