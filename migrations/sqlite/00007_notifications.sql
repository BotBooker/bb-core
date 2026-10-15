-- +goose Up
-- Уведомления и подписки на них (ERDB 2.20, 2.21, SQLite-вариант схемы,
-- см. migrations/postgres/00007_notifications.sql).
--
-- Отличия от PostgreSQL — только там, где диалекту не хватает возможностей:
--   * channel/status — TEXT + CHECK: ENUM в SQLite не поддерживается;
--   * payload — TEXT (JSON-строка), в PostgreSQL — JSONB;
--   * время хранится строкой RFC3339 в UTC (формат задаёт репозиторий).
CREATE TABLE notifications (
    id              TEXT    PRIMARY KEY,
    user_id         TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    booking_id      TEXT    REFERENCES bookings (id) ON DELETE SET NULL,
    channel         TEXT    NOT NULL
        CONSTRAINT notifications_channel_check CHECK (channel IN ('telegram', 'email', 'push')),
    template_code   TEXT    NOT NULL,
    payload         TEXT,
    status          TEXT    NOT NULL DEFAULT 'pending'
        CONSTRAINT notifications_status_check
            CHECK (status IN ('pending', 'processing', 'sent', 'failed')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT    NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    sent_at         TEXT    NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    last_error      TEXT    NOT NULL DEFAULT '',
    dedup_key       TEXT    NOT NULL,
    created_at      TEXT    NOT NULL,
    CONSTRAINT notifications_attempts_check CHECK (attempts >= 0)
);

CREATE UNIQUE INDEX notifications_dedup_key_idx ON notifications (dedup_key);

-- Выборка очереди для ClaimDue: только уведомления, ждущие отправки.
CREATE INDEX notifications_due_idx ON notifications (next_attempt_at) WHERE status = 'pending';

CREATE INDEX notifications_user_created_idx ON notifications (user_id, created_at DESC);

CREATE TABLE notification_subscriptions (
    user_id   TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel   TEXT    NOT NULL
        CONSTRAINT notification_subscriptions_channel_check
            CHECK (channel IN ('telegram', 'email', 'push')),
    endpoint  TEXT    NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, channel, endpoint)
);

-- +goose Down
-- Откат: сначала подписки, затем уведомления.
DROP TABLE IF EXISTS notification_subscriptions;

DROP TABLE IF EXISTS notifications;
