-- +goose Up
-- Пользователи сервиса BotBooker — профиль (ERDB 2.3, SQLite-вариант схемы,
-- см. migrations/postgres/00002_users.sql).
--
-- Отличия от PostgreSQL — только там, где диалекту не хватает возможностей:
--   * id — TEXT (канонический UUID строкой): типа uuid у SQLite нет, формат
--     проверяет модель (model.validateUUID);
--   * gender — TEXT + CHECK: ENUM в SQLite не поддерживается, набор значений
--     задаёт CHECK-ограничение;
--   * время хранится строкой RFC3339 в UTC (у SQLite нет типа timestamp,
--     формат значения задаёт репозиторий), дата рождения — строкой YYYY-MM-DD.
-- Роли у профиля нет: роль хранится в членстве (tenant_members.role_id,
-- миграция 00003). Значение id генерирует приложение (model.NewUser → UUIDv4).
-- IF NOT EXISTS не используется: чужая таблица с тем же именем должна ронять
-- старт явной ошибкой, а не подменять схему.
CREATE TABLE users (
    id            TEXT    PRIMARY KEY,
    email         TEXT    NOT NULL,
    first_name    TEXT    NOT NULL DEFAULT '',
    last_name     TEXT    NOT NULL DEFAULT '',
    middle_name   TEXT    NOT NULL DEFAULT '',
    phone_number  TEXT    NOT NULL DEFAULT '',
    gender        TEXT    NOT NULL DEFAULT 'unspecified'
        CONSTRAINT users_gender_check CHECK (gender IN ('unspecified', 'male', 'female')),
    birth_date    TEXT,
    created_at    TEXT    NOT NULL,
    updated_at    TEXT    NOT NULL,
    last_login_at TEXT,
    is_active     INTEGER NOT NULL DEFAULT 1,
    is_delete     INTEGER NOT NULL DEFAULT 0
);

-- Уникальность email — только среди активных пользователей (WHERE is_active = 1):
-- удаление — soft delete, поэтому адрес может встречаться у одной активной
-- записи и любого числа удалённых. Это разрешает повторную регистрацию после
-- удаления и не теряет историю. См. PostgreSQL-вариант схемы.
CREATE UNIQUE INDEX users_email_idx ON users (email) WHERE is_active = 1;

-- Телефон уникален только для заданных значений: пустая строка означает
-- «телефон не задан», поэтому индекс частичный — иначе пустой телефон
-- мог бы быть лишь у одного пользователя.
CREATE UNIQUE INDEX users_phone_number_idx ON users (phone_number) WHERE phone_number <> '';

-- +goose Down
-- Откат: таблица удаляется вместе с индексами.
DROP TABLE IF EXISTS users;
