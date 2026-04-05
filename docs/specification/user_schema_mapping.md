# User Schema Mapping: Telegram Bot API ↔ ERDB

## Шаг 1. Анализ полей

### Telegram Bot API — User

| Поле | Тип | Обязательное | Описание |
|------|-----|--------------|----------|
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
|------|-----|--------------|----------|
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
|-----------------------|-----------|-------------|
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
|-----------|--------------------|
| `user_id` | Дублирует `id` из Telegram API. Telegram ID является каноническим идентификатором. Внутренний ID может храниться отдельно как `user_id_erdb` при необходимости. |
| `full_name` | Дублирует информацию из `first_name` + `last_name` Telegram API. Полное имя может быть вычислено конкатенацией. |
| `password_hash` | Служебное поле аутентификации, не относится к профилю пользователя Telegram. Выносится в отдельную таблицу `UserCredentials`. |

---

## Шаг 4. Итоговая согласованная схема типа User

| Поле | Тип | Обязательное | Описание | Источник | Примечание |
|------|-----|--------------|----------|----------|------------|
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
|------|---------|
| `user_id` (ERDB) | Заменён на `id` из Telegram API как канонический идентификатор; сохранён как `user_id_erdb` для обратной совместимости |
| `full_name` (ERDB) | Заменён на пару `first_name` + `last_name` из Telegram API; полное имя вычисляется |
| `password_hash` (ERDB) | Вынесен в отдельную таблицу `UserCredentials`; не относится к профилю Telegram-пользователя |
