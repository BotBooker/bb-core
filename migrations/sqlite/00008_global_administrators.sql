-- +goose Up
-- Глобальные администраторы (superuser и global operator),
-- SQLite-вариант схемы, см. migrations/postgres/00008_global_administrators.sql.
--
-- Роль в тенанте остаётся атрибутом членства (tenant_members.role_id): колонки
-- ниже задают системный уровень доступа поверх мультитенантной модели.
-- is_superuser выставляет bootstrap первому администратору, is_global_operator —
-- суперпользователь через API. SQLite хранит BOOLEAN как INTEGER (0/1), значение
-- читает database/sql через driver.Bool — так же, как is_active/is_delete.
ALTER TABLE users ADD COLUMN is_superuser INTEGER NOT NULL DEFAULT 0;

ALTER TABLE users ADD COLUMN is_global_operator INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- Откат: глобальные флаги — обычные колонки users. SQLite (3.35+) умеет
-- удалять колонки ALTER TABLE ... DROP COLUMN, но не поддерживает IF EXISTS.
ALTER TABLE users DROP COLUMN is_global_operator;

ALTER TABLE users DROP COLUMN is_superuser;
