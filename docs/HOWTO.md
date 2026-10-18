# HOWTO

## Миграции

Схема БД описана goose-миграциями: `migrations/postgres/*.sql` и
`migrations/sqlite/*.sql`. SQL-файлы не встроены в бинарник: приложение читает
их с диска из каталога `MIGRATIONS_DIR` (по умолчанию `./migrations`).
В Docker-образе файлы лежат в `/app/migrations`, а рабочий каталог — `/app`,
поэтому значение по умолчанию подходит без настройки; при монтировании миграций
извне путь переопределяется переменной. Диалект определяется по `DSN`, отдельной
переменной для драйвера нет.

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

## Bootstrap: первый суперпользователь

Первого администратора создаёт отдельная команда `cmd/bootstrap` — по образцу
`cmd/migrate`. Она применяет миграции и идемпотентно создаёт суперпользователя
(системный флаг `is_superuser`; в личном тенанте — роль `admin`) с параметрами
из окружения:

```sh
make bootstrap     # DSN и SUPERUSER_* берутся из .env
```

Прямой запуск — так же, как у миграций, переменные идут **перед** командой:

```sh
DSN=sqlite:/tmp/database.db \
  SUPERUSER_EMAIL=admin@example.com \
  SUPERUSER_PASSWORD='change-me-please' \
  go run ./cmd/bootstrap
```

| Переменная | По умолчанию | Назначение |
| --- | --- | --- |
| `DSN` | — | подключение к БД; диалект определяется по DSN |
| `SUPERUSER_EMAIL` | — | email суперпользователя (обязательна) |
| `SUPERUSER_PASSWORD` | — | пароль суперпользователя (обязательна) |
| `SUPERUSER_FIRST_NAME` | `Super` | имя |
| `SUPERUSER_LAST_NAME` | `Admin` | фамилия |

Повторный запуск не сбрасывает пароль: если пользователь с таким email уже есть,
он получает (или сохраняет) флаг `is_superuser`. Пустые
`SUPERUSER_EMAIL`/`SUPERUSER_PASSWORD` — ошибка конфигурации (код возврата
процесса `1`). Второго суперпользователя через API завести нельзя, но
суперпользователь может назначать глобальных операторов:

```sh
# назначить (доступно только суперпользователю)
curl -X POST https://app.example/api/v1/users/{uuid}/global-operator \
  -H "Authorization: Bearer $TOKEN"

# снять права
curl -X DELETE https://app.example/api/v1/users/{uuid}/global-operator \
  -H "Authorization: Bearer $TOKEN"
```

Глобальный оператор (`is_global_operator`) администрирует все тенанты независимо
от роли в членстве. Обе операции идемпотентны; флаг виден в ответах `user.v1`.

## Пароли: хеширование и срок жизни

Пароли хешируются **Argon2id** (`internal/model/password.go`; параметры OWASP —
19 MiB, `t=2`, `p=1`), а не MD5/SHA: подбор пароля намеренно дорог. Алгоритм и
его параметры хранятся в самой строке хеша, поэтому их изменение не ломает уже
сохранённые пароли. Хеши прежнего формата (`pbkdf2_sha256$...`) продолжают
проверяться и при успешном входе «прозрачно» пересчитываются в Argon2id
(`model.NeedsRehash`, `AuthService.upgradeHash`) — пароль при этом не меняется,
поэтому срок его действия не сбрасывается.

`PASSWORD_TTL` задаёт срок жизни пароля — сутки с суффиксом `d` (`90d`, `1.5d`)
или любая единица `time.ParseDuration` (`2160h`); `0` или отрицательное значение
отключает проверку. Момент истечения считается от
`user_credentials.updated_at` — времени последнего изменения пароля.

Поток «пароль просрочен»:

1. `POST /auth/login` — пароль верный, но срок истёк → `403`
   `{"error":"password expired","code":"password_expired"}`; токен не выдаётся.
2. Клиент отправляет пользователя на смену пароля: `POST /auth/password` с
   `email`, `current_password`, `new_password`. Токен не нужен — личность
   подтверждает текущий пароль (именно поэтому ручка работает и без сессии).
3. Успех → `200` с новой сессией; срок действия пароля начинается заново
   (`updated_at` перезаписывается временем смены).

Автотесты:

```sh
go test ./internal/model/ ./internal/service/ ./internal/repository/ \
  ./cmd/bootstrap/ -run 'TestNeedsRehash|TestCredentials|TestAuthService|TestBootstrap|TestRun' -v
```

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
* `GET /users/me` — профиль действующего пользователя (JSON-ручка приложения);
* `POST /api/v1/users`, `GET|PATCH|DELETE /api/v1/users/{uuid}` — grpc-gateway
  поверх `user.v1.UserService` (в proto пути заданы от корня: `/users`,
  `/users/{uuid}`, префикс `/api/v1` добавляет монтирование gateway);
* `POST /api/v1/auth/{register,login,password,logout}`, `GET /api/v1/auth/me` —
  grpc-gateway поверх `auth.v1.AuthService` (в proto пути заданы от корня:
  `/auth/register` и т.д., префикс `/api/v1` добавляет монтирование gateway);
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
grpcurl -plaintext -d '{"email":"ivan@example.com","password":"secret-password"}' \
  localhost:50051 auth.v1.AuthService/Login
