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

## Наблюдаемость

### Минимальная инфраструктура (Docker)

```sh
docker run -d --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest
```

* [Web UI](http://localhost:16686)

### Логи

Структурированные логи приложения (`log/slog`) пишутся в stdout и, если задан
адрес коллектора, дублируются в OpenTelemetry Collector по OTLP/gRPC:

```sh
OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=http://localhost:4317 ./build/api
# или общий адрес для всех сигналов:
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 ./build/api
```

Без этих переменных экспорт логов выключен — приложение пишет только в stdout.
Адрес по умолчанию — `localhost:4317`; схема `https` включает TLS.

Логи пишутся структурированно через `log/slog` (`pkg/logger`) в stdout — в
формате `LOG_FORMAT` (`json` по умолчанию, `plain` для разработки) и с уровнем
`LOG_LEVEL`.

Если задан адрес OTel Collector, те же записи дополнительно уходят в него по
OTLP/gRPC, а вывод в stdout сохраняется: глобальный логгер собирается веером из
двух обработчиков (`pkg/logger.MultiHandler`). Переменные:

| Переменная | Приоритет | Назначение |
| --- | --- | --- |
| `OTEL_SERVICE_NAME` | - | назначить resource attribute service.name |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | ниже | общий адрес коллектора для всех сигналов |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | выше | адрес коллектора для логов |

Экспорт включается, только когда задана хотя бы одна из них. Схема в адресе
задаёт транспорт: `http://` — plaintext gRPC, `https://` — TLS, а адрес без схемы
(`localhost:4317`) трактуется как `http`. Без этих переменных приложение не
трогает сеть и пишет логи только в stdout.

```sh
OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=http://localhost:4317 go run ./cmd/api
```

Для локальной проверки подойдёт конфигурация `otel/customconfig.yaml` — приёмник
OTLP и debug-экспортёр, который печатает полученные записи:

```sh
docker run --rm -p 4317:4317 -p 4318:4318 \
  -v "$PWD/otel/customconfig.yaml:/etc/otelcol/config.yaml" \
  otel/opentelemetry-collector:latest
```

Реализация — `pkg/observability/logs.go` (`setupLogs`): OTel-провайдер логов
подключается к глобальному логгеру через `logger.SetOTelHandler`, вывод в stdout
сохраняется. Автотесты:
`go test ./pkg/observability/ -run 'TestSetupLogs|TestLogsEndpoint' -v` —
проверяют включение экспорта по переменным окружения и отправку записей в
поддельный OTLP-коллектор.

### Трассировка

Трассировка включается, только если задан адрес коллектора —
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` или общий `OTEL_EXPORTER_OTLP_ENDPOINT`.
Если ни одной из переменных нет, провайдер трассировки не создаётся вовсе, и
приложение не трогает сеть:

```sh
OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://localhost:4317 ./build/api
```

Спаны пишутся по OTLP/gRPC (порт `4317` по умолчанию), W3C Trace Context
пробрасывается через заголовки. Что и когда записывается:

| Сценарий | Когда запускается | Как включается |
| --- | --- | --- |
| Force Trace | любой спан | `ENABLE_FORCE_TRACE=true` |
| Context Propagation | входящий HTTP-запрос | заголовок `traceparent` (W3C) |
| App Startup | старт приложения | новая трасса, спан `app.startup` |
| App Shutdown | SIGINT / SIGTERM | новая трасса, спан `app.shutdown` |

Стартовый спан закрывается, когда HTTP- и gRPC-серверы подняты и приложение
готово принимать трафик. Завершающий спан открывается при перехвате сигнала и
закрывается перед выходом процесса — после остановки серверов и закрытия
ресурсов. Оба спана начинают собственную трассу с отдельным TraceID.

Для локальной проверки подойдёт конфигурация `otel/customconfig.yaml` — приёмник
OTLP и debug-экспортёр, который печатает полученные сигналы:

```sh
docker run --rm -p 4317:4317 -p 4318:4318 \
  -v "$PWD/otel/customconfig.yaml:/etc/otelcol/config.yaml" \
  otel/opentelemetry-collector:latest

# в другом терминале — проверить сквозной путь «запрос → трасса в коллекторе»
curl -s -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" \
  localhost:8080/health
```

Трассировка включается, только когда задан адрес OTel Collector для трасс.
Переменные и их приоритет:

| Переменная | Приоритет | Назначение |
| --- | --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | ниже | общий адрес коллектора для всех сигналов |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | выше | адрес коллектора для трасс |
| `ENABLE_FORCE_TRACE` | - | `true` — записывать все спаны (форс-трассировка) |

Если ни `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, ни `OTEL_EXPORTER_OTLP_ENDPOINT` не
заданы, провайдер трассировки не создаётся вовсе: глобальный трейсер остаётся
no-op, приложение не трогает сеть. Схема в адресе задаёт транспорт: `http://` —
plaintext gRPC, `https://` — TLS, а адрес без схемы (`localhost:4317`) трактуется
как `http`. Порт по умолчанию — `4317`.

```sh
OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://localhost:4317 go run ./cmd/api
```

Точка входа — `observability.SetupOTelSDK` (`pkg/observability/observability.go`):
она поднимает провайдеры всех трёх сигналов и возвращает функцию остановки.
Спаны запускаются по четырём сценариям — SDK, сэмплер и пропагатор живут в
`pkg/observability/tracing.go`, HTTP-middleware и спаны жизненного цикла — в
`internal/serverapi/tracing.go`:

| Сценарий | Что делает | Реализация |
| --- | --- | --- |
| Force Trace | записывает все спаны, независимо от входящего контекста | `ENABLE_FORCE_TRACE=true`, правило в `traceSampler` (`pkg/observability`) |
| Context Propagation | продолжает входящую трассу по заголовку `traceparent` (W3C Trace Context) | `traceMiddleware`, `otel.GetTextMapPropagator().Extract()` |
| App Startup | открывает спан `app.startup` в новой трассе и закрывает его, когда HTTP- и gRPC-серверы подняты | `startStartupSpan` в `ServerAPI.Run()` |
| App Shutdown | перехватывает SIGINT/SIGTERM, открывает спан `app.shutdown` в новой трассе и закрывает его перед выходом процесса | `startShutdownSpan` в `ServerAPI.Run()` |

Стартовый и завершающий спаны начинают собственную трассу: SDK генерирует
отдельный TraceID, поэтому запуск и остановка сервиса видны в коллекторе
самостоятельными трассами. Сэмплер не превращает каждый запрос без `traceparent`
в новую трассу — такие корневые спаны отбрасываются, если не включён
`ENABLE_FORCE_TRACE`.

Сквозная проверка трассы:

```sh
curl -s -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" \
  localhost:8080/health
```

Автотесты: `go test ./pkg/observability/ ./internal/serverapi/ -run
'TestSetupTraces|TestTrace|TestStartupAndShutdown' -v` — проверяют правила
сэмплирования всех четырёх сценариев, работу HTTP-middleware и выгрузку трасс в
поддельный OTLP-коллектор.

### Метрики

Метрики включаются, только если задан адрес коллектора —
`OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` или общий `OTEL_EXPORTER_OTLP_ENDPOINT`.
Если ни одной из переменных нет, провайдер метрик не создаётся вовсе: глобальный
`otel.GetMeterProvider()` остаётся no-op, приложение не трогает сеть:

```sh
OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=http://localhost:4317 ./build/api
```

| Переменная | Приоритет | Назначение |
| --- | --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | ниже | общий адрес коллектора для всех сигналов |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | выше | адрес коллектора для метрик |

Метрики уходят по OTLP/gRPC (порт `4317` по умолчанию): измерения собираются
периодическим reader'ом раз в минуту, а на остановке приложения делается
финальный сбор и выгрузка, поэтому данные не теряются при graceful shutdown.
Схема в адресе задаёт транспорт: `http://` — plaintext gRPC, `https://` — TLS, а
адрес без схемы (`localhost:4317`) трактуется как `http`.

Для локальной проверки подойдёт конфигурация `otel/customconfig.yaml` — приёмник
OTLP и debug-экспортёр, который печатает полученные сигналы:

```sh
docker run --rm -p 4317:4317 -p 4318:4318 \
  -v "$PWD/otel/customconfig.yaml:/etc/otelcol/config.yaml" \
  otel/opentelemetry-collector:latest
```

Реализация — `pkg/observability/metrics.go` (`setupMetrics`). Автотесты:
`go test ./pkg/observability/ -run 'TestSetupMetrics|TestMetricsEndpoint' -v` —
проверяют включение экспорта по переменным окружения и выгрузку метрик в
поддельный OTLP-коллектор.
