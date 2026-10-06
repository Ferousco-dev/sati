-- +goose Up
ALTER TABLE payments
    ADD COLUMN needs_refund            BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN gateway_response        TEXT NOT NULL DEFAULT '',
    ADD COLUMN provider_transaction_id TEXT;

DROP INDEX payments_one_paid_per_order_idx;
CREATE UNIQUE INDEX payments_one_paid_per_order_idx ON payments (order_id) WHERE status = 'PAID' AND NOT needs_refund;
CREATE INDEX payments_needs_refund_idx ON payments (created_at DESC) WHERE needs_refund;

CREATE TABLE payment_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider   TEXT NOT NULL,
    event_key  TEXT NOT NULL,
    event_type TEXT NOT NULL,
    reference  TEXT NOT NULL DEFAULT '',
    payload    JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, event_key)
);

-- +goose Down
DROP TABLE payment_events;
DROP INDEX payments_needs_refund_idx;
DROP INDEX payments_one_paid_per_order_idx;
CREATE UNIQUE INDEX payments_one_paid_per_order_idx ON payments (order_id) WHERE status = 'PAID';
ALTER TABLE payments DROP COLUMN needs_refund, DROP COLUMN gateway_response, DROP COLUMN provider_transaction_id;