```

Автотесты транспорта: `go test ./internal/servers/ -run TestSmoke -v` —
поднимают приложение целиком (SQLite во временном файле, gRPC-сервер по сети и
HTTP через gateway) и проверяют сценарий целиком: create → get → update → delete,
конфликт уникальности и отсутствующие записи.

Смок-тест ручек авторизации по HTTP (curl): `BASE_URL=http://localhost:8080 tests/smoke/auth.sh`
проверяет регистрацию, вход, профиль, смену пароля и выход, а также коды ответов
(400/401/409) и обратную совместимость прежних JSON-ручек `/auth/*`.

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

Записи, которые логируются с контекстом запроса (`slog.InfoContext`,
`slog.ErrorContext` и другие), дополнительно получают атрибут `trace_id`
активной трассы: его добавляет обработчик `pkg/logger.TraceHandler`,
оборачивающий вывод в stdout. Так лог в stdout можно связать с трассой в
коллекторе. Атрибут появляется, когда `trace_id` присутствует в контексте
(трассировка инициализирована) и запись логируется с контекстом: вне активной
трассы `trace_id` не выводится. Поэтому HTTP-запрос без входящего `traceparent`
логируется без атрибута: серверный спан для него не стартует, и сгенерированный
`trace_id`, по которому в коллекторе нет трассы, в лог не попадает (см.
«Трассировка»). `span_id` в логи не пишется: для связки записи с трассой
достаточно `trace_id`. Значения берутся только из контекста активного спана; для
OTLP-логов те же идентификаторы добавляет SDK логов OpenTelemetry, поэтому
отдельно в экспортируемые записи они не пишутся.

Проверить связку лога с трассой можно запросом с заголовком `traceparent`
(например, `POST /auth/register`): `trace_id` в записи совпадёт с `trace_id` из
входящего заголовка.

```json
{"time":"2026-10-14T02:00:00Z","level":"INFO","msg":"user created","uuid":"...","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"}
```

Автотесты: `go test ./pkg/logger/ -run 'TestTraceHandler|TestInitHandler' -v` —
проверяют подстановку `trace_id` из контекста, отсутствие атрибута вне трассы,
сохранение готового значения и отсутствие `span_id`.

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

### Логи запросов (access-логи)

Каждый запрос логируется отдельной структурированной записью независимо от
транспорта — поле `protocol` различает `http` и `grpc`. HTTP-записи делаются в
контексте серверной трассы, поэтому содержат `trace_id`, когда запрос продолжает
входящую трассу (`traceparent`) или включён `ENABLE_FORCE_TRACE`; без этого запрос
обслуживается вне трассировки, и атрибута в записи нет (см. раздел «Логи»).
gRPC-записи `trace_id` не содержат: серверный спан открывает только
HTTP-middleware (см. раздел «Трассировка»).

| Поле | HTTP | gRPC |
| --- | --- | --- |
| `protocol` | `http` | `grpc` |
| `method` | HTTP-метод | полное имя метода (`/user.v1.UserService/Get`) |
| `path` | путь запроса | — |
| `status` | код ответа | — |
| `code` | — | gRPC-код (`OK`, `Unauthenticated`, …) |
| `bytes` | размер тела ответа | — |
| `remote_addr` | адрес клиента | адрес клиента |
| `user_agent` | User-Agent | — |
| `duration` | время обработки | время обработки |

Уровень записи: бизнес-запросы — `INFO`, служебные (`/health`, Swagger UI,
`grpc.health.v1.Health`, `grpc.reflection`) — `DEBUG`, чтобы пробы и браузер не
зашумили лог (виден только при `LOG_LEVEL=DEBUG`). Тела запроса и ответа не
логируются: в них бывают пароли и персональные данные.

HTTP-middleware стоит между трассировкой и аутентификацией, а gRPC-интерцептор —
снаружи проверки токена, поэтому в лог попадают и отклонённые запросы (`401`,
`Unauthenticated`). Реализация — `internal/servers/accesslog.go`
(`logMiddleware`, `NewLogInterceptor`). Автотесты:
`go test ./internal/servers/ -run 'TestLogMiddleware|TestNewLogInterceptor|TestIsServicePath' -v`.

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
`internal/servers/tracing.go`:

| Сценарий | Что делает | Реализация |
| --- | --- | --- |
| Force Trace | записывает все спаны, независимо от входящего контекста | `ENABLE_FORCE_TRACE=true`, правило в `traceSampler` (`pkg/observability`) |
| Context Propagation | продолжает входящую трассу по заголовку `traceparent` (W3C Trace Context) | `traceMiddleware` (`startServerSpan`), `otel.GetTextMapPropagator().Extract()` |
| App Startup | открывает спан `app.startup` в новой трассе и закрывает его, когда HTTP- и gRPC-серверы подняты | `startStartupSpan` в `ServerAPI.Run()` |
| App Shutdown | перехватывает SIGINT/SIGTERM, открывает спан `app.shutdown` в новой трассе и закрывает его перед выходом процесса | `startShutdownSpan` в `ServerAPI.Run()` |

Стартовый и завершающий спаны начинают собственную трассу: SDK генерирует
отдельный TraceID, поэтому запуск и остановка сервиса видны в коллекторе
самостоятельными трассами. Запрос без `traceparent` новую трассу не заводит:
серверный спан не стартует вовсе, поэтому в логах запроса нет `trace_id`, который
ссылался бы на отсутствующую в коллекторе трассу. Форс-трассировка
(`ENABLE_FORCE_TRACE=true`) включает запись всех запросов — спан стартует и без
заголовка.

Сквозная проверка трассы:

```sh
curl -s -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" \
  localhost:8080/health
```

Автотесты: `go test ./pkg/observability/ ./internal/servers/ -run
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
