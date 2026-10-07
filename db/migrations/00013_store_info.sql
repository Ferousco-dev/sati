-- +goose Up
ALTER TABLE store_settings
    ADD COLUMN store_name         TEXT NOT NULL DEFAULT 'SATI-L Yoghurt & Confectionery',
    ADD COLUMN store_address      TEXT NOT NULL DEFAULT '',
    ADD COLUMN store_phone        TEXT NOT NULL DEFAULT '',
    ADD COLUMN store_email        TEXT NOT NULL DEFAULT '',
    ADD COLUMN instagram_url      TEXT NOT NULL DEFAULT '',
    ADD COLUMN tiktok_url         TEXT NOT NULL DEFAULT '',
    ADD COLUMN facebook_url       TEXT NOT NULL DEFAULT '',
    ADD COLUMN map_embed_url      TEXT NOT NULL DEFAULT '',
    ADD COLUMN opening_hours      JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN temporarily_closed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN closed_message     TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE store_settings
    DROP COLUMN store_name, DROP COLUMN store_address, DROP COLUMN store_phone, DROP COLUMN store_email,
    DROP COLUMN instagram_url, DROP COLUMN tiktok_url, DROP COLUMN facebook_url, DROP COLUMN map_embed_url,
    DROP COLUMN opening_hours, DROP COLUMN temporarily_closed, DROP COLUMN closed_message;
