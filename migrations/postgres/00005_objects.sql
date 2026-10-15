-- +goose Up
-- Объекты аренды и их «дочерние» сущности (ERDB 2.10-2.15).
--
-- Объект описывает то, что сдаётся в аренду: тип и подтип, цену в минорных
-- единицах, минимальную длительность, местоположение (location_id + address) и
-- статус из справочника statuses (мягкое удаление — статус deleted (4), а не
-- отдельный флаг). Услуги (object_addons), фото (photos/object_photos) и
-- расписание (object_weekday_rules, object_schedule_exceptions) ссылаются на
-- объект и удаляются каскадом вместе с ним.
--
-- IF NOT EXISTS не используется: миграция версионирована, а чужая таблица с тем
-- же именем должна ронять старт явной ошибкой, а не подменять схему.
CREATE TABLE rental_objects (
    id                   UUID         PRIMARY KEY,
    type                 INT          NOT NULL REFERENCES type_objects (id),
    subtype              VARCHAR(50),
    title                VARCHAR(100) NOT NULL,
    description          TEXT,
    price_minor          BIGINT       NOT NULL,
    currency             CHAR(3)      NOT NULL,
    min_duration_minutes INT          NOT NULL,
    location_id          INT          REFERENCES locations (id),
    address              VARCHAR(254),
    status_id            INT          NOT NULL DEFAULT 1 REFERENCES statuses (id),
    owner_id             UUID         REFERENCES users (id),
    created_at           TIMESTAMPTZ  NOT NULL,
    updated_at           TIMESTAMPTZ  NOT NULL,
    CONSTRAINT rental_objects_price_minor_check CHECK (price_minor >= 0),
    CONSTRAINT rental_objects_min_duration_minutes_check CHECK (min_duration_minutes > 0),
    CONSTRAINT rental_objects_currency_check CHECK (char_length(currency) = 3)
);

CREATE INDEX rental_objects_type_idx ON rental_objects (type);

CREATE INDEX rental_objects_location_idx ON rental_objects (location_id);

CREATE INDEX rental_objects_owner_idx ON rental_objects (owner_id);

-- Дополнительные услуги объекта: цена в минорных единицах и код валюты ISO-4217
-- (пока CHAR(3), перевод на справочник currencies — отдельная задача).
CREATE TABLE object_addons (
    id               UUID         PRIMARY KEY,
    object_id        UUID         NOT NULL REFERENCES rental_objects (id) ON DELETE CASCADE,
    name             VARCHAR(100) NOT NULL,
    price_minor      BIGINT       NOT NULL,
    currency         CHAR(3)      NOT NULL,
    duration_minutes INT          NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ  NOT NULL,
    CONSTRAINT object_addons_price_minor_check CHECK (price_minor >= 0),
    CONSTRAINT object_addons_duration_minutes_check CHECK (duration_minutes >= 0),
    CONSTRAINT object_addons_currency_check CHECK (char_length(currency) = 3)
);

CREATE INDEX object_addons_object_id_idx ON object_addons (object_id);

-- Фото — самостоятельный бизнес-ресурс: файл лежит в S3-совместимом хранилище,
-- а в БД хранятся ключ и метаданные. Один файл может использоваться несколькими
-- объектами, поэтому «главное фото» и порядок показа живут в связке object_photos.
CREATE TABLE photos (
    id           UUID         PRIMARY KEY,
    object_key   TEXT         NOT NULL,
    content_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
    size_bytes   BIGINT       NOT NULL,
    width        INT,
    height       INT,
    created_at   TIMESTAMPTZ  NOT NULL,
    is_active    BOOLEAN      NOT NULL DEFAULT true,
    is_delete    BOOLEAN      NOT NULL DEFAULT false,
    CONSTRAINT photos_size_bytes_check CHECK (size_bytes >= 0),
    CONSTRAINT photos_width_check CHECK (width IS NULL OR width > 0),
    CONSTRAINT photos_height_check CHECK (height IS NULL OR height > 0)
);

-- Один ключ в S3 = одна запись.
CREATE UNIQUE INDEX photos_object_key_idx ON photos (object_key);

-- Связка M:N объект — фото. Составной PK запрещает привязать фото дважды, а
-- атрибуты связи описывают роль фото для конкретного объекта: is_main (обложка)
-- и sort_order (порядок в галерее).
CREATE TABLE object_photos (
    object_id  UUID        NOT NULL REFERENCES rental_objects (id) ON DELETE CASCADE,
    photo_id   UUID        NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    sort_order INT         NOT NULL DEFAULT 0,
    is_main    BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (object_id, photo_id)
);

-- «Не более одной обложки на объект»: частичный уникальный индекс.
CREATE UNIQUE INDEX object_photos_main_idx ON object_photos (object_id) WHERE is_main = true;

-- Обратный поиск «где используется фото».
CREATE INDEX object_photos_photo_id_idx ON object_photos (photo_id);

-- Шаблон недели: слоты в БД не хранятся, а вычисляются из правил. Чтобы
-- «выключить» интервал, valid_from/valid_to (NOT NULL) по умолчанию равны
-- эпохе (ERDB пишет DEFAULT 0: 1970-01-01 — тот же «нулевой» сдвиг).
CREATE TABLE object_weekday_rules (
    id                UUID        PRIMARY KEY,
    object_id         UUID        NOT NULL REFERENCES rental_objects (id) ON DELETE CASCADE,
    weekday           SMALLINT    NOT NULL,
    opens_at          TIME        NOT NULL,
    closes_at         TIME        NOT NULL,
    slot_step_minutes INT         NOT NULL,
    valid_from        DATE        NOT NULL DEFAULT '1970-01-01',
    valid_to          DATE        NOT NULL DEFAULT '1970-01-01',
    timezone          VARCHAR(64) NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,
    CONSTRAINT object_weekday_rules_weekday_check CHECK (weekday BETWEEN 0 AND 6),
    CONSTRAINT object_weekday_rules_slot_step_minutes_check CHECK (slot_step_minutes BETWEEN 5 AND 1440),
    CONSTRAINT object_weekday_rules_opens_before_closes_check CHECK (opens_at < closes_at)
);

CREATE INDEX object_weekday_rules_object_id_idx ON object_weekday_rules (object_id);

-- Исключения расписания: недоступный интервал по объекту. Автор исключения —
-- необязательная ссылка (ON DELETE SET NULL): запись остаётся, ссылка обнуляется.
CREATE TABLE object_schedule_exceptions (
    id           UUID         PRIMARY KEY,
    object_id    UUID         NOT NULL REFERENCES rental_objects (id) ON DELETE CASCADE,
    period_start TIMESTAMPTZ  NOT NULL,
    period_end   TIMESTAMPTZ  NOT NULL,
    reason       VARCHAR(255),
    created_by   UUID         REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ  NOT NULL,
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
