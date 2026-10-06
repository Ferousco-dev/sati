-- +goose Up
CREATE TYPE user_role AS ENUM ('CUSTOMER', 'STAFF', 'ADMIN', 'RIDER');

CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL,
    email             CITEXT NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL,
    role              user_role NOT NULL DEFAULT 'CUSTOMER',
    email_verified    BOOLEAN NOT NULL DEFAULT FALSE,
    email_verified_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
DROP TYPE user_role;
