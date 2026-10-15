-- +goose Up
-- Учётные данные и токены доступа (сессии) сервиса BotBooker (ERDB 2.4, 2.5).
--
-- Пароли вынесены из users в отдельную таблицу user_credentials: профиль
-- читается почти каждым запросом, а хеш пароля нужен только при входе. Один
-- профиль может иметь несколько учётных данных — по одному на провайдера
-- ('email', 'telegram', 'google', 'apple', ...); если provider не указан,
-- авторизация идёт по 'email'. Поиск при логине закрыт индексом
-- idx_credentials_lookup (provider, provider_key).
--
-- user_tokens — это список выданных токенов. Сам токен (JWT) подписан и
-- хранится у клиента: подпись проверяется без обращения к базе, но отозвать
-- токен до истечения срока можно только по записи в базе. Поэтому при выходе
-- из системы строка помечается отозванной (revoked_at), а при проверке токена
-- сервис убеждается, что запись существует, не отозвана и не истекла.
--
-- id токена — это claim jti: идентификатор, по которому запись находится.
-- Хеш самого токена не хранится: секрет в JWT не передаётся, а подпись
-- проверяется секретом приложения, поэтому знание строки токена ничего не
-- добавляет к знанию jti.
--
-- Сессия действует в рамках одного тенанта, а роль зависит от тенанта: в записи
-- хранятся tenant_id и role_id — роль в этом тенанте на момент выдачи (для
-- аудита). Действующая роль берётся из tenant_members для активного тенанта;
-- обе ссылки смотрят на один справочник roles.
--
-- IF NOT EXISTS не используется: миграция версионирована, а чужая таблица с тем
-- же именем должна ронять старт явной ошибкой, а не подменять схему.
CREATE TABLE user_credentials (
    id            UUID         PRIMARY KEY,
    user_id       UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider      VARCHAR(20)  NOT NULL,
    provider_key  VARCHAR(254) NOT NULL,
    password_hash VARCHAR(254) NOT NULL,
    provider_meta JSONB,
    created_at    TIMESTAMPTZ  NOT NULL,
    last_login_at TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ  NOT NULL,
    revoked_at    TIMESTAMPTZ,
    is_active     BOOLEAN      NOT NULL DEFAULT true,
    is_delete     BOOLEAN      NOT NULL DEFAULT false
);

-- Быстрый поиск учётных данных при логине по паре провайдер + ключ провайдера.
CREATE INDEX idx_credentials_lookup ON user_credentials (provider, provider_key);

CREATE TABLE user_tokens (
    id           UUID        PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    tenant_id    UUID        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    role_id      SMALLINT    NOT NULL REFERENCES roles (id),
    issued_at    TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
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
