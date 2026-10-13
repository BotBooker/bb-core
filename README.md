# bb-core

[![codecov](https://codecov.io/gh/BotBooker/bb-core/graph/badge.svg?token=S7CGSXNE61)](https://codecov.io/gh/BotBooker/bb-core)

Backend‑часть сервиса BotBooker — платформы для бронирования услуг через чат‑ботов.

## Запуск

Точка входа `cmd/api` поднимает HTTP- и gRPC-серверы с одним и тем же API
(`internal/servers`): gRPC отдаёт proto-сервисы, HTTP — grpc-gateway поверх них.

```sh
DSN=sqlite:/tmp/database.db go run ./cmd/api   # миграции применятся при старте
```

| Переменная | По умолчанию | Назначение |
| --- | --- | --- |
| `SERVER_HOST`, `SERVER_PORT` | `localhost`, `8080` | HTTP: `/health`, `/api/v1`, `/api/swagger/` |
| `SERVER_GRPC_HOST`, `SERVER_GRPC_PORT` | `localhost`, `50051` | gRPC: `user.v1.UserService`, health, reflection |
| `DSN` | — | подключение к БД; диалект (postgres/sqlite) определяется по DSN |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | адрес OTel Collector для всех сигналов |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | — | адрес OTel Collector для логов |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | — | адрес OTel Collector для метрик |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | — | адрес OTel Collector для трасс |
| `ENABLE_FORCE_TRACE` | `false` | `true` — записывать все трассы (форс-трассировка) |

```sh
curl -s localhost:8080/health
grpcurl -plaintext localhost:50051 list
```

Подробности по наблюдаемости — в [docs/HOWTO.md](docs/HOWTO.md).
