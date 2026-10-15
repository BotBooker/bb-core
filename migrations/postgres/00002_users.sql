-- +goose Up
-- Пользователи сервиса BotBooker — профиль (ERDB 2.3).
--
-- Внешний контракт — proto user.v1 (идентификатор uuid строкой), служебные поля —
-- из ERDB (время последнего входа, признаки мягкого удаления). Хеш пароля здесь
-- не хранится: он вынесен в отдельную таблицу user_credentials (миграция 00004).
--
-- Роли у профиля нет: пользователь может состоять в нескольких тенантах и в
-- каждом иметь свою роль (tenant_members.role_id, миграция 00003), поэтому роль
-- хранится в членстве, а не в users.
--
-- id — нативный UUID и первичный ключ: 16 байт вместо 36, формат проверяет сама
-- СУБД, а отдельная колонка uuid не нужна — идентификатор один. Значение
-- генерирует приложение (model.NewUser → UUIDv4): в SQLite генерации UUID нет,
-- а оба диалекта должны получать идентификатор одинаково, поэтому DEFAULT нет.
-- gender — нативный ENUM: набор значений закрытый (proto enum Gender), и его
-- проверяет СУБД, а не только модель. Новое значение добавляется отдельной
-- миграцией: ALTER TYPE user_gender ADD VALUE 'other'; — в PostgreSQL ≥12 это
-- можно и внутри транзакции, но использовать значение в той же транзакции нельзя.
-- Время — UTC-моменты (TIMESTAMPTZ): модель работает только с UTC.
-- Дата рождения — DATE: время суток у даты рождения смысла не имеет,
-- поэтому в модели это model.Date, а не time.Time.
-- IF NOT EXISTS не используется: миграция версионирована, а чужая таблица
-- с тем же именем должна ронять старт явной ошибкой, а не подменять схему.
CREATE TYPE user_gender AS ENUM ('unspecified', 'male', 'female');

CREATE TABLE users (
    id            UUID         PRIMARY KEY,
    email         VARCHAR(254) NOT NULL,
    first_name    VARCHAR(100) NOT NULL DEFAULT '',
    last_name     VARCHAR(100) NOT NULL DEFAULT '',
    middle_name   VARCHAR(100) NOT NULL DEFAULT '',
    phone_number  VARCHAR(20)  NOT NULL DEFAULT '',
    gender        user_gender  NOT NULL DEFAULT 'unspecified',
    birth_date    DATE,
    created_at    TIMESTAMPTZ  NOT NULL,
    updated_at    TIMESTAMPTZ  NOT NULL,
    last_login_at TIMESTAMPTZ,
    is_active     BOOLEAN      NOT NULL DEFAULT true,
    is_delete     BOOLEAN      NOT NULL DEFAULT false
);

-- Уникальность email — только среди активных пользователей: удаление здесь
-- soft delete (is_active = false), поэтому у одного адреса может быть одна
-- активная запись и сколько угодно удалённых. Индекс частичный (WHERE is_active):
-- он держит «один активный email» и заодно освобождает адрес для повторной
-- регистрации после удаления, не теряя историю. Отдельный индекс
-- users_email_idx используется вместо UNIQUE на колонке — имя предсказуемо.
CREATE UNIQUE INDEX users_email_idx ON users (email) WHERE is_active = true;

-- Пустая строка в phone_number — это «телефон не задан», а таких пользователей
-- может быть сколько угодно, поэтому обычный UNIQUE-индекс не подходит: он
-- разрешил бы единственную строку с пустым телефоном. Частичный индекс
-- (WHERE phone_number <> '') держит уникальность только для заданных номеров.
CREATE UNIQUE INDEX users_phone_number_idx ON users (phone_number) WHERE phone_number <> '';

-- +goose Down
-- Откат: таблица удаляется вместе с индексами, затем — собственный тип ENUM
-- (в PostgreSQL типы существуют отдельно от таблиц).
DROP TABLE IF EXISTS users;

DROP TYPE IF EXISTS user_gender;
