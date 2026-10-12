# User Schema Mapping: Telegram Bot API ↔ ERDB

## Шаг 1. Анализ полей

### Telegram Bot API — User

| Поле | Тип | Обязательное | Описание |
| ---- | --- | ------------ | -------- |
| `id` | Integer | Да | Уникальный идентификатор пользователя или бота (до 52 значащих бит) |
| `is_bot` | Boolean | Да | `True`, если пользователь — бот |
| `first_name` | String | Да | Имя пользователя или бота |
| `last_name` | String | Нет | Фамилия пользователя или бота |
| `username` | String | Нет | Юзернейм пользователя или бота |
| `language_code` | String | Нет | IETF language tag языка пользователя |
| `is_premium` | True | Нет | `True`, если пользователь — Telegram Premium |
| `added_to_attachment_menu` | True | Нет | `True`, если пользователь добавил бота в меню вложений |
| `can_join_groups` | Boolean | Нет | `True`, если бот может быть добавлен в группы (только getMe) |
| `can_read_all_group_messages` | Boolean | Нет | `True`, если privacy mode отключён (только getMe) |
| `supports_inline_queries` | Boolean | Нет | `True`, если бот поддерживает inline-запросы (только getMe) |
| `can_connect_to_business` | Boolean | Нет | `True`, если бот может быть подключён к Telegram Business (только getMe) |
| `has_main_web_app` | Boolean | Нет | `True`, если у бота есть главное Web App (только getMe) |
| `has_topics_enabled` | Boolean | Нет | `True`, если у бота включён режим тем в личных чатах (только getMe) |

### ERDB — User

| Поле | Тип | Обязательное | Описание |
| ---- | --- | ------------ | -------- |
| `user_id` | INT (PK, автоинкремент) | Да | Внутренний первичный ключ пользователя |
| `full_name` | VARCHAR(100) | Да | Полное имя пользователя |
| `email` | VARCHAR(255), UNIQUE | Да | Электронная почта |
| `phone` | VARCHAR(20) | Да | Номер телефона |
| `password_hash` | VARCHAR(255) | Да | Хеш пароля (bcrypt) |
| `role` | ENUM('client','operator','admin') | Да | Роль пользователя в системе |
| `created_at` | TIMESTAMP | Да | Дата и время создания записи |
| `last_login` | TIMESTAMP | Нет | Дата и время последнего входа |

---

## Шаг 2. Таблица сопоставления

| Telegram Bot API поле | ERDB поле | Комментарий |
| --------------------- | --------- | ----------- |
| `id` | `user_id` | Аналог по смыслу, но разные значения: Telegram ID vs внутренний PK. Тип отличается (64-bit Integer vs INT) |
| `is_bot` | — | Нет аналога в ERDB |
| `first_name` | — | Нет прямого аналога; в ERDB `full_name` — единое поле |
| `last_name` | — | Нет аналога в ERDB |
| `username` | — | Нет аналога в ERDB |
| `language_code` | — | Нет аналога в ERDB |
| `is_premium` | — | Нет аналога в ERDB |
| `added_to_attachment_menu` | — | Нет аналога в ERDB |
| `can_join_groups` | — | Нет аналога в ERDB (только для ботов, getMe) |
| `can_read_all_group_messages` | — | Нет аналога в ERDB (только для ботов, getMe) |
| `supports_inline_queries` | — | Нет аналога в ERDB (только для ботов, getMe) |
| `can_connect_to_business` | — | Нет аналога в ERDB (только для ботов, getMe) |
| `has_main_web_app` | — | Нет аналога в ERDB (только для ботов, getMe) |
| `has_topics_enabled` | — | Нет аналога в ERDB (только для ботов, getMe) |
| — | `full_name` | В ERDB единое поле; в Telegram API разделено на `first_name` + `last_name` |
| — | `email` | Нет аналога в Telegram API; кастомное поле сервиса |
| — | `phone` | Нет аналога в Telegram API; кастомное поле сервиса |
| — | `password_hash` | Нет аналога в Telegram API; служебное поле аутентификации |
| — | `role` | Нет аналога в Telegram API; кастомное поле ролевой модели |
| — | `created_at` | Нет аналога в Telegram API; служебное поле |
| — | `last_login` | Нет аналога в Telegram API; служебное поле |

---

## Шаг 3. Согласование — единая схема

**Принципы:**

