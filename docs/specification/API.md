# Спецификация API сервиса автоматизированного бронирования

## 1. Общие сведения

- **Базовый URL**: `https://api.booking-service.com/v1`
- **Формат данных**: JSON (Content‑Type: `application/json`)
- **Аутентификация**: JWT‑токен в заголовке `Authorization: Bearer <token>`
- **Кодировка**: UTF‑8
- **Версионирование**: через URL (`/v1`, `/v2`)
- **Идентификаторы**: `UUID` строкой, значение генерирует приложение;
  целочисленный PK (`INT`/`SMALLINT`) — только у справочников (`type_objects`,
  `locations`, `statuses`, `booking_statuses`, `currencies`, `payment_methods`,
  `roles`; см. `ERDB.md`). На границе API значения справочников передаются
  **строковым кодом** (`code`: `RUB`, `pending`, `card`), а в БД хранится
  внешний ключ (`*_id`).
- **Деньги**: `BIGINT` в минорных единицах (копейки) вместе с валютой; в API —
  код ISO‑4217 (`currency`, например `RUB`), в БД — ссылка на справочник
  `currencies` (`currency_id`). `float`/`DECIMAL` не используются.
- **Коды ответов**: успех — `200 OK` с телом ресурса; `201`/`204` не
  используются, потому что HTTP (grpc-gateway) и gRPC обслуживает один адаптер.
- **Источник истины схемы**: `docs/specification/ERDB.md`.
- **Имена на границе API**: поля запросов/ответов задаются в `proto/`; часть имён
  в контракте отличается от колонок БД и указывается здесь явно
  (например, `start_at`/`end_at` в API ↔ `slot_start`/`slot_end` в `bookings`).

## 2. Методы аутентификации и авторизации

### 2.1. Регистрация пользователя

- **Endpoint**: `POST /auth/register`
- **Тело запроса**:

  ```json
  {
    "email": "user@example.com",
    "password": "secure_password",
    "first_name": "Иван",
    "last_name": "Иванов",
    "middle_name": "Иванович",
    "phone_number": "+79991234567"
  }
  ```

- **Ответ (200 OK)** — профиль и выданная сессия:

  ```json
  {
    "user": {
      "uuid": "7c3ca7c0-ec6f-42d5-a77e-6ca55c037ae3",
      "email": "user@example.com",
      "first_name": "Иван",
      "last_name": "Иванов",
      "middle_name": "Иванович",
      "phone_number": "+79991234567"
    },
    "session": {
      "token": "eyJ...abc123",
      "expires_at": "2025-12-08T10:00:00Z"
    }
  }
  ```

### 2.2. Вход в систему

- **Endpoint**: `POST /auth/login`
- **Тело запроса**:

  ```json
  {
    "email": "user@example.com",
    "password": "secure_password"
  }
  ```

- **Ответ (200 OK)**:

  ```json
  {
    "session": {
      "token": "eyJ...abc123",
      "expires_at": "2025-12-08T10:00:00Z"
    }
  }
  ```

### 2.3. Восстановление пароля

- **Endpoint**: `POST /auth/reset-password`
- **Тело запроса**:

  ```json
  {
    "email": "user@example.com"
  }
  ```

- **Ответ (200 OK)**: `{}` (письмо отправлено)

### 2.4. Глобальные операторы

Суперпользователь (создаётся bootstrap, `is_superuser = true`) может назначать
глобальных операторов. Глобальный оператор (`is_global_operator = true`)
администрирует все тенанты независимо от роли в членстве. Обе операции доступны
только суперпользователю и идемпотентны; изменить самого суперпользователя
нельзя.

#### 2.4.1. Назначить глобального оператора

- **Endpoint**: `POST /users/{uuid}/global-operator`
- **Тело запроса**: пустое
- **Ответ `200 OK`**:

  ```json
  {
    "user": {
      "uuid": "0f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f",
      "email": "operator@example.com",
      "isSuperuser": false,
      "isGlobalOperator": true
    }
  }
  ```

- **Ошибки**: `400` — невалидный UUID; `401` — нет действующего токена;
  `403` — вызывающий не суперпользователь; `404` — пользователь не найден.

#### 2.4.2. Снять права глобального оператора

- **Endpoint**: `DELETE /users/{uuid}/global-operator`
- **Ответ `200 OK`**: `{}` (google.protobuf.Empty)
- **Ошибки**: те же, что у назначения; попытка изменить суперпользователя даёт
  `403`.

## 3. Управление объектами аренды

### 3.1. Получение списка объектов

- **Endpoint**: `GET /objects`
- **Параметры запроса (query)**:
  + `type` (INT — ссылка на `type_objects.id`, например `1` — транспорт, `2` — недвижимость)
  + `subtype` (строка, например `автомобиль`, `квартира`)
  + `location_id` (INT — ссылка на `locations.id`)
  + `status` (строка — `statuses.code`: `active`, `under_repair`, `archived`;
    `deleted` — внутреннее состояние и в выдаче не встречается)
  + `min_price_minor`, `max_price_minor` (целое число в минорных единицах)
  + `page_size`, `page_token` (keyset-пагинация)
- **Ответ (200 OK)**:

  ```json
  {
    "items": [
      {
        "uuid": "0f2d9c1e-3b6a-4c8d-9e10-2a3b4c5d6e7f",
        "type": 1,
        "subtype": "автомобиль",
        "title": "Toyota Camry",
        "price_minor": 250000,
        "currency": "RUB",
        "location_id": 1,
        "address": "ул. Ленина, 1",
        "status": "active",
        "photos": ["https://.../photo1.jpg"]
      }
    ],
    "total": 50,
    "next_page_token": "..."
  }
  ```

