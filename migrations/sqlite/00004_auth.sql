-- +goose Up
-- Учётные данные и токены доступа (сессии) сервиса BotBooker
-- (ERDB 2.4, 2.5, SQLite-вариант схемы, см. migrations/postgres/00004_auth.sql).
--
-- Отличия от PostgreSQL — только там, где диалекту не хватает возможностей:
--   * id/user_id/tenant_id — TEXT (канонический UUID строкой): типа uuid нет;
--   * provider_meta — TEXT (JSON-строка), в PostgreSQL — JSONB;
--   * время хранится строкой RFC3339 в UTC (формат задаёт репозиторий).
--
-- Внешние ключи с ON DELETE CASCADE работают, потому что драйвер включает
-- PRAGMA foreign_keys: у SQLite проверка внешних ключей по умолчанию выключена
-- (см. internal/database/sqlite).
CREATE TABLE user_credentials (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider      TEXT    NOT NULL,
    provider_key  TEXT    NOT NULL,
    password_hash TEXT    NOT NULL,
    provider_meta TEXT,
    created_at    TEXT    NOT NULL,
    last_login_at TEXT,
    expires_at    TEXT    NOT NULL,
    revoked_at    TEXT,
    is_active     INTEGER NOT NULL DEFAULT 1,
    is_delete     INTEGER NOT NULL DEFAULT 0
);

-- Быстрый поиск учётных данных при логине по паре провайдер + ключ провайдера.
CREATE INDEX idx_credentials_lookup ON user_credentials (provider, provider_key);

CREATE TABLE user_tokens (
    id           TEXT    PRIMARY KEY,
    user_id      TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    tenant_id    TEXT    NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    role_id      INTEGER NOT NULL REFERENCES roles (id),
    issued_at    TEXT    NOT NULL,
    expires_at   TEXT    NOT NULL,
    revoked_at   TEXT,
    last_used_at TEXT,
    CONSTRAINT user_tokens_issued_before_expires_check CHECK (expires_at > issued_at)
);

-- Все сессии пользователя (например, «выйти на всех устройствах»), удаление
-- просроченных записей и выборка сессий тенанта: операции идут по user_id,
-- expires_at и tenant_id.
CREATE INDEX user_tokens_user_id_idx ON user_tokens (user_id);

CREATE INDEX user_tokens_tenant_id_idx ON user_tokens (tenant_id);

CREATE INDEX user_tokens_expires_at_idx ON user_tokens (expires_at);

-- +goose Down
-- Откат: сначала зависимая таблица, затем учётные данные.
DROP TABLE IF EXISTS user_tokens;

DROP TABLE IF EXISTS user_credentials;
