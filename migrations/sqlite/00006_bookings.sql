-- +goose Up
-- Брони, услуги в брони и платежи (ERDB 2.16-2.19, SQLite-вариант схемы,
-- см. migrations/postgres/00006_bookings.sql).
--
-- Анти-оверлап реализован fallback-вариантом из ERDB 4.16: у SQLite нет
-- диапазонных EXCLUDE-ограничений, поэтому «две активные брони (1, 2, 3) одного
-- объекта не начинаются в один момент» держит частичный уникальный индекс
-- bookings_object_slot_idx, а полную проверку пересечения делает приложение в
-- транзакции. Остальные отличия — неточные типы (TEXT/INTEGER/REAL), ENUM через
-- CHECK, время — строка RFC3339 в UTC.
CREATE TABLE bookings (
    id                TEXT    PRIMARY KEY,
    object_id         TEXT    NOT NULL REFERENCES rental_objects (id),
    client_id         TEXT    NOT NULL REFERENCES users (id),
    slot_start        TEXT    NOT NULL,
    slot_end          TEXT    NOT NULL,
    status_id         INTEGER NOT NULL DEFAULT 1 REFERENCES booking_statuses (id),
    total_price_minor INTEGER NOT NULL DEFAULT 0,
    currency_id       INTEGER NOT NULL DEFAULT 1 REFERENCES currencies (id),
    idempotency_key   TEXT    NOT NULL DEFAULT '',
    cancel_reason     TEXT    NOT NULL DEFAULT '',
    cancelled_at      TEXT    NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    created_at        TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL,
    CONSTRAINT bookings_slot_check CHECK (slot_end > slot_start),
    CONSTRAINT bookings_total_price_minor_check CHECK (total_price_minor >= 0)
);

-- Идемпотентность создания брони клиентом.
CREATE UNIQUE INDEX bookings_idempotency_idx ON bookings (client_id, idempotency_key);

-- Fallback анти-оверлапа для SQLite: активные брони одного объекта не могут
-- начинаться в один момент времени (status_id 1, 2, 3 — pending/confirmed/paid).
CREATE UNIQUE INDEX bookings_object_slot_idx ON bookings (object_id, slot_start)
    WHERE status_id IN (1, 2, 3);

-- «Брони клиента» (свежие сверху) и «слоты объекта» — горячие выборки.
CREATE INDEX bookings_client_slot_idx ON bookings (client_id, slot_start DESC);

CREATE INDEX bookings_object_start_idx ON bookings (object_id, slot_start);

CREATE INDEX bookings_status_id_idx ON bookings (status_id);

CREATE TABLE booking_addons (
    booking_id       TEXT    NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    addon_id         TEXT    NOT NULL REFERENCES object_addons (id),
    quantity         INTEGER NOT NULL DEFAULT 1,
    unit_price_minor INTEGER NOT NULL,
    PRIMARY KEY (booking_id, addon_id),
    CONSTRAINT booking_addons_quantity_check CHECK (quantity > 0),
    CONSTRAINT booking_addons_unit_price_minor_check CHECK (unit_price_minor >= 0)
);

CREATE TABLE bookings_journal (
    id                TEXT    PRIMARY KEY,
    booking_id        TEXT    NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    user_id           TEXT    REFERENCES users (id),
    object_id         TEXT    REFERENCES rental_objects (id),
    slot_start        TEXT    NOT NULL,
    slot_end          TEXT    NOT NULL,
    status_id         INTEGER NOT NULL REFERENCES booking_statuses (id),
    total_price_minor INTEGER NOT NULL,
    currency_id       INTEGER NOT NULL REFERENCES currencies (id),
    created_at        TEXT    NOT NULL
);

CREATE INDEX bookings_journal_booking_idx ON bookings_journal (booking_id, created_at DESC);

CREATE TABLE payments (
    id                TEXT    PRIMARY KEY,
    booking_id        TEXT    NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    user_id           TEXT    NOT NULL REFERENCES users (id),
    amount_minor      INTEGER NOT NULL,
    currency_id       INTEGER NOT NULL REFERENCES currencies (id),
    payment_method_id INTEGER NOT NULL REFERENCES payment_methods (id),
    status            TEXT    NOT NULL
        CONSTRAINT payments_status_check CHECK (status IN ('succeeded', 'failed', 'refunded')),
    paid_at           TEXT    NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    created_at        TEXT    NOT NULL,
    CONSTRAINT payments_amount_minor_check CHECK (amount_minor >= 0)
);

CREATE INDEX payments_booking_idx ON payments (booking_id);

CREATE INDEX payments_status_idx ON payments (status);

CREATE INDEX payments_payment_method_id_idx ON payments (payment_method_id);

-- +goose Down
-- Откат: сначала платежи, затем журнал, услуги брони и сами брони.
DROP TABLE IF EXISTS payments;

DROP TABLE IF EXISTS bookings_journal;

DROP TABLE IF EXISTS booking_addons;

DROP TABLE IF EXISTS bookings;
