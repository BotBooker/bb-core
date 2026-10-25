# bb-core

[![golang version][golang-image]][golang-url]
[![License][license-image]][license-url]
[![codecov][codecov-image]][codecov-url]
[![CodeQL Advanced][CodeQL-image]][CodeQL-url]
[![Trivy Security Scan][Trivy-image]][Trivy-url]

Backend‑часть сервиса BotBooker — платформы для бронирования услуг через чат‑ботов.

## Запуск

Точка входа `cmd/api` поднимает HTTP- и gRPC-серверы с одним и тем же API
(`internal/servers`): gRPC отдаёт proto-сервисы, HTTP — grpc-gateway поверх них.

```sh
DSN=sqlite:/tmp/database.db go run ./cmd/api   # миграции применятся при старте
```

| Переменная | По умолчанию | Назначение |
| --- | --- | --- |
| `SERVER_HOST`, `SERVER_PORT` | `localhost`, `8080` | HTTP: `/health` (liveness), `/ready` (readiness), `/api/v1`, `/api/swagger/` |
| `SERVER_GRPC_HOST`, `SERVER_GRPC_PORT` | `localhost`, `50051` | gRPC: `user.v1.UserService`, `auth.v1.AuthService`, health, reflection |
| `DSN` | — | подключение к БД; диалект (postgres/sqlite) определяется по DSN |
| `MIGRATIONS_DIR` | `./migrations` | каталог goose-миграций (`postgres/`, `sqlite/`); в образе — `/app/migrations` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | адрес OTel Collector для всех сигналов |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | — | адрес OTel Collector для логов |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | — | адрес OTel Collector для метрик |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | — | адрес OTel Collector для трасс |
| `ENABLE_FORCE_TRACE` | `false` | `true` — записывать все трассы (форс-трассировка) |

```sh
curl -s localhost:8080/health
curl -s localhost:8080/ready
grpcurl -plaintext localhost:50051 list
```

## Аутентификация

Ручки авторизации живут в `internal/servers/auth.go`:

| Ручка | Назначение |
| --- | --- |
| `POST /auth/register` | Самостоятельная регистрация (роль `client`), сразу выдаёт токен |
| `POST /auth/login` | Вход по email и паролю |
| `POST /auth/password` | Смена пароля по текущему паролю |
| `POST /auth/logout` | Выход (отзыв токена) |
| `GET /auth/tenants` | Тенанты действующего пользователя |
| `POST /auth/tenant/switch` | Новая сессия в выбранном тенанте |
| `POST /auth/logout-all` | Выход из всех сессий пользователя |
| `GET /users/me` | Профиль действующего пользователя |

Те же операции доступны как proto-сервис `auth.v1.AuthService`
(`proto/auth/v1/auth.proto`): gRPC-методы `Register`, `Login`, `ChangePassword`,
`Logout`, `GetCurrentUser`, `ListMyTenants`, `SwitchTenant`, `LogoutAll`, а через
grpc-gateway — ручки
`POST /api/v1/auth/{register,login,password,logout}`,
`GET /api/v1/auth/tenants`, `POST /api/v1/auth/tenant/switch` и
`POST /api/v1/auth/logout-all`, `GET /api/v1/auth/me`.
Один адаптер (`internal/servers/auth_service.go`) обслуживает оба транспорта,
поэтому коды и тела ответов совпадают.

Смок-тест ручек авторизации:

```sh
BASE_URL=http://localhost:8080 tests/smoke/auth.sh
```

## Мультитенантность

Пользователь входит в тенант по ссылке `/t/{tenant_slug}`; публичный каталог
тенанта доступен без токена (`/api/v1/public/tenants/{slug}` и
`/api/v1/public/tenants/{slug}/objects`). Регистрация с `tenant_slug` создаёт
клиентское членство в тенанте ссылки, без него — личный тенант (обратная
совместимость).

Сессия привязана к тенанту, роль — атрибут членства. `GET /auth/tenants`,
`POST /auth/tenant/switch` и `POST /auth/logout-all` управляют сессиями;
приостановка/удаление тенанта, исключение участника и смена роли отзывают
затронутые сессии. Управление тенантами доступно через `tenant.v1.TenantService`
(`/api/v1/tenants...`).

On-premise: `deploy/compose.yaml`, Helm-чарт `deploy/helm/bb-core`, скрипты
backup/restore и [docs/RUNBOOK.md](docs/RUNBOOK.md). Тестовый стенд в
Kubernetes — `make kind-test` (kind-кластер, PostgreSQL в кластере,
образ `bb-core:dev`, smoke-проверка `/ready`); удалить стенд — `make kind-down`.

Пароли хранятся только в виде хеша **Argon2id** (`pkg/password`; `internal/model` видит только строку хеша).
Хеши прежнего формата (PBKDF2-HMAC-SHA256) продолжают проверяться и
«прозрачно» пересчитываются в Argon2id при успешном входе — сбрасывать пароли
при обновлении не нужно.

### Срок жизни пароля

