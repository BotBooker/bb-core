-- +goose Up
-- Учётные данные и токены доступа (сессии) сервиса BotBooker
-- (SQLite-вариант схемы, см. migrations/postgres/00003_auth.sql).
--
-- Отличия от PostgreSQL — только там, где диалекту не хватает возможностей:
--   * user_id — TEXT (канонический UUID строкой): типа uuid у SQLite нет;
--   * время хранится строкой RFC3339 в UTC (формат задаёт репозиторий);
--   * role — TEXT + CHECK: ENUM в SQLite не поддерживается.
--
-- Внешние ключи с ON DELETE CASCADE работают, потому что драйвер включает
-- PRAGMA foreign_keys: у SQLite проверка внешних ключей по умолчанию выключена
-- (см. internal/database/sqlite).
CREATE TABLE user_credentials (
    user_id       TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE user_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role         TEXT NOT NULL
        CONSTRAINT user_tokens_role_check CHECK (role IN ('client', 'operator', 'admin')),
    issued_at    TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    revoked_at   TEXT,
    last_used_at TEXT,
    CONSTRAINT user_tokens_issued_before_expires_check CHECK (expires_at > issued_at)
);

-- Все сессии пользователя (например, «выйти на всех устройствах») и удаление
-- просроченных записей: обе операции идут по user_id и по expires_at.
CREATE INDEX user_tokens_user_id_idx ON user_tokens (user_id);

CREATE INDEX user_tokens_expires_at_idx ON user_tokens (expires_at);

-- +goose Down
-- Откат: сначала зависимая таблица, затем учётные данные.
DROP TABLE IF EXISTS user_tokens;

DROP TABLE IF EXISTS user_credentials;
