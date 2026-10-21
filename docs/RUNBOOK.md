# Runbook: bb-core on-premise

Эксплуатация установки bb-core: развёртывание, первичная инициализация,
управление тенантами, резервное копирование и восстановление.

## 1. Требования

- PostgreSQL 14+ (рекомендуется 16) или SQLite для dev/тестов;
- 2 vCPU, 2 GiB RAM на экземпляр API (минимум), внешний балансировщик;
- секрет `JWT_SECRET` не короче 32 байт, одинаковый на всех репликах;
- `PUBLIC_BASE_URL` — внешний адрес, по которому открывается `/t/{tenant_slug}`.

Диалект определяется по `DSN`: `postgres://...` или `sqlite:/path/file.db`.
Для промышленной установки используйте PostgreSQL: SQLite не рассчитан на
несколько реплик и конкурентную запись.

## 2. Docker Compose

```sh
cd deploy
export POSTGRES_PASSWORD="$(openssl rand -hex 24)"
export JWT_SECRET="$(openssl rand -base64 32)"
export SUPERUSER_EMAIL=admin@example.com
export SUPERUSER_PASSWORD='change-me-please'
export PUBLIC_BASE_URL=http://localhost:8080

docker compose up -d postgres
docker compose up -d api
docker compose run --rm bootstrap
```

Проверка:

```sh
curl -fsS http://localhost:8080/health
docker compose logs -f api
```

`MIGRATE_ON_START=true` применяет миграции при старте. Если миграции
раскатываются отдельным шагом, поставьте `MIGRATE_ON_START=false` и запустите
`/app/migrate` из того же образа до старта новой версии. SQL-файлы входят в
образ (`/app/migrations`, переменная `MIGRATIONS_DIR`), поэтому отдельно
копировать их на хост не нужно.

## 3. Helm

```sh
kubectl create secret generic bb-core-secrets \
  --from-literal=DSN='postgres://bbcore:password@postgres:5432/bbcore?sslmode=require' \
  --from-literal=JWT_SECRET="$(openssl rand -base64 32)"

helm upgrade --install bb-core ./deploy/helm/bb-core \
  --set image.repository=registry.example.com/bb-core \
  --set image.tag=0.1.0 \
  --set config.publicBaseURL=https://bb.example.com \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=bb.example.com
```

Bootstrap выполняется один раз (Job или `kubectl run`, см. `NOTES.txt` чарта).
Повторный запуск безопасен: пароль существующего суперпользователя не
сбрасывается.

Чарт по умолчанию запускает контейнер по модели `restricted`
(`docs/specification/programming.md`, §7): non-root `55555:55555`, read-only
корневая ФС, seccomp `RuntimeDefault`, `capabilities.drop: [ALL]`. Меняя
`podSecurityContext`/`securityContext` в `values.yaml`, сохраняйте эти
инварианты — иначе CI-сканер `trivy config` вернёт HIGH.

## 4. Первичная инициализация и вход

1. `bootstrap` создаёт суперпользователя, его личный тенант и членство
   владельца с ролью `admin`; в лог печатаются `tenant_slug` и `login_url`.
2. Суперпользователь создаёт рабочий тенант:
   `POST /api/v1/tenants` с `name` и `slug`.
3. Ссылка для входа в тенант: `${PUBLIC_BASE_URL}/t/{slug}`. Клиент
   регистрируется по ссылке (`tenant_slug` в `POST /auth/register`) и сразу
   становится участником с ролью `client`; личный тенант при этом не создаётся.
4. Вход: `POST /auth/login` с `email`, `password` и необязательным
   `tenant_slug`. Пустой `tenant_slug` — самое раннее действующее членство
   (обратная совместимость).
5. Список доступных тенантов: `GET /auth/tenants`.
   Переключение: `POST /auth/tenant/switch` с `{"tenant_slug":"..."}`.
   Полный выход: `POST /auth/logout-all`.

Bootstrap — это не «создание тенанта на каждый запуск»: команда идемпотентна и
нужна только для первого администратора установки.

## 5. Управление тенантами

Все ручки требуют токен; права проверяются по членству и роли, а не только
транспортом.