`PASSWORD_TTL` (по умолчанию `90d`) задаёт, сколько пароль действует с момента
последнего изменения. Формат — сутки с суффиксом `d` (`90d`, `1.5d`) или любая
единица `time.ParseDuration` (`2160h`, `30m`). Когда срок истёк, вход больше не
выдаёт токен:
`POST /auth/login` отвечает `403` с телом

```json
{"error": "password expired", "code": "password_expired"}
```

и клиент обязан отправить пользователя на смену пароля — `POST /auth/password`
с текущим и новым паролем. Только после смены выдаётся токен. Значение `0` или
отрицательное отключает проверку (пароль не истекает).

### Первый суперпользователь

Первого администратора создаёт команда bootstrap — она же применяет миграции.
Это **суперпользователь**: системный флаг `is_superuser` даёт ему права
администратора во всех тенантах и на всех объектах, а в личном тенанте он
дополнительно получает роль `admin`:

```sh
make bootstrap          # берёт DSN и SUPERUSER_* из .env
# или напрямую
DSN=sqlite:/tmp/database.db \
  SUPERUSER_EMAIL=admin@example.com \
  SUPERUSER_PASSWORD='change-me-please' \
  go run ./cmd/bootstrap
```

| Переменная | Назначение |
| --- | --- |
| `SUPERUSER_EMAIL` | Email первого суперпользователя (`is_superuser`, роль `admin` в личном тенанте) |
| `SUPERUSER_PASSWORD` | Его пароль (в базе хранится только хеш) |
| `SUPERUSER_FIRST_NAME`, `SUPERUSER_LAST_NAME` | Имя и фамилия (`Super`, `Admin` по умолчанию) |

Команда идемпотентна: если пользователь с таким email уже есть, пароль ему не
сбрасывается, но системный флаг `is_superuser` гарантируется (это также
переводит администратора, созданного до появления флага). Второго
суперпользователя через API завести нельзя.

### Глобальные операторы

Суперпользователь может назначать **глобальных операторов** — пользователей,
которые администрируют все тенанты. Права не зависят от роли в конкретном
членстве: глобальный оператор действует как `admin` в любом тенанте.

| Метод | Путь | Назначение |
| --- | --- | --- |
| `POST` | `/api/v1/users/{uuid}/global-operator` | Назначить глобального оператора |
| `DELETE` | `/api/v1/users/{uuid}/global-operator` | Снять права глобального оператора |

Обе операции доступны только суперпользователю (`403` для остальных), идемпотентны
и возвращают/подтверждают обновлённый профиль. Флаг `is_global_operator` виден в
`User` во всех ручках пользователей. Суперпользователя изменить нельзя: его права
не зависят от этого флага.

## Сборка

| Цель | Назначение |
| --- | --- |
| `make build` | DEV-сборка `./build/api` с ldflags (версия, дата, хост) |
| `make build-release` | Релизная статическая сборка (`CGO_ENABLED=0`, `-trimpath`, `-s -w`) |
| `make run` | Запуск через `go run ./cmd/api` |
| `make debug` | Сборка (если нужно) и запуск `./build/api` с `LOG_LEVEL=DEBUG` |
| `make test` | Тесты с профилем покрытия |
| `make coverage` | Проверка порога покрытия (≥80 %) |
| `make lint` | `golangci-lint` и `bearer` |
| `make go-check` | `govulncheck` — уязвимости зависимостей |

**Обратите внимание:** `make build` больше не релизная сборка, а DEV-бинарник с
ldflags; релиз собирается целью `make build-release`. Цель `build-debug` удалена.

## Документация

- [docs/specification/programming.md](docs/specification/programming.md) —
  правила кода: принципы, логирование и санитизация, обработка ошибок, тесты,
  сборка и CI.
- [docs/specification/architecture.md](docs/specification/architecture.md) —
  архитектура приложения.
- [docs/HOWTO.md](docs/HOWTO.md) — наблюдаемость (трассировка, метрики, логи).
- [docs/ROADMAP.md](docs/ROADMAP.md) — текущее состояние, продуктовые вехи и
  инженерный backlog.
- [docs/RUNBOOK.md](docs/RUNBOOK.md) — on-premise: деплой, backup/restore,
  диагностика.

---
[golang-image]: https://img.shields.io/github/go-mod/go-version/BotBooker/bb-core
[golang-url]: https://go.dev
[license-image]: https://img.shields.io/github/license/BotBooker/bb-core
[license-url]: https://opensource.org/license/apache-2.0
[codecov-image]: https://codecov.io/gh/BotBooker/bb-core/graph/badge.svg?token=S7CGSXNE61
[codecov-url]: https://codecov.io/gh/BotBooker/bb-core
[CodeQL-image]: https://github.com/BotBooker/bb-core/actions/workflows/codeql.yml/badge.svg
[CodeQL-url]: https://github.com/BotBooker/bb-core/actions/workflows/codeql.yml
[Trivy-image]: https://github.com/BotBooker/bb-core/actions/workflows/trivy-scan.yml/badge.svg
[Trivy-url]: https://github.com/BotBooker/bb-core/actions/workflows/trivy-scan.yml
