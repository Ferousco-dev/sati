-- +goose Up
CREATE TABLE notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    order_id   UUID REFERENCES orders (id) ON DELETE CASCADE,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX notifications_user_created_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_user_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

CREATE TYPE email_job_status AS ENUM ('PENDING', 'SENDING', 'SENT', 'FAILED');

CREATE TABLE email_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind            TEXT NOT NULL,
    to_email        TEXT NOT NULL,
    to_name         TEXT NOT NULL DEFAULT '',
    payload         JSONB NOT NULL DEFAULT '{}',
    status          email_job_status NOT NULL DEFAULT 'PENDING',
    attempts        INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 5,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at       TIMESTAMPTZ,
    last_error      TEXT NOT NULL DEFAULT '',
    dedupe_key      TEXT UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at         TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);

CREATE INDEX email_jobs_due_idx ON email_jobs (next_attempt_at) WHERE status IN ('PENDING', 'SENDING');
CREATE INDEX email_jobs_finished_idx ON email_jobs (finished_at) WHERE finished_at IS NOT NULL;

ALTER TABLE store_settings ADD COLUMN notification_email TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE store_settings DROP COLUMN notification_email;
DROP TABLE email_jobs;
DROP TYPE email_job_status;
DROP TABLE notifications;
