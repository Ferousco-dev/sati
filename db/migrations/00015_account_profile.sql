-- +goose Up
-- phone is optional and only used to contact a customer about an order.
-- avatar_key points at the profile picture in file storage.
-- deleted_at marks an account the customer deleted. The row stays, so the email can never be
-- registered again and the account cannot be used unless someone restores it by hand.
ALTER TABLE users
    ADD COLUMN phone      TEXT NOT NULL DEFAULT '',
    ADD COLUMN avatar_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX users_deleted_at_idx ON users (deleted_at) WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX users_deleted_at_idx;
ALTER TABLE users DROP COLUMN phone, DROP COLUMN avatar_key, DROP COLUMN deleted_at;
