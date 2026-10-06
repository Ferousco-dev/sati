-- +goose Up
CREATE TABLE store_settings (
    id                BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    delivery_fee_kobo BIGINT NOT NULL DEFAULT 0 CHECK (delivery_fee_kobo >= 0),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO store_settings (id) VALUES (TRUE);

CREATE TABLE addresses (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    recipient_name TEXT NOT NULL,
    phone          TEXT NOT NULL,
    address_line   TEXT NOT NULL,
    area           TEXT NOT NULL,
    city           TEXT NOT NULL,
    instructions   TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX addresses_user_id_idx ON addresses (user_id);

CREATE TYPE order_status AS ENUM (
    'PENDING', 'CONFIRMED', 'PREPARING', 'READY',
    'OUT_FOR_DELIVERY', 'DELIVERED', 'COMPLETED', 'CANCELLED', 'FAILED'
);
CREATE TYPE payment_status AS ENUM (
    'UNPAID', 'PENDING', 'PAID', 'FAILED', 'REFUNDED', 'PARTIALLY_REFUNDED'
);
CREATE TYPE payment_method AS ENUM ('ONLINE', 'PAY_ON_PICKUP');
CREATE TYPE fulfillment_type AS ENUM ('PICKUP', 'DELIVERY');

CREATE SEQUENCE order_number_seq START 10001;

CREATE TABLE orders (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_number        BIGINT NOT NULL UNIQUE DEFAULT nextval('order_number_seq'),
    customer_id         UUID NOT NULL REFERENCES users (id),
    status              order_status NOT NULL DEFAULT 'PENDING',
    fulfillment_type    fulfillment_type NOT NULL,
    subtotal_kobo       BIGINT NOT NULL CHECK (subtotal_kobo >= 0),
    delivery_fee_kobo   BIGINT NOT NULL CHECK (delivery_fee_kobo >= 0),
    total_kobo          BIGINT NOT NULL CHECK (total_kobo >= 0),
    payment_status      payment_status NOT NULL DEFAULT 'UNPAID',
    payment_method      payment_method NOT NULL,
    payment_provider    TEXT,
    payment_reference   TEXT,
    delivery_address_id UUID REFERENCES addresses (id) ON DELETE SET NULL,
    delivery_address    JSONB,
    notes               TEXT NOT NULL DEFAULT '',
    idempotency_key     TEXT,
    cancelled_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX orders_customer_created_idx ON orders (customer_id, created_at DESC);
CREATE INDEX orders_status_created_idx ON orders (status, created_at DESC);
CREATE UNIQUE INDEX orders_customer_idempotency_idx ON orders (customer_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE order_items (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id                UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id              UUID NOT NULL REFERENCES products (id),
    product_name_snapshot   TEXT NOT NULL,
    unit_price_snapshot_kobo BIGINT NOT NULL CHECK (unit_price_snapshot_kobo >= 0),
    quantity                INTEGER NOT NULL CHECK (quantity > 0),
    line_total_kobo         BIGINT NOT NULL CHECK (line_total_kobo >= 0)
);

CREATE INDEX order_items_order_id_idx ON order_items (order_id);

-- +goose Down
DROP TABLE order_items;
DROP TABLE orders;
DROP SEQUENCE order_number_seq;
DROP TYPE fulfillment_type;
DROP TYPE payment_method;
DROP TYPE payment_status;
DROP TYPE order_status;
DROP TABLE addresses;
DROP TABLE store_settings;
