-- +goose Up
-- M1: глобальные администраторы (superuser и global operator) и изоляция данных
-- тенантов, SQLite-вариант схемы, см. migrations/postgres/00008_global_administrators.sql.
-- Это одна миграция: релиза с промежуточной схемой не было, поэтому обе части M1
-- применяются одним шагом.
--
-- Глобальные администраторы.
-- Роль в тенанте остаётся атрибутом членства (tenant_members.role_id): колонки
-- ниже задают системный уровень доступа поверх мультитенантной модели.
-- is_superuser выставляет bootstrap первому администратору, is_global_operator —
-- суперпользователь через API. SQLite хранит BOOLEAN как INTEGER (0/1), значение
-- читает database/sql через driver.Bool — так же, как is_active/is_delete.
ALTER TABLE users ADD COLUMN is_superuser INTEGER NOT NULL DEFAULT 0;

ALTER TABLE users ADD COLUMN is_global_operator INTEGER NOT NULL DEFAULT 0;

-- Изоляция данных тенантов (M1a).
--
-- SQLite не умеет добавлять NOT NULL-колонку и составной FK к существующей
-- таблице без пересоздания. Здесь tenant_id добавляется nullable, backfill
-- выполняется, а NOT NULL и ссылочная целостность tenant-scoped связей
-- обеспечиваются репозиториями. На чистой базе схема создаётся этими же
-- ALTER-ами до появления данных, поэтому NULL в tenant_id не остаётся.
--
-- Индексы и tenant-scoped уникальные ключи повторяют PostgreSQL.

ALTER TABLE rental_objects ADD COLUMN tenant_id TEXT REFERENCES tenants (id) ON DELETE CASCADE;

-- Владелец объекта: его самое раннее членство (владелец тенанта в приоритете).
UPDATE rental_objects
SET tenant_id = (
    SELECT tm.tenant_id
    FROM tenant_members AS tm
    JOIN tenants AS t ON t.id = tm.tenant_id
    WHERE tm.user_id = rental_objects.owner_id
      AND t.status <> 'deleted'
    ORDER BY tm.is_owner DESC, tm.joined_at, tm.tenant_id
    LIMIT 1
)
WHERE tenant_id IS NULL;

UPDATE rental_objects
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;

ALTER TABLE bookings ADD COLUMN tenant_id TEXT REFERENCES tenants (id) ON DELETE CASCADE;

UPDATE bookings
SET tenant_id = (
    SELECT o.tenant_id FROM rental_objects AS o WHERE o.id = bookings.object_id
)
WHERE tenant_id IS NULL;

UPDATE bookings
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;

ALTER TABLE bookings_journal ADD COLUMN tenant_id TEXT REFERENCES tenants (id) ON DELETE CASCADE;

UPDATE bookings_journal
SET tenant_id = (
    SELECT b.tenant_id FROM bookings AS b WHERE b.id = bookings_journal.booking_id
)
WHERE tenant_id IS NULL;

UPDATE bookings_journal
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;

ALTER TABLE payments ADD COLUMN tenant_id TEXT REFERENCES tenants (id) ON DELETE CASCADE;

UPDATE payments
SET tenant_id = (
    SELECT b.tenant_id FROM bookings AS b WHERE b.id = payments.booking_id
)
WHERE tenant_id IS NULL;

UPDATE payments
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;

ALTER TABLE notifications ADD COLUMN tenant_id TEXT REFERENCES tenants (id) ON DELETE CASCADE;

UPDATE notifications
SET tenant_id = (
    SELECT b.tenant_id FROM bookings AS b WHERE b.id = notifications.booking_id
)
WHERE booking_id IS NOT NULL
  AND tenant_id IS NULL;

UPDATE notifications
SET tenant_id = (
    SELECT tm.tenant_id
    FROM tenant_members AS tm
    JOIN tenants AS t ON t.id = tm.tenant_id
    WHERE tm.user_id = notifications.user_id
      AND t.status <> 'deleted'
    ORDER BY tm.is_owner DESC, tm.joined_at, tm.tenant_id
    LIMIT 1
)
WHERE tenant_id IS NULL;

UPDATE notifications
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;

DROP INDEX IF EXISTS bookings_idempotency_idx;
CREATE UNIQUE INDEX bookings_idempotency_idx ON bookings (tenant_id, client_id, idempotency_key);

DROP INDEX IF EXISTS notifications_dedup_key_idx;
CREATE UNIQUE INDEX notifications_dedup_key_idx ON notifications (tenant_id, dedup_key);

CREATE INDEX rental_objects_tenant_idx ON rental_objects (tenant_id);
CREATE INDEX bookings_tenant_client_slot_idx ON bookings (tenant_id, client_id, slot_start DESC);
CREATE INDEX bookings_tenant_object_start_idx ON bookings (tenant_id, object_id, slot_start);
CREATE INDEX bookings_journal_tenant_idx ON bookings_journal (tenant_id);
CREATE INDEX payments_tenant_idx ON payments (tenant_id);
CREATE INDEX notifications_tenant_created_idx ON notifications (tenant_id, created_at DESC);

-- +goose Down
-- Откат идёт в порядке, обратном применению: сначала снимается изоляция
-- тенантов (tenant-scoped индексы и колонки tenant_id, глобальные уникальные
-- ключи возвращаются), затем — глобальные флаги users.
DROP INDEX IF EXISTS notifications_tenant_created_idx;
DROP INDEX IF EXISTS payments_tenant_idx;
DROP INDEX IF EXISTS bookings_journal_tenant_idx;
DROP INDEX IF EXISTS bookings_tenant_object_start_idx;
DROP INDEX IF EXISTS bookings_tenant_client_slot_idx;
DROP INDEX IF EXISTS rental_objects_tenant_idx;

DROP INDEX IF EXISTS notifications_dedup_key_idx;
CREATE UNIQUE INDEX notifications_dedup_key_idx ON notifications (dedup_key);

DROP INDEX IF EXISTS bookings_idempotency_idx;
CREATE UNIQUE INDEX bookings_idempotency_idx ON bookings (client_id, idempotency_key);

ALTER TABLE notifications DROP COLUMN tenant_id;
ALTER TABLE payments DROP COLUMN tenant_id;
ALTER TABLE bookings_journal DROP COLUMN tenant_id;
ALTER TABLE bookings DROP COLUMN tenant_id;
ALTER TABLE rental_objects DROP COLUMN tenant_id;

-- Глобальные флаги — обычные колонки users. SQLite (3.35+) умеет удалять
-- колонки ALTER TABLE ... DROP COLUMN, но не поддерживает IF EXISTS.
ALTER TABLE users DROP COLUMN is_global_operator;

ALTER TABLE users DROP COLUMN is_superuser;
