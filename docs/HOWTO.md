# HOWTO

## Миграции

Схема БД описана goose-миграциями: `migrations/postgres/*.sql` и
`migrations/sqlite/*.sql` встроены в бинарник (`migrations/embed.go`).
Диалект определяется по `DSN`, отдельной переменной для драйвера нет.

Миграции применяются при старте приложения:

```sh
DSN=postgres://user:password@localhost:5432/bbcore?sslmode=disable ./build/api
DSN=sqlite:/tmp/database.db SERVER_PORT=8080 ./build/api
```

Если раскаткой миграций занимается отдельный шаг пайплайна, авто-применение
отключается переменной `MIGRATE_ON_START=false`.

Ручная раскатка и откат — тем же кодом, но отдельным процессом:

```sh
make migrate-up     # применить все неприменённые миграции
make migrate-down   # откатить последнюю применённую (повторный вызов — предыдущую)
```

Команды берут `DSN` из окружения (Makefile подхватывает `.env`), диалект
определяется по DSN — как и при старте приложения. Если откатывать нечего, шаг
завершается успешно: состояние БД уже соответствует требуемому.

При прямом запуске DSN тоже задаётся переменной окружения **перед** командой,
иначе `go run` примет `DSN=...` за аргумент программы и тот будет отклонён:

```sh
DSN=sqlite:/tmp/database.db go run ./cmd/migrate down
```

Формат файлов: один файл — одна версия (`00002_users.sql`), внутри секции
`-- +goose Up` и `-- +goose Down`. Пары `*.up.sql` + `*.down.sql` goose не
поддерживает: он считает каждый файл отдельной миграцией и падает на дубликате
версии.

## Серверы

Одна точка входа (`cmd/api`) поднимает два сервера с одним и тем же API:

| Сервер | Адрес по умолчанию | Переменные | Что отдаёт |
| --- | --- | --- | --- |
| HTTP | `localhost:8080` | `SERVER_HOST`, `SERVER_PORT` | собственные ручки и grpc-gateway |
| gRPC | `localhost:50051` | `SERVER_GRPC_HOST`, `SERVER_GRPC_PORT` | proto-сервисы по HTTP/2 |

Хендлеры у транспортов общие: `grpc-gateway` вызывает тот же адаптер proto-сервиса
в процессе приложения, поэтому HTTP-ручки не зависят от доступности gRPC-порта
(и наоборот). Оба сервера закрываются graceful shutdown по SIGINT/SIGTERM.

Ручки:

* `GET /health` — готовность приложения (`ok`), отдельно от gRPC-health;
* `GET /users/me` — скелет профиля, ждёт работ по аутентификации;
* `POST /api/v1/users`, `GET|PATCH|DELETE /api/v1/users/{uuid}` — grpc-gateway
  поверх `user.v1.UserService` (в proto пути заданы от корня: `/users`,
  `/users/{uuid}`, префикс `/api/v1` добавляет монтирование gateway);
* `GET /api/swagger/` — Swagger UI (спека в `pkg/api`).

В gRPC дополнительно включены `grpc.health.v1.Health` (нужен балансировщикам и
readiness-пробам) и reflection (нужен grpcurl при отладке).

Коды ошибок согласованы у обоих транспортов: `NotFound` → 404, `AlreadyExists`
(конфликт уникальности email или телефона) → 409, `InvalidArgument` → 400,
остальное → 500 без деталей внутренней ошибки.

```sh
# HTTP
curl -s localhost:8080/health
curl -s -X POST localhost:8080/api/v1/users -H 'Content-Type: application/json' \
  -d '{"email":"willi@example.com","first_name":"Вася","last_name":"Петров","phone_number":"+79990000001"}'
curl -s localhost:8080/api/v1/users/<uuid>

# gRPC (reflection включён — proto файлы для grpcurl не нужны)
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
grpcurl -plaintext -d '{"email":"ivan@example.com","first_name":"Иван","last_name":"Иванов"}' \
  localhost:50051 user.v1.UserService/Create
grpcurl -plaintext -d '{"uuid": "7c3ca7c0-ec6f-42d5-a77e-6ca55c037ae3"}' \
  localhost:50051 user.v1.UserService/Get
```

Автотесты транспорта: `go test ./internal/serverapi/ -run TestSmoke -v` —
поднимают приложение целиком (SQLite во временном файле, gRPC-сервер по сети и
HTTP через gateway) и проверяют сценарий целиком: create → get → update → delete,
конфликт уникальности и отсутствующие записи.
