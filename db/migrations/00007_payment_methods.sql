-- +goose Up
ALTER TYPE payment_method ADD VALUE IF NOT EXISTS 'PAY_ON_DELIVERY';
ALTER TYPE payment_method ADD VALUE IF NOT EXISTS 'BANK_TRANSFER';

ALTER TABLE store_settings
    ADD COLUMN online_enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN bank_transfer_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN pay_on_pickup_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN pay_on_delivery_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN bank_name               TEXT NOT NULL DEFAULT '',
    ADD COLUMN bank_account_number     TEXT NOT NULL DEFAULT '',
    ADD COLUMN bank_account_name       TEXT NOT NULL DEFAULT '',
    ADD COLUMN whatsapp_number         TEXT NOT NULL DEFAULT '',
    ADD COLUMN whatsapp_message        TEXT NOT NULL DEFAULT 'Hello, I have paid for order {order_number}. Total: {total}.';

-- +goose Down
ALTER TABLE store_settings
    DROP COLUMN online_enabled,
    DROP COLUMN bank_transfer_enabled,
    DROP COLUMN pay_on_pickup_enabled,
    DROP COLUMN pay_on_delivery_enabled,
    DROP COLUMN bank_name,
    DROP COLUMN bank_account_number,
    DROP COLUMN bank_account_name,
    DROP COLUMN whatsapp_number,
    DROP COLUMN whatsapp_message;
