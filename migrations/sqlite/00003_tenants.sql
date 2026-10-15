-- +goose Up
-- Тенанты и членство пользователей (ERDB 2.1, 2.2, SQLite-вариант схемы,
-- см. migrations/postgres/00003_tenants.sql).
--
-- Отличия от PostgreSQL — только там, где диалекту не хватает возможностей:
--   * id/role_id — TEXT/INTEGER (UUID строкой, типа uuid у SQLite нет);
--   * status — TEXT + CHECK: ENUM в SQLite не поддерживается;
--   * metadata — TEXT (JSON-строка), в PostgreSQL — JSONB;
--   * время хранится строкой RFC3339 в UTC (формат задаёт репозиторий).
CREATE TABLE tenants (
    id         TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL,
    slug       TEXT    NOT NULL,
    status     TEXT    NOT NULL DEFAULT 'active'
        CONSTRAINT tenants_status_check CHECK (status IN ('active', 'suspended', 'deleted')),
    metadata   TEXT,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

CREATE UNIQUE INDEX tenants_slug_idx ON tenants (slug);

CREATE TABLE tenant_members (
    tenant_id TEXT    NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id   TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id   INTEGER NOT NULL DEFAULT 32767 REFERENCES roles (id),
    is_owner  INTEGER NOT NULL DEFAULT 0,
    joined_at TEXT    NOT NULL,
    PRIMARY KEY (tenant_id, user_id)
);

-- «Ровно один владелец у тенанта»: частичный уникальный индекс по is_owner.
CREATE UNIQUE INDEX tenant_members_owner_idx ON tenant_members (tenant_id) WHERE is_owner = 1;

CREATE INDEX tenant_members_user_id_idx ON tenant_members (user_id);

CREATE INDEX tenant_members_role_id_idx ON tenant_members (role_id);

-- +goose Down
-- Откат: сначала связка, затем тенанты.
DROP TABLE IF EXISTS tenant_members;

DROP TABLE IF EXISTS tenants;
