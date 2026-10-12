-- +goose Up
-- Учётные данные и токены доступа (сессии) сервиса BotBooker.
--
-- Пароли вынесены из users в отдельную таблицу user_credentials — Шаг 3
-- маппинга (docs/specification/user_schema_mapping.md): профиль читается почти
-- каждым запросом, а хеш пароля нужен только при входе. Здесь же лежит и
-- реализация «выхода из системы».
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
-- role хранится на момент выдачи — для аудита и на случай, если роль
-- пользователя поменяется: действующая роль берётся из users, а не из токена.
CREATE TABLE user_credentials (
    user_id       UUID         PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    password_hash VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL,
    updated_at    TIMESTAMPTZ  NOT NULL
);

CREATE TABLE user_tokens (
    id           UUID        PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role         VARCHAR(20) NOT NULL
        CONSTRAINT user_tokens_role_check CHECK (role IN ('client', 'operator', 'admin')),
    issued_at    TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
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
