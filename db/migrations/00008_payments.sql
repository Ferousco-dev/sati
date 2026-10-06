-- +goose Up
ALTER TABLE orders ADD COLUMN status_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE payments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID NOT NULL REFERENCES orders (id),
    method      payment_method NOT NULL,
    amount_kobo BIGINT NOT NULL CHECK (amount_kobo >= 0),
    status      payment_status NOT NULL,
    provider    TEXT NOT NULL,
    reference   TEXT,
    recorded_by UUID REFERENCES users (id) ON DELETE SET NULL,
    note        TEXT NOT NULL DEFAULT '',
    paid_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX payments_order_id_idx ON payments (order_id);
CREATE INDEX payments_paid_at_idx ON payments (paid_at) WHERE status = 'PAID';
CREATE UNIQUE INDEX payments_one_paid_per_order_idx ON payments (order_id) WHERE status = 'PAID';
CREATE UNIQUE INDEX payments_provider_reference_idx ON payments (provider, reference) WHERE reference IS NOT NULL;

-- +goose Down
DROP TABLE payments;
ALTER TABLE orders DROP COLUMN status_reason;
