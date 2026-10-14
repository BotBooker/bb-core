-- +goose Up
-- Фундамент схемы: справочники (reference tables) сервиса BotBooker
-- (SQLite-вариант схемы, см. migrations/postgres/00001_foundation.sql).
--
-- Отличия от PostgreSQL — только там, где диалекту не хватает возможностей:
--   * типы неточные: SMALLINT/INT -> INTEGER, CHAR(3) -> TEXT + CHECK,
--     BOOLEAN -> INTEGER 0/1, DOUBLE PRECISION -> REAL, TIMESTAMPTZ -> TEXT
--     (строки RFC3339 в UTC, формат задаёт репозиторий);
--   * автоинкремент -> INTEGER PRIMARY KEY AUTOINCREMENT (у SQLite он работает
--     только для INTEGER PRIMARY KEY).
-- Значения, назначенные миграцией, заданы явно и совпадают с PostgreSQL, чтобы
-- «число -> код» было одинаковым в обоих диалектах.
CREATE TABLE roles (
    id        INTEGER PRIMARY KEY,
    code      TEXT    NOT NULL,
    title     TEXT    NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX roles_code_idx ON roles (code);

CREATE TABLE type_objects (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT    NOT NULL
);

CREATE UNIQUE INDEX type_objects_title_idx ON type_objects (title);

CREATE TABLE locations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    type       TEXT    NOT NULL,
    region     TEXT    NOT NULL,
    population INTEGER,
    lat        REAL,
    lon        REAL,
    timezone   TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL,
    CONSTRAINT locations_population_check CHECK (population IS NULL OR population >= 0),
    CONSTRAINT locations_lat_check CHECK (lat IS NULL OR lat BETWEEN -90 AND 90),
    CONSTRAINT locations_lon_check CHECK (lon IS NULL OR lon BETWEEN -180 AND 180)
);

CREATE UNIQUE INDEX locations_name_region_idx ON locations (name, region);

CREATE INDEX locations_region_idx ON locations (region);

CREATE TABLE statuses (
    id    INTEGER PRIMARY KEY,
    code  TEXT    NOT NULL,
    title TEXT    NOT NULL
);

CREATE UNIQUE INDEX statuses_code_idx ON statuses (code);

CREATE TABLE booking_statuses (
    id        INTEGER PRIMARY KEY,
    code      TEXT    NOT NULL,
    title     TEXT    NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX booking_statuses_code_idx ON booking_statuses (code);

CREATE TABLE currencies (
    id          INTEGER PRIMARY KEY,
    code        TEXT    NOT NULL,
    title       TEXT    NOT NULL,
    symbol      TEXT    NOT NULL DEFAULT '',
    minor_units INTEGER NOT NULL DEFAULT 2,
    is_active   INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT currencies_code_length_check CHECK (length(code) = 3),
    CONSTRAINT currencies_minor_units_check CHECK (minor_units >= 0)
);

CREATE UNIQUE INDEX currencies_code_idx ON currencies (code);

CREATE TABLE payment_methods (
    id        INTEGER PRIMARY KEY,
    code      TEXT    NOT NULL,
    title     TEXT    NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX payment_methods_code_idx ON payment_methods (code);

-- Seed: значения из ERDB.md (разделы 2.6, 2.9, 2.22, 2.23, 2.24).
INSERT INTO roles (id, code, title) VALUES
    (1, 'admin', 'Администратор'),
    (2, 'operator', 'Оператор'),
    (3, 'manager', 'Менеджер'),
    (32767, 'client', 'Пользователь');

INSERT INTO statuses (id, code, title) VALUES
    (1, 'active', 'Активен'),
    (2, 'under_repair', 'На ремонте'),
    (3, 'archived', 'В архиве'),
    (4, 'deleted', 'Удалён');

INSERT INTO booking_statuses (id, code, title) VALUES
    (1, 'pending', 'Ожидает подтверждения'),
    (2, 'confirmed', 'Подтверждена'),
    (3, 'paid', 'Оплачена'),
    (4, 'cancelled', 'Отменена'),
    (5, 'completed', 'Завершена');

INSERT INTO currencies (id, code, title, symbol) VALUES
    (1, 'RUB', 'Российский рубль', '₽'),
    (2, 'USD', 'Доллар США', '$'),
    (3, 'EUR', 'Евро', '€');

INSERT INTO payment_methods (id, code, title) VALUES
    (1, 'card', 'Банковская карта'),
    (2, 'sbp', 'СБП'),
    (3, 'wallet', 'Электронный кошелёк');

-- +goose Down
-- Откат: таблицы удаляются в порядке, обратном созданию; seed уходит вместе с
-- таблицами.
DROP TABLE IF EXISTS payment_methods;

DROP TABLE IF EXISTS currencies;

DROP TABLE IF EXISTS booking_statuses;

DROP TABLE IF EXISTS statuses;

DROP TABLE IF EXISTS locations;

DROP TABLE IF EXISTS type_objects;

DROP TABLE IF EXISTS roles;
