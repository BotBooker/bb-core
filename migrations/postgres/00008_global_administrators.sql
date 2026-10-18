-- +goose Up
-- M1: глобальные администраторы (superuser и global operator) и изоляция данных
-- тенантов. Это одна миграция: релиза с промежуточной схемой не было, поэтому
-- обе части M1 применяются одним шагом.
--
-- Глобальные администраторы.
-- Роль в тенанте остаётся атрибутом членства (tenant_members.role_id): колонки
-- ниже не заменяют её, а задают системный уровень доступа поверх мультитенантной
-- модели. is_superuser выставляет bootstrap первому администратору: он управляет
-- всеми объектами и тенантами. is_global_operator назначает суперпользователь
-- через API: такой пользователь администрирует все тенанты, даже если его роль
-- в конкретном членстве ниже admin.
--
-- Флаги хранятся в users, потому что это свойства учётной записи, а не
-- конкретного членства: они одинаковы во всех тенантах и должны быть видны при
-- каждой аутентификации вместе с профилем.
ALTER TABLE users ADD COLUMN is_superuser BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE users ADD COLUMN is_global_operator BOOLEAN NOT NULL DEFAULT false;

-- Изоляция данных тенантов (M1a).
--
-- tenant_id добавляется в бизнес-таблицы, которые принадлежат тенанту:
-- rental_objects, bookings, bookings_journal, payments, notifications.
-- Backfill идёт от владельца объекта/клиента/брони; если у пользователя ещё нет
-- членства, строка относится к самому раннему активному тенанту установки.
-- После backfill колонки становятся NOT NULL.
--
-- Составные FK (tenant_id, object_id) и (tenant_id, booking_id) не позволяют
-- сослаться на объект или бронь другого тенанта даже при ошибке в приложении.
-- Уникальные ключи идемпотентности и dedup_key сделаны tenant-scoped.

ALTER TABLE rental_objects ADD COLUMN tenant_id UUID REFERENCES tenants (id) ON DELETE CASCADE;

-- Владелец объекта: его самое раннее членство (владелец тенанта в приоритете).
UPDATE rental_objects AS o
SET tenant_id = m.tenant_id
FROM (
    SELECT DISTINCT ON (tm.user_id) tm.user_id, tm.tenant_id
    FROM tenant_members AS tm
    JOIN tenants AS t ON t.id = tm.tenant_id
    WHERE t.status <> 'deleted'
    ORDER BY tm.user_id, tm.is_owner DESC, tm.joined_at, tm.tenant_id
) AS m
WHERE o.owner_id = m.user_id
  AND o.tenant_id IS NULL;

-- Объекты без владельца (или владелец без членства) — в самый ранний тенант.
UPDATE rental_objects
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;

ALTER TABLE bookings ADD COLUMN tenant_id UUID REFERENCES tenants (id) ON DELETE CASCADE;

UPDATE bookings AS b
SET tenant_id = o.tenant_id
FROM rental_objects AS o
WHERE b.object_id = o.id
  AND b.tenant_id IS NULL;

UPDATE bookings
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;

ALTER TABLE bookings_journal ADD COLUMN tenant_id UUID REFERENCES tenants (id) ON DELETE CASCADE;

UPDATE bookings_journal AS j
SET tenant_id = b.tenant_id
FROM bookings AS b
WHERE j.booking_id = b.id
  AND j.tenant_id IS NULL;

UPDATE bookings_journal
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;

ALTER TABLE payments ADD COLUMN tenant_id UUID REFERENCES tenants (id) ON DELETE CASCADE;

UPDATE payments AS p
SET tenant_id = b.tenant_id
FROM bookings AS b
WHERE p.booking_id = b.id
  AND p.tenant_id IS NULL;

UPDATE payments
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;

ALTER TABLE notifications ADD COLUMN tenant_id UUID REFERENCES tenants (id) ON DELETE CASCADE;

UPDATE notifications AS n
SET tenant_id = b.tenant_id
FROM bookings AS b
WHERE n.booking_id = b.id
  AND n.tenant_id IS NULL;

