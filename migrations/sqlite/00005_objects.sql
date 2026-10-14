-- +goose Up
-- Объекты аренды и их «дочерние» сущности (ERDB 2.10-2.15, SQLite-вариант схемы,
-- см. migrations/postgres/00005_objects.sql).
--
-- Отличия от PostgreSQL — только там, где диалекту не хватает возможностей:
--   * id/object_id/owner_id/created_by — TEXT (UUID строкой);
--   * CHAR(3) -> TEXT + CHECK (length(currency) = 3);
--   * BOOLEAN -> INTEGER 0/1, DOUBLE PRECISION -> REAL;
--   * время — строки RFC3339 в UTC, дата — YYYY-MM-DD, время суток — HH:MM.
CREATE TABLE rental_objects (
    id                   TEXT    PRIMARY KEY,
    type                 INTEGER NOT NULL REFERENCES type_objects (id),
    subtype              TEXT,
    title                TEXT    NOT NULL,
    description          TEXT,
    price_minor          INTEGER NOT NULL,
    currency             TEXT    NOT NULL,
    min_duration_minutes INTEGER NOT NULL,
    location_id          INTEGER REFERENCES locations (id),
    address              TEXT,
    status_id            INTEGER NOT NULL DEFAULT 1 REFERENCES statuses (id),
    owner_id             TEXT    REFERENCES users (id),
    created_at           TEXT    NOT NULL,
    updated_at           TEXT    NOT NULL,
    CONSTRAINT rental_objects_price_minor_check CHECK (price_minor >= 0),
    CONSTRAINT rental_objects_min_duration_minutes_check CHECK (min_duration_minutes > 0),
    CONSTRAINT rental_objects_currency_check CHECK (length(currency) = 3)
);

CREATE INDEX rental_objects_type_idx ON rental_objects (type);

CREATE INDEX rental_objects_location_idx ON rental_objects (location_id);

CREATE INDEX rental_objects_owner_idx ON rental_objects (owner_id);

CREATE TABLE object_addons (
    id               TEXT    PRIMARY KEY,
    object_id        TEXT    NOT NULL REFERENCES rental_objects (id) ON DELETE CASCADE,
    name             TEXT    NOT NULL,
    price_minor      INTEGER NOT NULL,
    currency         TEXT    NOT NULL,
    duration_minutes INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT    NOT NULL,
    CONSTRAINT object_addons_price_minor_check CHECK (price_minor >= 0),
    CONSTRAINT object_addons_duration_minutes_check CHECK (duration_minutes >= 0),
    CONSTRAINT object_addons_currency_check CHECK (length(currency) = 3)
);

CREATE INDEX object_addons_object_id_idx ON object_addons (object_id);

CREATE TABLE photos (
    id           TEXT    PRIMARY KEY,
    object_key   TEXT    NOT NULL,
    content_type TEXT    NOT NULL DEFAULT 'application/octet-stream',
    size_bytes   INTEGER NOT NULL,
    width        INTEGER,
    height       INTEGER,
    created_at   TEXT    NOT NULL,
    is_active    INTEGER NOT NULL DEFAULT 1,
    is_delete    INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT photos_size_bytes_check CHECK (size_bytes >= 0),
    CONSTRAINT photos_width_check CHECK (width IS NULL OR width > 0),
    CONSTRAINT photos_height_check CHECK (height IS NULL OR height > 0)
);

-- Один ключ в S3 = одна запись.
CREATE UNIQUE INDEX photos_object_key_idx ON photos (object_key);

CREATE TABLE object_photos (
    object_id  TEXT    NOT NULL REFERENCES rental_objects (id) ON DELETE CASCADE,
    photo_id   TEXT    NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_main    INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL,
    PRIMARY KEY (object_id, photo_id)
);

-- «Не более одной обложки на объект»: частичный уникальный индекс.
CREATE UNIQUE INDEX object_photos_main_idx ON object_photos (object_id) WHERE is_main = 1;

-- Обратный поиск «где используется фото».
CREATE INDEX object_photos_photo_id_idx ON object_photos (photo_id);

CREATE TABLE object_weekday_rules (
    id                TEXT    PRIMARY KEY,
    object_id         TEXT    NOT NULL REFERENCES rental_objects (id) ON DELETE CASCADE,
    weekday           INTEGER NOT NULL,
    opens_at          TEXT    NOT NULL,
    closes_at         TEXT    NOT NULL,
    slot_step_minutes INTEGER NOT NULL,
    valid_from        TEXT    NOT NULL DEFAULT '1970-01-01',
    valid_to          TEXT    NOT NULL DEFAULT '1970-01-01',
    timezone          TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL,
    CONSTRAINT object_weekday_rules_weekday_check CHECK (weekday BETWEEN 0 AND 6),
    CONSTRAINT object_weekday_rules_slot_step_minutes_check CHECK (slot_step_minutes BETWEEN 5 AND 1440),
    CONSTRAINT object_weekday_rules_opens_before_closes_check CHECK (opens_at < closes_at)
);

CREATE INDEX object_weekday_rules_object_id_idx ON object_weekday_rules (object_id);

CREATE TABLE object_schedule_exceptions (
    id           TEXT    PRIMARY KEY,
    object_id    TEXT    NOT NULL REFERENCES rental_objects (id) ON DELETE CASCADE,
    period_start TEXT    NOT NULL,
    period_end   TEXT    NOT NULL,
    reason       TEXT,
    created_by   TEXT    REFERENCES users (id) ON DELETE SET NULL,
    created_at   TEXT    NOT NULL,
    CONSTRAINT object_schedule_exceptions_period_check CHECK (period_end > period_start)
);

CREATE INDEX object_schedule_exceptions_object_period_idx
    ON object_schedule_exceptions (object_id, period_start, period_end);

-- +goose Down
-- Откат: дочерние таблицы удаляются до rental_objects (на неё ссылаются FK).
DROP TABLE IF EXISTS object_schedule_exceptions;

DROP TABLE IF EXISTS object_weekday_rules;

DROP TABLE IF EXISTS object_photos;

DROP TABLE IF EXISTS photos;

DROP TABLE IF EXISTS object_addons;

DROP TABLE IF EXISTS rental_objects;
