# Спецификация API сервиса автоматизированного бронирования

## 1. Общие сведения

- **Базовый URL**: `https://api.booking-service.com/v1`
- **Формат данных**: JSON (Content‑Type: `application/json`)
- **Аутентификация**: JWT‑токен в заголовке `Authorization: Bearer <token>`
- **Кодировка**: UTF‑8
- **Версионирование**: через URL (`/v1`, `/v2`)

## 2. Методы аутентификации и авторизации

### 2.1. Регистрация пользователя

- **Endpoint**: `POST /auth/register`
- **Тело запроса**:

  ```json
  {
    "full_name": "Иван Иванов",
    "email": "user@example.com",
    "phone": "+79991234567",
    "password": "secure_password"
  }
  ```

- **Ответ (201 Created)**:

  ```json
  {
    "user_id": 123,
    "email": "user@example.com",
    "role": "client"
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
    "token": "eyJ...abc123",
    "user_id": 123,
    "role": "client"
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

## 3. Управление объектами аренды

### 3.1. Получение списка объектов

- **Endpoint**: `GET /objects`
- **Параметры запроса (query)**:
  + `type` (например, `транспорт`, `недвижимость`)
  + `subtype` (например, `автомобиль`, `квартира`)
  + `location` (строка)
  + `min_price`, `max_price` (число)
  + `page`, `limit` (пагинация)
- **Ответ (200 OK)**:

  ```json
  {
    "items": [
      {
        "object_id": 456,
        "type": "транспорт",
        "subtype": "автомобиль",
        "title": "Toyota Camry",
        "price_per_unit": 2500.00,
        "location": "Москва, ул. Ленина, 1",
        "photos": ["https://.../photo1.jpg"]
      }
    ],
    "total": 50,
    "page": 1
  }
  ```

### 3.2. Получение детальной информации об объекте

- **Endpoint**: `GET /objects/{object_id}`
- **Ответ (200 OK)**:

  ```json
  {
    "object_id": 456,
    "type": "транспорт",
    "subtype": "автомобиль",
    "title": "Toyota Camry",
    "description": "Новый автомобиль с кондиционером",
    "price_per_unit": 2500.00,
    "min_duration": 60,
    "location": "Москва, ул. Ленина, 1",
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
    "object_id": 456,
    "start_datetime": "2025-12-01T10:00:00Z",
    "end_datetime": "2025-12-01T12:00:00Z",
    "addons": [
      {"addon_id": 789, "quantity": 1}
    ]
  }
  ```

- **Ответ (201 Created)**:

  ```json
  {
    "booking_id": 101,
    "total_price": 5000.00,
    "status": "ожидает"
  }
  ```

#### 4.2. Получение списка бронирований пользователя

- **Endpoint**: `GET /bookings/my`
- **Ответ (200 OK)**:

  ```json
  [
    {
      "booking_id": 101,
      "object_id": 456,
      "start_datetime": "2025-12-01T10:00:00Z",
      "end_datetime": "2025-12-01T12:00:00Z",
      "total_price": 5000.00,
      "status": "оплачено",
      "object_title": "Toyota Camry"
    }
  ]
  ```

#### 4.3. Изменение бронирования

- **Endpoint**: `PUT /bookings/{booking_id}`
- **Тело запроса** (частичное обновление):

  ```json
  {
    "start_datetime": "2025-12-01T11:00:00Z",
    "addons": [
      {"addon_id": 790, "quantity": 2}
    ]
  }
  ```

- **Ответ (200 OK)**: обновлённый объект бронирования.

#### 4.4. Отмена бронирования

- **Endpoint**: `DELETE /bookings/{booking_id}`
- **Ответ (204 No Content)**: `{}`

### 5. Платежи

#### 5.1. Инициализация платежа

- **Endpoint**: `POST /payments`
- **Тело запроса**:

  ```json
  {
    "booking_id": 101,
    "payment_method": "карта"
  }
  ```

- **Ответ (200 OK)**:

  ```json
  {
    "payment_id": 202,
    "redirect_url": "https://payment-gateway.com/pay/abc123",
    "status": "ожидает"
  }
  ```

#### 5.2. Проверка статуса платежа

- **Endpoint**: `GET /payments/{payment_id}`
- **Ответ (200 OK)**:

  ```json
  {
    "payment_id": 202,
    "booking_id": 101,
    "amount": 5000.00,
    "status": "успешно",
    "paid_at": "2025-11-08T03:30:00Z"
  }
  ```

### 6. Дополнительные услуги

#### 6.1. Получение списка услуг для объекта

- **Endpoint**: `GET /objects/{object_id}/addons`
- **Ответ (200 OK)**:

  ```json
  [
    {
      "addon_id": 789,
      "name": "Заправка топливом",
      "price": 500.00,
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

- **Ответ (201 Created)**: `{}`

### 8. Веб‑панель операторов (административные методы)

#### 8.1. Получение статистики

- **Endpoint**: `GET /admin/analytics`
- **Параметры**: `period` (`day`, `week`, `month`)
- **Ответ (200 OK)**:

  ```json
  {
    "total_bookings": 150,
    "revenue": 750000.00,
    "avg_booking_value": 5000.00
  }
  ```