UPDATE notifications AS n
SET tenant_id = m.tenant_id
FROM (
    SELECT DISTINCT ON (tm.user_id) tm.user_id, tm.tenant_id
    FROM tenant_members AS tm
    JOIN tenants AS t ON t.id = tm.tenant_id
    WHERE t.status <> 'deleted'
    ORDER BY tm.user_id, tm.is_owner DESC, tm.joined_at, tm.tenant_id
) AS m
WHERE n.user_id = m.user_id
  AND n.tenant_id IS NULL;

UPDATE notifications
SET tenant_id = (
    SELECT id FROM tenants
    WHERE status <> 'deleted'
    ORDER BY created_at, id
    LIMIT 1
)
WHERE tenant_id IS NULL;


-- Явная ошибка вместо «NOT NULL violation»: администратор видит, что данные
-- есть, а тенанта нет, и может создать тенант/назначить владельца.
-- StatementBegin/End обязательны: внутри DO-блока есть «;», а goose по
-- умолчанию режет миграцию на операторы именно по ним.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM rental_objects WHERE tenant_id IS NULL)
        OR EXISTS (SELECT 1 FROM bookings WHERE tenant_id IS NULL)
        OR EXISTS (SELECT 1 FROM bookings_journal WHERE tenant_id IS NULL)
        OR EXISTS (SELECT 1 FROM payments WHERE tenant_id IS NULL)
        OR EXISTS (SELECT 1 FROM notifications WHERE tenant_id IS NULL)
    THEN
        RAISE EXCEPTION 'tenant backfill failed: create a tenant before migrating';
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE rental_objects ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE bookings ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE bookings_journal ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE payments ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE notifications ALTER COLUMN tenant_id SET NOT NULL;

-- Цель составных FK: ссылка на (tenant_id, id).
ALTER TABLE rental_objects ADD CONSTRAINT rental_objects_tenant_id_key UNIQUE (tenant_id, id);
ALTER TABLE bookings ADD CONSTRAINT bookings_tenant_id_key UNIQUE (tenant_id, id);

ALTER TABLE bookings
    ADD CONSTRAINT bookings_tenant_object_fk
    FOREIGN KEY (tenant_id, object_id) REFERENCES rental_objects (tenant_id, id);

ALTER TABLE bookings_journal
    ADD CONSTRAINT bookings_journal_tenant_booking_fk
    FOREIGN KEY (tenant_id, booking_id) REFERENCES bookings (tenant_id, id) ON DELETE CASCADE;

ALTER TABLE payments
    ADD CONSTRAINT payments_tenant_booking_fk
    FOREIGN KEY (tenant_id, booking_id) REFERENCES bookings (tenant_id, id) ON DELETE CASCADE;

-- Уведомление может быть не привязано к брони (booking_id NULL) — MATCH SIMPLE
-- пропускает такую строку; при наличии брони она обязана быть из того же тенанта.
ALTER TABLE notifications
    ADD CONSTRAINT notifications_tenant_booking_fk
    FOREIGN KEY (tenant_id, booking_id) REFERENCES bookings (tenant_id, id) ON DELETE CASCADE;

-- Идемпотентность и dedup — в пределах тенанта, а не установки.
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
-- тенантов (tenant-scoped индексы, составные FK и колонки tenant_id, глобальные
-- уникальные ключи возвращаются), затем — глобальные флаги users.
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

ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_tenant_booking_fk;
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_tenant_booking_fk;
ALTER TABLE bookings_journal DROP CONSTRAINT IF EXISTS bookings_journal_tenant_booking_fk;
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_tenant_object_fk;
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_tenant_id_key;
ALTER TABLE rental_objects DROP CONSTRAINT IF EXISTS rental_objects_tenant_id_key;

ALTER TABLE notifications DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE payments DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE bookings_journal DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE bookings DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE rental_objects DROP COLUMN IF EXISTS tenant_id;

-- Глобальные флаги — обычные колонки users: данные о назначениях при откате
-- теряются, в ERDB они не хранятся.
ALTER TABLE users DROP COLUMN is_global_operator;

ALTER TABLE users DROP COLUMN is_superuser;