| Операция | Ручка | Кто |
| --- | --- | --- |
| Создать тенант | `POST /api/v1/tenants` | суперпользователь, глобальный оператор |
| Список тенантов | `GET /api/v1/tenants` | глобальные администраторы — все, остальные — свои |
| Прочитать тенант | `GET /api/v1/tenants/{uuid}` | участник или глобальный администратор |
| Приостановить/возобновить | `PATCH /api/v1/tenants/{uuid}/status` | глобальный администратор |
| Мягко удалить | `DELETE /api/v1/tenants/{uuid}` | администратор тенанта или глобальный |
| Участники | `GET/POST /api/v1/tenants/{uuid}/members` | staff тенанта, глобальный администратор |
| Сменить роль | `PATCH /api/v1/tenants/{uuid}/members/{user_uuid}` | администратор тенанта |
| Исключить | `DELETE /api/v1/tenants/{uuid}/members/{user_uuid}` | администратор тенанта |
| Передать владение | `POST /api/v1/tenants/{uuid}/ownership` | владелец или глобальный администратор |

Приостановка (`suspended`) и удаление (`deleted`) отзывают все сессии тенанта;
смена роли и исключение — сессии участника в этом тенанте. Владельца нельзя
исключить или понизить: сначала передайте владение.

### Offboarding тенанта

1. Переведите тенант в `suspended`, проверьте, что участники потеряли доступ
   (старые токены отозваны).
2. Выгрузите данные, если они нужны: объекты, брони, платежи, уведомления
   содержат `tenant_id`.
3. Переведите в `deleted` (мягкое удаление). Строки сохраняются для аудита;
   физическое удаление — отдельная процедура по политике retention.
4. Удалите секреты тенанта (интеграции, ключи) из внешних систем.

## 6. Резервное копирование

```sh
export DSN='postgres://bbcore:password@postgres:5432/bbcore?sslmode=require'
./deploy/scripts/backup.sh /backup/bb-core-$(date -u +%Y%m%dT%H%M%SZ).dump
```

- Делайте backup перед каждой миграцией и по расписанию (например, ежедневно).
- Храните копии вне хоста БД, шифруйте at-rest, ограничивайте доступ.
- Проверяйте восстановление на отдельной базе не реже раза в квартал.
- Retention: минимум 30 дней ежедневных копий и 12 месячных, если иное не
  требует политика организации.

## 7. Восстановление

```sh
export DSN='postgres://bbcore:password@postgres:5432/bbcore_restore?sslmode=require'
./deploy/scripts/restore.sh /backup/bb-core-2026-10-18T000000Z.dump
```

`pg_restore --clean --if-exists` удаляет объекты в целевой базе: **не**
запускайте на рабочей базе без подтверждения. Рекомендуемый порядок:

1. Остановите API (или переведите в maintenance-режим балансировщиком).
2. Восстановите дамп в новую базу, проверьте целостность:
   `SELECT count(*) FROM tenants;`, `SELECT count(*) FROM bookings;`,
   `SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1;`.
3. Переключите `DSN` на восстановленную базу и запустите `/app/migrate`.
4. Запустите API, проверьте `/health`, вход и `GET /api/v1/tenants`.
5. Если версия схемы старше кода — сначала накатите миграции, затем API.

При откате релиза: остановите новые реплики, верните предыдущий образ и, если
миграция необратима, восстановите backup. Обратные миграции (`goose down`)
предусмотрены, но на промышленных данных предпочтительнее restore.

## 8. Наблюдаемость

- `/health` — readiness/liveness.
- OTLP: задайте `OTEL_EXPORTER_OTLP_ENDPOINT`,
  `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (при необходимости),
  `OTEL_METRICS_EXPORTER=otlp`; при выключенном экспортёре состояние пишется в
  `Debug`.
- Логи структурированные, `trace_id` добавляется автоматически. Не логируйте
  токены, пароли и тела запросов с PII.
- Метрики Prometheus отдаются через OTLP-коллектор (см. конфигурацию
  observability).

## 9. Диагностика

| Симптом | Что проверить |
| --- | --- |
| 401 на всех ручках тенанта | срок/отзыв токена, статус тенанта, членство |
| 403 при switch | пользователь не участник тенанта или тенант не active |
| 404 на `/t/{slug}` | slug, статус тенанта (suspended/deleted скрыты) |
| 500 после миграции | `MIGRATE_ON_START`, версия схемы, откат/restore |
| Пустой публичный каталог | у объектов `tenant_id`, статус не `deleted` |
| Сессии не отзываются | проверить `user_tokens.revoked_at`, писать в БД |

Полезные запросы:

```sql
SELECT id, slug, status FROM tenants ORDER BY created_at;
SELECT tenant_id, user_id, role_id, is_owner FROM tenant_members WHERE tenant_id = $1;
SELECT id, user_id, tenant_id, revoked_at FROM user_tokens WHERE tenant_id = $1;
SELECT count(*) FROM rental_objects WHERE tenant_id = $1 AND status_id <> 4;
```