- Структура следует Telegram Bot API (внешний контракт).
- Поля из ERDB, которых нет в Telegram API, добавляются с атрибутом `custom: true`.
- Исключены поля ERDB, которые дублируют Telegram API или не относятся к сущности пользователя Telegram.

### Исключённые поля ERDB и причины

| Поле ERDB | Причина исключения |
| ----------- | -------------------- |
| `user_id` | Дублирует `id` из Telegram API. Telegram ID является каноническим идентификатором. Внутренний ID может храниться отдельно как `user_id_erdb` при необходимости. |
| `full_name` | Дублирует информацию из `first_name` + `last_name` Telegram API. Полное имя может быть вычислено конкатенацией. |
| `password_hash` | Служебное поле аутентификации, не относится к профилю пользователя Telegram. Выносится в отдельную таблицу `UserCredentials`. |

---

## Шаг 4. Итоговая согласованная схема типа User

| Поле | Тип | Обязательное | Описание | Источник | Примечание |
| ---- | --- | ------------ | -------- | -------- | ---------- |
| `id` | Integer (64-bit) | Да | Уникальный идентификатор пользователя Telegram | Telegram API | Основной PK; соответствует `tg_user_id` |
| `is_bot` | Boolean | Да | `True`, если пользователь — бот | Telegram API | — |
| `first_name` | String | Да | Имя пользователя или бота | Telegram API | — |
| `last_name` | String | Нет | Фамилия пользователя или бота | Telegram API | — |
| `username` | String | Нет | Юзернейм пользователя или бота | Telegram API | — |
| `language_code` | String | Нет | IETF language tag языка пользователя | Telegram API | — |
| `is_premium` | Boolean | Нет | `True`, если пользователь — Telegram Premium | Telegram API | Тип `True` нормализован к `Boolean` |
| `added_to_attachment_menu` | Boolean | Нет | `True`, если бот добавлен в меню вложений | Telegram API | Тип `True` нормализован к `Boolean` |
| `can_join_groups` | Boolean | Нет | `True`, если бот может быть добавлен в группы | Telegram API | Возвращается только в getMe |
| `can_read_all_group_messages` | Boolean | Нет | `True`, если privacy mode отключён | Telegram API | Возвращается только в getMe |
| `supports_inline_queries` | Boolean | Нет | `True`, если бот поддерживает inline-запросы | Telegram API | Возвращается только в getMe |
| `can_connect_to_business` | Boolean | Нет | `True`, если бот может быть подключён к Telegram Business | Telegram API | Возвращается только в getMe |
| `has_main_web_app` | Boolean | Нет | `True`, если у бота есть главное Web App | Telegram API | Возвращается только в getMe |
| `has_topics_enabled` | Boolean | Нет | `True`, если включён режим тем в личных чатах | Telegram API | Возвращается только в getMe |
| `email` | String | Нет | Электронная почта пользователя | ERDB | `custom: true`; UNIQUE |
| `phone` | String | Нет | Номер телефона пользователя | ERDB | `custom: true` |
| `role` | String (ENUM) | Да | Роль: `client`, `operator`, `admin` | ERDB | `custom: true`; default: `client` |
| `created_at` | Timestamp | Да | Дата и время создания записи | ERDB | `custom: true`; default: NOW() |
| `last_login` | Timestamp | Нет | Дата и время последнего входа | ERDB | `custom: true` |
| `user_id_erdb` | Integer | Нет | Внутренний ID из legacy-системы | ERDB | `custom: true`; преобразовано из `user_id` |

---

## Итоговый список исключённых полей

| Поле | Причина |
| ---- | ------- |
| `user_id` (ERDB) | Заменён на `id` из Telegram API как канонический идентификатор; сохранён как `user_id_erdb` для обратной совместимости |
| `full_name` (ERDB) | Заменён на пару `first_name` + `last_name` из Telegram API; полное имя вычисляется |
| `password_hash` (ERDB) | Вынесен в отдельную таблицу `UserCredentials`; не относится к профилю Telegram-пользователя |

---

## Шаг 5. Реализованная модель (`internal/model`)

Код повторяет согласованную схему из Шага 4, но без Telegram-специфичных полей:
сервис идентифицирует пользователя по `id` (UUID, в proto v1 — поле `uuid`),
а Telegram-поля (если понадобятся) живут в интеграционной таблице.
Внешний контракт — proto v1 (`proto/user/v1/user.proto`),
конвертация модель ↔ proto — в `internal/serverapi/user_mapping.go`.

