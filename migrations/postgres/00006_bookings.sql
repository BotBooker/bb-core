-- +goose Up
-- Брони, услуги в брони и платежи (ERDB 2.16-2.19).
--
-- Бронь связывает объект и клиента на интервал slot_start..slot_end; статус и
-- валюта берутся из справочников (booking_statuses, currencies), а цены хранятся
-- в минорных единицах. Двойную бронь защищает ограничение БД: в PostgreSQL —
-- EXCLUDE по диапазону с расширением btree_gist (активные статусы 1, 2, 3).
--
-- IF NOT EXISTS не используется для таблиц: миграция версионирована, а чужая
-- таблица с тем же именем должна ронять старт явной ошибкой. Для расширения
-- btree_gist IF NOT EXISTS нужен: оно принадлежит базе, а не схеме миграций, и
-- при повторной раскатке в новую схему уже установлено.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE bookings (
    id                UUID         PRIMARY KEY,
    object_id         UUID         NOT NULL REFERENCES rental_objects (id),
    client_id         UUID         NOT NULL REFERENCES users (id),
    slot_start        TIMESTAMPTZ  NOT NULL,
    slot_end          TIMESTAMPTZ  NOT NULL,
    status_id         SMALLINT     NOT NULL DEFAULT 1 REFERENCES booking_statuses (id),
    total_price_minor BIGINT       NOT NULL DEFAULT 0,
    currency_id       SMALLINT     NOT NULL DEFAULT 1 REFERENCES currencies (id),
    idempotency_key   VARCHAR(64)  NOT NULL DEFAULT '',
    cancel_reason     VARCHAR(255) NOT NULL DEFAULT '',
    cancelled_at      TIMESTAMPTZ  NOT NULL DEFAULT 'epoch',
    created_at        TIMESTAMPTZ  NOT NULL,
    updated_at        TIMESTAMPTZ  NOT NULL,
    CONSTRAINT bookings_slot_check CHECK (slot_end > slot_start),
    CONSTRAINT bookings_total_price_minor_check CHECK (total_price_minor >= 0)
);

-- Анти-оверлап: две активные брони (pending, confirmed, paid) одного объекта не
-- могут пересекаться по времени. tstzrange с '[)' считает интервал полуоткрытым.
ALTER TABLE bookings
    ADD CONSTRAINT bookings_no_overlap EXCLUDE USING gist (
        object_id WITH =,
        tstzrange(slot_start, slot_end, '[)') WITH &&
    ) WHERE (status_id IN (1, 2, 3));

-- Идемпотентность создания брони клиентом.
CREATE UNIQUE INDEX bookings_idempotency_idx ON bookings (client_id, idempotency_key);

-- «Брони клиента» (свежие сверху) и «слоты объекта» — горячие выборки.
CREATE INDEX bookings_client_slot_idx ON bookings (client_id, slot_start DESC);

CREATE INDEX bookings_object_start_idx ON bookings (object_id, slot_start);

CREATE INDEX bookings_status_id_idx ON bookings (status_id);

-- Услуги в брони: снимок выбранных услуг. unit_price_minor фиксирует цену на
-- момент брони, поэтому изменение прайса не переписывает историю.
CREATE TABLE booking_addons (
    booking_id       UUID   NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    addon_id         UUID   NOT NULL REFERENCES object_addons (id),
    quantity         INT    NOT NULL DEFAULT 1,
    unit_price_minor BIGINT NOT NULL,
    PRIMARY KEY (booking_id, addon_id),
    CONSTRAINT booking_addons_quantity_check CHECK (quantity > 0),
    CONSTRAINT booking_addons_unit_price_minor_check CHECK (unit_price_minor >= 0)
);

-- Журнал бронирований: неизменяемый снимок состояния брони на момент изменения
-- (намеренная денормализация ради истории и разбора инцидентов). user_id и
-- object_id — необязательные ссылки для аудита.
CREATE TABLE bookings_journal (
    id                UUID        PRIMARY KEY,
    booking_id        UUID        NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    user_id           UUID        REFERENCES users (id),
    object_id         UUID        REFERENCES rental_objects (id),
    slot_start        TIMESTAMPTZ NOT NULL,
    slot_end          TIMESTAMPTZ NOT NULL,
    status_id         SMALLINT    NOT NULL REFERENCES booking_statuses (id),
    total_price_minor BIGINT      NOT NULL,
    currency_id       SMALLINT    NOT NULL REFERENCES currencies (id),
    created_at        TIMESTAMPTZ NOT NULL
);

CREATE INDEX bookings_journal_booking_idx ON bookings_journal (booking_id, created_at DESC);

-- Платежи: деньги в минорных единицах, валюта и способ оплаты — из справочников.
-- status — нативный ENUM: набор значений закрытый, проверяется СУБД.
CREATE TYPE payment_status AS ENUM ('succeeded', 'failed', 'refunded');

CREATE TABLE payments (
    id                UUID           PRIMARY KEY,
    booking_id        UUID           NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    user_id           UUID           NOT NULL REFERENCES users (id),
    amount_minor      BIGINT         NOT NULL,
    currency_id       SMALLINT       NOT NULL REFERENCES currencies (id),
    payment_method_id SMALLINT       NOT NULL REFERENCES payment_methods (id),
    status            payment_status NOT NULL,
    paid_at           TIMESTAMPTZ    NOT NULL DEFAULT 'epoch',
    created_at        TIMESTAMPTZ    NOT NULL,
    CONSTRAINT payments_amount_minor_check CHECK (amount_minor >= 0)
);

CREATE INDEX payments_booking_idx ON payments (booking_id);

CREATE INDEX payments_status_idx ON payments (status);

CREATE INDEX payments_payment_method_id_idx ON payments (payment_method_id);

-- +goose Down
-- Откат: сначала платежи (ссылаются на bookings, users, справочники), затем
-- журнал и услуги, потом брони и собственный тип ENUM платежа.
DROP TABLE IF EXISTS payments;

DROP TYPE IF EXISTS payment_status;

DROP TABLE IF EXISTS bookings_journal;

DROP TABLE IF EXISTS booking_addons;

DROP TABLE IF EXISTS bookings;
