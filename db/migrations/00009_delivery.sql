-- +goose Up
CREATE TABLE delivery_zones (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    fee_kobo   BIGINT NOT NULL CHECK (fee_kobo >= 0),
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE addresses ADD COLUMN zone_id UUID REFERENCES delivery_zones (id) ON DELETE SET NULL;

CREATE TYPE delivery_status AS ENUM ('ASSIGNED', 'PICKED_UP', 'DELIVERED', 'FAILED', 'CANCELLED');

CREATE TABLE delivery_orders (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       UUID NOT NULL UNIQUE REFERENCES orders (id) ON DELETE CASCADE,
    rider_id       UUID NOT NULL REFERENCES users (id),
    zone_id        UUID REFERENCES delivery_zones (id) ON DELETE SET NULL,
    status         delivery_status NOT NULL DEFAULT 'ASSIGNED',
    assigned_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    assigned_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    picked_up_at   TIMESTAMPTZ,
    delivered_at   TIMESTAMPTZ,
    failure_reason TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX delivery_orders_rider_status_idx ON delivery_orders (rider_id, status);
CREATE INDEX delivery_orders_status_idx ON delivery_orders (status, assigned_at DESC);

-- +goose Down
DROP TABLE delivery_orders;
DROP TYPE delivery_status;
ALTER TABLE addresses DROP COLUMN zone_id;
DROP TABLE delivery_zones;