| Поле модели | Тип в Go | Proto v1 | Колонка БД (план) | Примечание |
| ----------- | -------- | -------- | ----------------- | ---------- |
| `ID` | `string` | `uuid` | `id` — PRIMARY KEY (`UUID` в PG, `TEXT` в SQLite) | канонический UUIDv4, генерирует приложение (`model.NewUser`) |
| `Email` | `string` | `email` | NOT NULL + частичный уникальный индекс `users_email_idx` (`WHERE is_active`) | обязателен, до 254 байт, локальная часть до 64 байт; адрес уникален только среди активных записей, удалённые его не занимают |
| `FirstName`, `LastName` | `string` | `first_name`, `last_name` | NOT NULL | 1..100 рун |
| `MiddleName` | `string` | `middle_name` | NOT NULL DEFAULT `''` | до 100 рун, допускается пустое |
| `PhoneNumber` | `string` | `phone_number` | NOT NULL DEFAULT `''` + частичный уникальный индекс `users_phone_number_idx` | E.164; необязателен (пусто или E.164); непустые значения уникальны |
| `Gender` | `Gender` | `gender` (enum) | NOT NULL DEFAULT `'unspecified'` | ENUM `user_gender` в PG, `CHECK` в SQLite: `unspecified` / `male` / `female` |
| `BirthDate` | `*model.Date` | `birthday` | NULL (`DATE` в PG, `TEXT` `YYYY-MM-DD` в SQLite) | календарная дата без времени; не может быть в будущем. Имя в proto отличается от модели и колонки |
| `Role` | `Role` | — | NOT NULL DEFAULT `'client'` | служебное поле (ERDB); API роль не задаёт |
| `CreatedAt`, `UpdatedAt` | `time.Time` | `created_at`, `updated_at` | NOT NULL | всегда в UTC |
| `LastLoginAt` | `*time.Time` | — | NULL | служебное поле (ERDB `last_login`) |
| `IsActive` | `bool` | — | NOT NULL DEFAULT `true` (`BOOLEAN` в PG, `INTEGER` 0/1 в SQLite) | служебный признак soft delete: удаление — это `is_active = false`, а не физический `DELETE` |

### Расхождения с исходными документами

| Что | Почему |
| --- | ------ |
| `full_name` (ERDB) | не хранится: складывается из фамилии, имени и отчества (`User.FullName()`) |
| `password_hash` (ERDB) | не относится к профилю: по Шагу 3 выносится в отдельную таблицу `UserCredentials` |
| Telegram-поля (`id`, `is_bot`, `username`, `language_code`, `is_premium`, …) | нет в proto v1: канонический идентификатор — UUID, Telegram id при необходимости хранится в интеграционной таблице |
| `user_id` (ERDB, INT PK) | первичный ключ — `users.id` типа `uuid`: идентификатор один и он же внешний контракт, числовой суррогат не заводится. Если понадобятся целочисленные FK «по стилю ERDB», служебную колонку `user_id BIGSERIAL NOT NULL UNIQUE` можно добавить отдельной миграцией |
| Обязательность телефона | телефон необязателен: в proto у правила `pattern` выставлен `ignore_empty`, поэтому пустая строка допустима при создании и очищает поле при обновлении |
| Уникальность телефона | уникальность держит частичный индекс (`WHERE phone_number <> ''`): непустые номера не повторяются, а пользователей без телефона может быть сколько угодно. Обычный `UNIQUE` разрешил бы единственную строку с пустым телефоном. Нарушение уникальности СУБД (PG `23505`, SQLite `2067`) при реализации репозитория переводится в доменную ошибку дубликата |
| Тип даты рождения | в proto дата — `google.type.Date` (год, месяц, день без времени и пояса), в модели и БД — календарная дата (`model.Date`, `DATE`/`YYYY-MM-DD`): `Timestamp` для даты рождения не подходит, потому что он всегда про момент времени, а `google.type.Date` допускает и частичную дату (например, только месяц и день, год 0 — если год рождения скрыт). Пустой объект (все компоненты нулевые) означает «дата не задана» и превращается в `nil`, неполная дата вроде «только год» отклоняется валидацией модели |
| `gender` — ENUM | в модели это Go-тип `model.Gender`, в PostgreSQL — нативный тип `user_gender`, в SQLite — `TEXT` + `CHECK` (ENUM в SQLite нет). Значение вне набора отклоняет СУБД (в PG — `SQLSTATE 22P02`), а не только модель |
| `role` — не ENUM | набор ролей служебный и может расширяться административно, поэтому в обоих диалектах это `VARCHAR(20)` + `CHECK`; при необходимости роль переводится в ENUM тем же способом, что и `gender` |