### 3.2. Получение детальной информации об объекте

- **Endpoint**: `GET /objects/{uuid}`
- **Ответ (200 OK)**:

  ```json
  {
    "uuid": "0f2d9c1e-3b6a-4c8d-9e10-2a3b4c5d6e7f",
    "type": 1,
    "subtype": "автомобиль",
    "title": "Toyota Camry",
    "description": "Новый автомобиль с кондиционером",
    "price_minor": 250000,
    "currency": "RUB",
    "min_duration_minutes": 60,
    "location_id": 1,
    "address": "ул. Ленина, 1",
    "status": "active",
    "photos": [
      {"url": "https://.../photo1.jpg", "is_main": true},
      {"url": "https://.../photo2.jpg", "is_main": false}
    ]
  }
  ```

## 4. Управление бронированиями

### 4.1. Создание бронирования

- **Endpoint**: `POST /bookings`
- **Тело запроса**:

  ```json
  {
    "object_uuid": "0f2d9c1e-3b6a-4c8d-9e10-2a3b4c5d6e7f",
    "start_at": "2025-12-01T10:00:00Z",
    "end_at": "2025-12-01T12:00:00Z",
    "addons": [
      {"addon_uuid": "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", "quantity": 1}
    ],
    "idempotency_key": "5c1f8e2a-9b4d-4f6e-8a3c-2d1e0f9a8b7c"
  }
  ```

- **Ответ (200 OK)**:

  ```json
  {
    "uuid": "9d8c7b6a-1f2e-4d3c-8b7a-6f5e4d3c2b1a",
    "total_price_minor": 500000,
    "currency": "RUB",
    "status": "pending"
  }
  ```

#### 4.2. Получение списка бронирований пользователя

- **Endpoint**: `GET /bookings/my`
- **Ответ (200 OK)**:

  ```json
  [
    {
      "uuid": "9d8c7b6a-1f2e-4d3c-8b7a-6f5e4d3c2b1a",
      "object_uuid": "0f2d9c1e-3b6a-4c8d-9e10-2a3b4c5d6e7f",
      "start_at": "2025-12-01T10:00:00Z",
      "end_at": "2025-12-01T12:00:00Z",
      "total_price_minor": 500000,
      "currency": "RUB",
      "status": "paid",
      "object_title": "Toyota Camry"
    }
  ]
  ```

#### 4.3. Изменение бронирования

- **Endpoint**: `PATCH /bookings/{uuid}`
- **Тело запроса** (частичное обновление):

  ```json
  {
    "start_at": "2025-12-01T11:00:00Z",
    "addons": [
      {"addon_uuid": "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e", "quantity": 2}
    ]
  }
  ```

- **Ответ (200 OK)**: обновлённый объект бронирования.

#### 4.4. Отмена бронирования

- **Endpoint**: `POST /bookings/{uuid}/cancel`
- **Ответ (200 OK)**: отменённая бронь со `status = "cancelled"`.

### 5. Платежи

#### 5.1. Инициализация платежа

- **Endpoint**: `POST /payments`
- **Тело запроса**:

  ```json
  {
    "booking_uuid": "9d8c7b6a-1f2e-4d3c-8b7a-6f5e4d3c2b1a",
    "payment_method": "card"
  }
  ```

- **Ответ (200 OK)**:

  ```json
  {
    "uuid": "3c4d5e6f-7a8b-4c9d-0e1f-2a3b4c5d6e7f",
    "redirect_url": "https://payment-gateway.com/pay/abc123",
    "status": "pending"
  }
  ```

#### 5.2. Проверка статуса платежа

- **Endpoint**: `GET /payments/{uuid}`
- **Ответ (200 OK)**:

  ```json
  {
    "uuid": "3c4d5e6f-7a8b-4c9d-0e1f-2a3b4c5d6e7f",
    "booking_uuid": "9d8c7b6a-1f2e-4d3c-8b7a-6f5e4d3c2b1a",
    "amount_minor": 500000,
    "currency": "RUB",
    "status": "succeeded",
    "paid_at": "2025-11-08T03:30:00Z"
  }
  ```

### 6. Дополнительные услуги

#### 6.1. Получение списка услуг для объекта

- **Endpoint**: `GET /objects/{uuid}/addons`
- **Ответ (200 OK)**:

  ```json
  [
    {
      "uuid": "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
      "name": "Заправка топливом",
      "price_minor": 50000,
      "currency": "RUB",
      "duration_minutes": 15
    }
  ]
  ```

### 7. Уведомления

#### 7.1. Подписка на уведомления

- **Endpoint**: `POST /notifications/subscribe`
- **Тело запроса**:

  ```json
  {
    "channel": "telegram",
    "endpoint": "user_telegram_id_123"
  }
  ```

- **Ответ (200 OK)**: `{}`

### 8. Веб‑панель операторов (административные методы)

#### 8.1. Получение статистики

- **Endpoint**: `GET /admin/analytics`
- **Параметры**: `period` (`day`, `week`, `month`)
- **Ответ (200 OK)**:

  ```json
  {
    "total_bookings": 150,
    "revenue_minor": 75000000,
    "currency": "RUB",
    "avg_booking_value_minor": 500000
  }
  ```
