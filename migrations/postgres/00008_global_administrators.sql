-- +goose Up
-- Глобальные администраторы (superuser и global operator).
--
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

-- +goose Down
-- Откат: глобальные флаги — обычные колонки users, поэтому удаляются вместе с
-- ними. Данные о назначениях при откате теряются: в ERDB они не хранятся.
ALTER TABLE users DROP COLUMN is_global_operator;

ALTER TABLE users DROP COLUMN is_superuser;
