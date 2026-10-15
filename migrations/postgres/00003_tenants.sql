-- +goose Up
-- Тенанты и членство пользователей (ERDB 2.1, 2.2).
--
-- Тенант — организация-владелец; контракт tenant.v1. Отдельного поля «владелец»
-- у тенанта нет: владелец — участник с is_owner = true (ровно один на тенант,
-- держит частичный уникальный индекс tenant_members_owner_idx).
--
-- Мультитенантность реализована связкой M:N tenant_members: тенант объединяет
-- группу пользователей, а один пользователь может состоять в разных тенантах.
-- Роль — атрибут членства (role_id, FK -> roles.id): в разных тенантах у одной
-- учётной записи роли независимы, поэтому в users роли нет.
--
-- IF NOT EXISTS не используется: миграция версионирована, а чужая таблица с тем
-- же именем должна ронять старт явной ошибкой, а не подменять схему.
CREATE TYPE tenant_status AS ENUM ('active', 'suspended', 'deleted');

CREATE TABLE tenants (
    id         UUID          PRIMARY KEY,
    name       VARCHAR(255)  NOT NULL,
    slug       VARCHAR(255)  NOT NULL,
    status     tenant_status NOT NULL DEFAULT 'active',
    metadata   JSONB,
    created_at TIMESTAMPTZ   NOT NULL,
    updated_at TIMESTAMPTZ   NOT NULL
);

CREATE UNIQUE INDEX tenants_slug_idx ON tenants (slug);

-- Составной PK (tenant_id, user_id) запрещает добавить пользователя в тенант
-- дважды. role_id по умолчанию — client (32767): новое членство даёт базовую роль.
CREATE TABLE tenant_members (
    tenant_id UUID        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id   UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id   SMALLINT    NOT NULL DEFAULT 32767 REFERENCES roles (id),
    is_owner  BOOLEAN     NOT NULL DEFAULT false,
    joined_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, user_id)
);

-- «Ровно один владелец у тенанта»: частичный уникальный индекс по is_owner.
CREATE UNIQUE INDEX tenant_members_owner_idx ON tenant_members (tenant_id) WHERE is_owner = true;

-- «В каких тенантах состоит пользователь» и «участники с заданной ролью».
CREATE INDEX tenant_members_user_id_idx ON tenant_members (user_id);

CREATE INDEX tenant_members_role_id_idx ON tenant_members (role_id);

-- +goose Down
-- Откат: сначала связка (ссылается на tenants, users, roles), затем тенанты,
-- потом собственный тип ENUM.
DROP TABLE IF EXISTS tenant_members;

DROP TABLE IF EXISTS tenants;

DROP TYPE IF EXISTS tenant_status;
