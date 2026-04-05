# ER‑диаграмма базы данных сервиса автоматизированного бронирования

Ниже представлена детальная схема сущностей, их атрибутов и взаимосвязей в нотации **Питера Чена** (классическая ER‑нотация).

## 1. Список сущностей и их атрибуты

1. **`User`** (пользователи)
   * `user_id` (PK) — INT, автоинкремент;
   * `full_name` — VARCHAR(100);
   * `email` — VARCHAR(255), UNIQUE;
   * `phone` — VARCHAR(20);
   * `password_hash` — VARCHAR(255);
   * `role` — ENUM('client', 'operator', 'admin');
   * `created_at` — TIMESTAMP;
   * `last_login` — TIMESTAMP.

2. **`Objects`** (объекты аренды)
   * `object_id` (PK) — INT, автоинкремент;
   * `type` — ENUM('транспорт', 'недвижимость', 'рабочее_место', 'прочее');
   * `subtype` — VARCHAR(50);
   * `title` — VARCHAR(100);
   * `description` — TEXT;
   * `price_per_unit` — DECIMAL(10,2);
   * `min_duration` — INT (в минутах/часах);
   * `location` — VARCHAR(255);
   * `status` — ENUM('активен', 'на_ремонте', 'архив');
   * `owner_id` — INT (FK → User.user_id);
   * `created_at` — TIMESTAMP.

3. **`Bookings`** (бронирования)
   * `booking_id` (PK) — INT, автоинкремент;
   * `user_id` (FK) — INT;
   * `object_id` (FK) — INT;
   * `start_datetime` — TIMESTAMP;
   * `end_datetime` — TIMESTAMP;
   * `status` — ENUM('ожидает', 'подтверждено', 'оплачено', 'отменено', 'завершено');
   * `total_price` — DECIMAL(10,2);
   * `created_at` — TIMESTAMP;
   * `updated_at` — TIMESTAMP.

4. **`Payments`** (платежи)
   * `payment_id` (PK) — INT, автоинкремент;
   * `booking_id` (FK) — INT;
   * `amount` — DECIMAL(10,2);
   * `payment_method` — ENUM('карта', 'СБП', 'кошелёк');
   * `transaction_id` — VARCHAR(50), UNIQUE;
   * `status` — ENUM('успешно', 'ошибка', 'возврат');
   * `paid_at` — TIMESTAMP.

5. **`Addons`** (дополнительные услуги)
   * `addon_id` (PK) — INT, автоинкремент;
   * `name` — VARCHAR(100);
   * `price` — DECIMAL(10,2);
   * `duration_minutes` — INT;
   * `applicable_types` — JSON (массив типов объектов).

6. **`BookingAddons`** (связь бронирований и услуг)
   * `booking_addon_id` (PK) — INT, автоинкремент;
   * `booking_id` (FK) — INT;
   * `addon_id` (FK) — INT;
   * `quantity` — INT.

7. **`ObjectPhotos`** (фото объектов)
   * `photo_id` (PK) — INT, автоинкремент;
   * `object_id` (FK) — INT;
   * `url` — VARCHAR(500);
   * `is_main` — BOOLEAN;
   * `order` — INT.

8. **`Notifications`** (уведомления)
   * `notification_id` (PK) — INT, автоинкремент;
   * `user_id` (FK) — INT;
   * `message` — TEXT;
   * `channel` — ENUM('telegram', 'email', 'push');
   * `sent_at` — TIMESTAMP;
   * `status` — ENUM('отправлено', 'доставлено', 'ошибка').

9. **`AuditLog`** (журнал аудита)
   * `log_id` (PK) — INT, автоинкремент;
   * `entity` — VARCHAR(50);
   * `entity_id` — INT;
   * `action` — ENUM('создание', 'изменение', 'удаление');
   * `old_value` — JSON;
   * `new_value` — JSON;
   * `user_id` (FK) — INT;
   * `timestamp` — TIMESTAMP.

### 2. Связи между сущностями (Relationships)

1. **User ↔ Bookings**
   * Тип: 1:M (один пользователь → много бронирований);
   * FK: `Bookings.user_id` → `User.user_id`.

2. **Objects ↔ Bookings**
   * Тип: 1:M (один объект → много бронирований);
   * FK: `Bookings.object_id` → `Objects.object_id`.

3. **Bookings ↔ Payments**
   * Тип: 1:1 (одно бронирование → один платёж);
   * FK: `Payments.booking_id` → `Bookings.booking_id`.

4. **Objects ↔ ObjectPhotos**
   * Тип: 1:M (один объект → много фото);
   * FK: `ObjectPhotos.object_id` → `Objects.object_id`.

5. **Addons ↔ BookingAddons**
   * Тип: 1:M (одна услуга → много записей в бронировании);
   * FK: `BookingAddons.addon_id` → `Addons.addon_id`.

6. **Bookings ↔ BookingAddons**
   * Тип: 1:M (одно бронирование → много дополнительных услуг);
   * FK: `BookingAddons.booking_id` → `Bookings.booking_id`.

7. **User ↔ Notifications**
   * Тип: 1:M (один пользователь → много уведомлений);
   * FK: `Notifications.user_id` → `User.user_id`.

8. **User ↔ AuditLog**
   * Тип: 1:M (один пользователь → много записей аудита);
   * FK: `AuditLog.user_id` → `User.user_id`.

## 3. Ограничения и индексы

**Первичные ключи (PK):**

* Все сущности имеют автоинкрементный `id` как PK.

**Внешние ключи (FK):**

* Обеспечивают ссылочную целостность (ON DELETE RESTRICT / ON UPDATE CASCADE).

**Уникальные индексы (UNIQUE):**

* `User.email`;
* `Payments.transaction_id`.

**Индексы для производительности:**

* `Bookings(start_datetime, end_datetime)` — для поиска свободных объектов;
* `Objects(type, subtype, status)` — фильтрация объектов;
* `Payments(paid_at)` — отчёты по платежам.

### 4. Визуализация связей (текстовая схема)

```console
User
├── Bookings
│   ├── Payments
│   └── BookingAddons
│       └── Addons
└── Notifications


Objects
├── Bookings
└── ObjectPhotos

AuditLog (связано с User и любыми entity_id)
```

## 5. Примечания

1. **Нормализация:** схема соответствует 3NF (нет транзитивных зависимостей).
2. **Масштабируемость:** для высокой нагрузки рекомендуется шардинг по `user_id` или `object_id`.
3. **Безопасность:** пароли хранятся в виде хешей (bcrypt), чувствительные данные — в зашифрованном виде.
4. **Локализация:** текстовые поля (названия, описания) хранятся в UTF‑8.
5. **Версионирование:** для изменений структуры БД использовать миграционные скрипты (например, Liquibase).