---

## Шаг 6. Миграции

Схема из Шага 5 описана goose-миграциями:

| Диалект | Файл | Отличия от другого диалекта |
|---------|------|-----------------------------|
| PostgreSQL | `migrations/postgres/00002_users.sql` | `id UUID` (PK), `gender user_gender` (нативный ENUM), время — `TIMESTAMPTZ`, дата рождения — `DATE` |
| SQLite | `migrations/sqlite/00002_users.sql` | `id TEXT` (PK), `gender TEXT` + `CHECK`, время — строка RFC3339, дата рождения — `TEXT` `YYYY-MM-DD` |

Решения и их причины:

- **один файл — одна версия.** Goose считает каждый `.sql` отдельной миграцией
  и падает на дубликате номера, поэтому пары `00002_users.up.sql` +
  `00002_users.down.sql` не подходят: секции `-- +goose Up` и `-- +goose Down`
  живут в одном файле (`00001_foundation.up/down.sql` переведены в тот же вид);
- **`id` — нативный `uuid` и первичный ключ** (в SQLite — `TEXT`): 16 байт вместо
  36, формат проверяет сама СУБД (`22P02` на мусор), а отдельная колонка `uuid`
  больше не нужна — идентификатор один. Значение генерирует приложение
  (`model.NewUser` → UUIDv4), `DEFAULT` нет: в SQLite генерации UUID нет, а
  диалекты должны получать идентификатор одинаково. Через `database/sql`-фасад
  (`internal/database/postgres`) Go-строка биндится и читается напрямую — проверено
  отдельным зондом; при прямом использовании pgx (без `database/sql`) понадобится
  `pgtype.UUID`, потому что `UUIDCodec` принимает только `UUIDValuer`;
- **`gender` — ENUM в PostgreSQL** (`CREATE TYPE user_gender AS ENUM (...)`) и
  `TEXT` + `CHECK` в SQLite (ENUM там нет). Модель уже описывает набор типом
  `model.Gender`, ENUM добавляет проверку на стороне БД. Откат (`-- +goose Down`)
  удаляет и тип: `DROP TYPE IF EXISTS user_gender`. Новое значение добавляется
  миграцией `ALTER TYPE user_gender ADD VALUE ...` — в PostgreSQL ≥12 это
  допустимо внутри транзакции, но использовать значение в той же транзакции нельзя;
- **`role` остаётся `CHECK`**: набор служебный и может расширяться;
- **`email` — частичный уникальный индекс `users_email_idx`** вместо `UNIQUE` на
  колонке: гарантия та же (один активный адрес), но имя индекса предсказуемо
  (в PostgreSQL ограничение создавало индекс `users_email_key`, в SQLite —
  автоиндекс). Индекс частичный (`WHERE is_active`), потому что удаление — это
  soft delete: у адреса может быть одна активная запись и сколько угодно
  удалённых, поэтому после удаления адрес снова свободен для регистрации, а
  история сохраняется;
- **`phone_number` — частичный уникальный индекс**: телефон необязателен, пустая
  строка означает «не задан», поэтому уникальность накладывается условием
  `WHERE phone_number <> ''` — непустые номера не повторяются, а пользователей без
  телефона может быть сколько угодно. Обычный `UNIQUE` (в том числе `UNIQUE` на
  колонке) разрешил бы только одну запись с пустым телефоном. Индекс частичный в
  обоих диалектах: `CREATE UNIQUE INDEX users_phone_number_idx ON users (phone_number)
  WHERE phone_number <> ''`;
- **`phone_number` — `NOT NULL DEFAULT ''`**: пустая строка означает
  «телефон не задан», что совпадает с необязательностью поля в API;
- **`birth_date` — `DATE`** (в SQLite — `TEXT` `YYYY-MM-DD`): у даты рождения
  нет времени суток, в модели это `model.Date`, а не `time.Time`;
- **`full_name` не хранится**: вычисляется в `User.FullName()`;
- применение миграций — при старте приложения (`MIGRATE_ON_START`, по умолчанию
  включено), код — `internal/database/migrate.go`, каталоги берутся из
  `migrations.FS`; ручная раскатка и откат — `make migrate-up` / `make migrate-down`
  (см. `docs/HOWTO.md`).
