-- +goose Up
-- Уведомления и подписки на них (ERDB 2.20, 2.21).
--
-- notifications — очередь исходящих уведомлений: канал, шаблон, payload и статус
-- обработки. Секреты не хранятся (см. ERDB 2.20). Очередь выбирается частичным
-- индексом notifications_due_idx (next_attempt_at WHERE status = 'pending').
-- Ссылка на бронь необязательна и обнуляется при её удалении (ON DELETE SET NULL).
--
-- notification_subscriptions — подписки пользователя на каналы: PK
-- (user_id, channel, endpoint) не даёт продублировать подписку (ERDB 4.21).
--
-- IF NOT EXISTS не используется: миграция версионирована, а чужая таблица с тем
-- же именем должна ронять старт явной ошибкой, а не подменять схему.
CREATE TYPE notification_channel AS ENUM ('telegram', 'email', 'push');

CREATE TYPE notification_status AS ENUM ('pending', 'processing', 'sent', 'failed');

CREATE TABLE notifications (
    id              UUID                PRIMARY KEY,
    user_id         UUID                NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    booking_id      UUID                REFERENCES bookings (id) ON DELETE SET NULL,
    channel         notification_channel NOT NULL,
    template_code   VARCHAR(64)         NOT NULL,
    payload         JSONB,
    status          notification_status NOT NULL DEFAULT 'pending',
    attempts        SMALLINT            NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ         NOT NULL DEFAULT 'epoch',
    sent_at         TIMESTAMPTZ         NOT NULL DEFAULT 'epoch',
    last_error      TEXT                NOT NULL DEFAULT '',
    dedup_key       VARCHAR(128)        NOT NULL,
    created_at      TIMESTAMPTZ         NOT NULL,
    CONSTRAINT notifications_attempts_check CHECK (attempts >= 0)
);

CREATE UNIQUE INDEX notifications_dedup_key_idx ON notifications (dedup_key);

-- Выборка очереди для ClaimDue: только уведомления, ждущие отправки.
CREATE INDEX notifications_due_idx ON notifications (next_attempt_at) WHERE status = 'pending';

CREATE INDEX notifications_user_created_idx ON notifications (user_id, created_at DESC);

CREATE TABLE notification_subscriptions (
    user_id   UUID                 NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel   notification_channel NOT NULL,
    endpoint  VARCHAR(255)         NOT NULL,
    is_active BOOLEAN              NOT NULL DEFAULT true,
    PRIMARY KEY (user_id, channel, endpoint)
);

-- +goose Down
-- Откат: таблицы, затем собственные типы ENUM (они существуют отдельно от таблиц).
DROP TABLE IF EXISTS notification_subscriptions;

DROP TABLE IF EXISTS notifications;

DROP TYPE IF EXISTS notification_status;

DROP TYPE IF EXISTS notification_channel;
