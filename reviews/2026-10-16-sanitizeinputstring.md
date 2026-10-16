# Ревью: `fix(core): input new rules for SanitizeInputString`

* **Ветка:** `fix/input_new_rules_for_sanitizeinputstring`
* **Коммит:** `122e9b3` (родитель — `b24347e`, `main`)
* **Дата ревью:** 2026-10-16
* **Объём:** 11 файлов в коммите + проверка всего репозитория на соответствие правилам `.clinerules/code.md` и `.clinerules/logs.md`
* **Артефакты проверки:** `make build`, `make test`, `make coverage`, `go vet ./...`, `golangci-lint run ./...`, `bearer s .`, `govulncheck ./...`

## Что меняет коммит

* `pkg/tools/string.go` — `SanitizeString` переименована в `SanitizeInputString`; вызовы и тесты обновлены.
* `internal/servers/accesslog.go` — атрибуты access-лога переведены на типизированные `slog.String/Int/Duration`; значения `protocol` и тексты сообщений стали
  `HTTP`/`gRPC`; `path` и `user_agent` прогоняются через `SanitizeInputString`.
* `internal/servers/auth.go`, `internal/model/user.go` — переименование вызовов санитайзера.
* `pkg/observability/{logs,metrics,tracing}.go` — в записях о включении экспорта `protocol` стал `gRPC`.
* `Makefile` — `build` теперь DEV-сборка, `build-release` — релизная (`build-debug` удалён).
* `docs/specification/programming.md` — новый файл-заглушка (KISS/SOLID).

## Итоговый вердикт

| # | Требование | Вердикт |
| --- | --- | --- |
| 1.1 | Код чистый, соответствует стилю проекта | ✅ PASS (замечания P3–P4) |
| 1.2 | Нет известных CVE | ✅ PASS |
| 1.3 | Код документирован (сложная логика, публичные API) | ⚠️ PASS с замечанием (P3) |
| 1.4 | Обработка ошибок корректна, сообщения осмысленны | ❌ FAIL (P2) |
| 2.1 | OpenTelemetry внедрён полностью (traces + структурные логи) | ✅ PASS |
| 2.2 | Ключевые метрики экспортируются в Prometheus | ❌ FAIL (P2) |
| 2.3 | Логи содержат контекст (trace ID, request ID) | ⚠️ PASS частично (P3) |
| 3.1 | Покрытие бизнес-логики ≥ 80 % | ✅ PASS (88.6 % всего) |
| 3.2 | Есть unit- и интеграционные тесты | ✅ PASS |
| 3.3 | `make test` выполняется без ошибок | ✅ PASS |
| 3.4 | `make coverage` формирует отчёт | ✅ PASS (88.6 %, exit 0) |
| 4.1 | `make build` проходит успешно | ✅ PASS |
| 4.2 | Docker-образ на минимальной базе, без критичных уязвимостей | N/A — Dockerfile/`docker-compose` в репозитории нет |
| 4.3 | CI проходит все проверки (lint, tests, security) | ❌ FAIL (P2) |
| 5.1 | `make lint` без ошибок и предупреждений | ✅ PASS |
| L.1 | Логи: санитизация ввода, отсутствие секретов | ❌ FAIL (P2) |
| L.2 | Логи: состояние OTel-экспорта только на уровне `Debug` | ❌ FAIL (P2) |
| L.3 | Логи: `context.Context` передаётся в логгер | ❌ FAIL (P3) |

## 1. Качество кода

### 1.1 Чистота, структура, стиль — PASS

* Рефакторинг переименования выполнен полностью: поиск `SanitizeString\b` по репозиторию не находит ни одного старого вызова,
  `SanitizeInputString` используется в проде (`auth.go`, `accesslog.go`, `model/user.go`) и в тестах.
* `go vet ./...` — без замечаний; форматтеры `gofmt/gofumpt/goimports` (в составе golangci-lint) — без замечаний.
* Комментарии в стиле проекта: русский, объясняют «почему», а не «что»; пакеты документированы.

Замечания (не блокируют):

* **P3.** `pkg/tools/string.go` совмещает две обязанности (маскирование секретов и чистку для логов) и принимает вариативный `key ...string`,
  хотя использует только `key[0]` и молча игнорирует остальные. Читалось бы лучше как `SanitizeForLog(value string)` + `MaskIfSensitive(key, value string)`.
* **P4.** Непоследовательные типы атрибутов: в `accesslog.go` теперь `slog.String/Int/Duration`, а в `pkg/observability/logs.go:48`,
  `metrics.go:51`, `tracing.go:68` — `slog.String("endpoint", endpoint)` рядом с нетипизированным `"protocol", "gRPC"`. Стоит привести к одному виду.

### 1.2 CVE — PASS

```text
$ govulncheck ./...
No vulnerabilities found.
Your code is affected by 0 vulnerabilities.
This scan also found 0 vulnerabilities in packages you import and 1
vulnerability in modules you require, but your code doesn't appear to call these
vulnerabilities.
```

Одна уязвимость числится в модуле-зависимости, но из кода недостижима. Рекомендация: убрать её плановым bump зависимостей.

### 1.3 Документирование — PASS с замечанием

* Публичные функции и сложная логика (трассировка, обработчики логов, мост `otelslog`, сэмплер, `TraceHandler`) документированы подробно.
* **P3.** `docs/specification/programming.md` — заглушка: заголовок, «1. Principles:» и два пункта `KISS`/`SOLID`, пункта 2 нет.
  Либо наполнить содержательно (правила санитизации ввода, работа с секретами, требования к логированию), либо убрать: документ из 6 строк
  создаёт ложное впечатление о наличии спецификации.
* **P3.** Сами правила (`sanitize user input`, `Debug` для телеметрии) живут в `.clinerules/`, а `.clinerules/` перечислен в `.gitignore`.
  Правила не версионируются и не видны CI и остальным разработчикам — их место в `docs/specification/` или `CONTRIBUTING.md`.

### 1.4 Обработка ошибок — FAIL

**P2. Санитайзер применяется к данным, которые дальше используются как учётные — `internal/servers/auth.go:267`:**

```go
func parseBearer(value string) (string, bool) {
	value = tools.SanitizeInputString(value) // <-- изменяет креденшл
	if len(value) <= len(bearerPrefix) || !strings.EqualFold(value[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(value[len(bearerPrefix):])

	return token, token != ""
}
```

`SanitizeInputString` без ключа делает ровно две вещи: заменяет `\n`/`\r` на пробел и **обрезает значение до 500 байт**, добавляя `... [truncated]`.
Значит валидный `Authorization` длиннее 500 байт будет изуродован ещё до проверки подписи и превратится в `401`.

Оценка риска: с дефолтной конфигурацией JWT ≈ 355 байт (issuer `bb-core`, claims `sub`/`tenant`/`jti` — UUID), то есть сегодня баг не проявляется.
Но issuer задаётся окружением (`AUTH_JWT_ISSUER`, по умолчанию `bb-core` — `internal/config/config.go:90`), а состав claims может вырасти:
при заголовке длиннее 500 байт вход перестанет работать молча и только у части клиентов. Санитизация — операция уровня *логирования*,
на пути проверки креденшла её быть не должно.

Как исправить:

```go
// parseBearer выделяет токен из значения заголовка как есть.
func parseBearer(value string) (string, bool) {
	// Ограничение защищает от DoS, но не искажает значение: слишком длинный
	// заголовок отвергается, а не обрезается.
	const maxAuthorizationLength = 4096
	if len(value) > maxAuthorizationLength {
		return "", false
	}

	// ... остальная логика без изменений
}
```

Санитайзер применять только там, где значение попадает в лог, и с ключом `authorization` — тогда вернётся `***`.

Дополнительно (P3): `internal/model/user.go:81` санитизирует значение, которое затем попадает в `err.Error()` и возвращается клиенту
(`writeError(w, http.StatusBadRequest, err.Error())`). Для ответа клиенту лучше стабильное сообщение, а санитизированное значение — только в лог.

## 2. Наблюдаемость

### 2.1 OpenTelemetry (traces + структурные логи) — PASS

* `pkg/observability` настраивает три сигнала (traces, metrics, logs) по OTLP/gRPC; каждый включается только при заданном адресе коллектора,
  иначе глобальные провайдеры остаются no-op (`observability.go:27-96`).
* Логи: `logger.SetOTelHandler(otelslog.NewHandler(...))` + `MultiHandler`, вывод в stdout сохраняется; `setupLogs` вызывается после
  `logger.InitAndSetDefault` (`internal/servers/server.go:36` → `Run()` → `SetupOTelSDK`), поэтому OTel-обработчик не теряется.
* Трассировка: W3C Trace Context + Baggage, серверный спан в `traceMiddleware`, кастомный сэмплер (форс-режим, продолжение удалённой трассы,
  стартовый/финальный спаны), `otelpgx` для запросов к БД.
* Завершение: `otelShutdown` вызывается с бюджетом 5 с и не влияет на код возврата процесса.

### 2.2 Метрики в Prometheus — FAIL

* В коде нет ни Prometheus-экспортёра, ни эндпоинта скрейпинга: зависимость `github.com/prometheus/client_golang` в `go.mod` отсутствует,
  поиск по `prometheus|promhttp|/metrics` даёт только упоминания в правилах и документации. Метрики уходят **только** push-ом по OTLP/gRPC
  (`pkg/observability/metrics.go`), то есть требуют OTel Collector.
* Более того, приложение не создаёт **ни одной** метрики: `otel.GetMeterProvider()` только устанавливается, инструментов
  (`Int64Counter`, `Float64Histogram`, `RecordStats` и т. п.) в коде нет — `otelpgx` подключён исключительно как трассировщик
  (`internal/database/postgres/postgres.go:100`). «Ключевые метрики» в смысле требования отсутствуют.

Как исправить (выбрать один из вариантов):

1. Добавить Prometheus-экспортёр OTel (`go.opentelemetry.io/otel/exporters/prometheus`) и отдавать `promhttp.Handler()` на отдельном
   `/metrics` (например, на служебном порту, недоступном извне).
2. Документировать, что Prometheus скрейпит OTel Collector (в репозитории `otel/customconfig.yaml` собирает OTLP и печатает в debug —
   без Prometheus-экспортёра), и добавить туда `prometheus` exporter.

В любом случае стоит добавить сами метрики: счётчик запросов/ошибок и гистограмму длительности (HTTP + gRPC) либо `otelhttp`/`otelgrpc`-инструментацию.

### 2.3 Логи с контекстом — PASS частично

* `trace_id` и `span_id` активного спана добавляет `pkg/logger.TraceHandler` (`pkg/logger/trace_handler.go:69`), вне трассы атрибуты не появляются;
  поведение покрыто тестами, `coverage: pkg/logger 95.7 %`.
* **P3.** `request_id` в логах нет вообще: middleware, который генерирует/пробрасывает идентификатор запроса, в проекте отсутствует.
  Требование «Logs include context (trace ID, request ID)» выполнено только наполовину. Минимальный шаг — middleware, который
  читает `X-Request-ID` (или генерирует UUID) и кладёт его в контекст, а `TraceHandler`/отдельный `slog`-атрибут добавляет его в запись.

## 3. Тестирование

### 3.1 Покрытие — PASS

`make test` пересобрал `coverage.txt`; итог `make coverage` — **88.6 %**, порог не ниже 80 % выдержан. Покрытие по пакетам (свежий отчёт):

| Пакет | Покрытие |
| --- | --- |
| `internal/service` | 85.2 % |
| `internal/servers` | 89.8 % |
| `internal/repository` | 93.0 % |
| `internal/model` | 95.6 % |
| `internal/jwt` | 94.8 % |
| `pkg/tools` | 100 % |
| `pkg/logger` | 95.7 % |
| `pkg/observability` | 96.5 % |
| `internal/database/postgres` | 59.8 % (локально без `PG`) |
| `internal/database/sqlite` | 78.0 % |

Ниже 80 % только драйверные пакеты БД: их интеграционные тесты пропускаются без `PG`/`REDIS_HOST`, а в CI переменные заданы
(`.github/workflows/go.yml`), поэтому в CI покрытие выше. Ключевая для этого коммита функция `SanitizeInputString` — 100 %, `parseBearer` — 100 %.

**Пробел в тестах (P3):** ни один тест не проверяет поведение `parseBearer` на длинном заголовке (>500 байт) — то есть баг из 1.4 не поймался бы;
также нет теста, что `path`/`user_agent` в access-логе действительно очищены от переводов строк (тестируется только сама `SanitizeInputString`).

### 3.2 Unit- и интеграционные тесты — PASS

* Unit: табличные тесты почти для каждого пакета, подмена глобального логгера (`captureLogs`), фейковый OTLP-коллектор для logs/metrics/traces.
* Интеграционные: реальные Postgres/Redis в CI, smoke-скрипты `tests/smoke/auth.sh` и `tests/smoke/traceparent.sh`, e2e-прогон `cmd/bootstrap`, `cmd/migrate`.

### 3.3 `make test` — PASS

```text
$ make test
...
  ✅ TestSanitizeInputStringTruncationKeepsValidUTF8/4-byte_runes_cut_on_rune_boundary (0s)
  ✅ TestSanitizeInputStringTruncationKeepsValidUTF8/mixed_ascii_and_multibyte_rolled_back (0s)
EXIT=0
```

### 3.4 `make coverage` — PASS

```text
$ make coverage
coverage at: 88.6%
EXIT=0
```

Замечание (P4): цель `coverage` не регенерирует отчёт — она только считает процент по уже существующему `coverage.txt`
(и вызывает `make test`, только если файла нет). Плюс проверка порога опирается на `bc` и конвейер `xargs`; логику проще и надёжнее перенести
в `go tool cover` + маленький скрипт на Go.

## 4. Сборка и деплой

### 4.1 `make build` — PASS

```text
$ make build
go build -ldflags='-X "main.version=dev built at 2026-10-16T04:41:47 on ... with go version go1.26.9 darwin/amd64"' -o ./build/api ./cmd/api
$ ls -la build/
-rwxr-xr-x 1 user staff 40957728 build/api
```

**P3.** Смысл цели изменился: раньше `build` собирал релизный бинарник, теперь — DEV, а релиз переехал в `build-release`.
Ссылок на `make build` в репозитории нет (`grep` по README/docs/CI чист), но если внешний пайплайн или разработчик вызывает `make build`,
ожидая релизную сборку, он молча получит dev-бинарник с `-X main.version=...`. Стоит отразить новую семантику в `README.md`/`docs/HOWTO.md`.

### 4.2 Docker — N/A

В репозитории нет `Dockerfile` и `docker-compose.yml` (поиск по корню и `find` по дереву). Есть только `trivy-scan` в режиме `fs` (vuln, secret, misconfig),
поэтому требование «минимальная база, без критичных CVE» неприменимо. Когда образ появится — многоступенчатая сборка `golang:1.26-alpine` → `scratch`/`alpine`
и сканирование образа в CI (сейчас сканируется файловая система, а не образ).

### 4.3 CI — FAIL

* **P2. Фильтры путей не покрывают изменённые директории.** `.github/workflows/go.yml` запускается только при изменениях
  `go.mod`, `go.sum`, `cmd/**`, `internal/**`, `.github/workflows/**`. В этом коммите затронуты `pkg/tools`, `pkg/observability` и `Makefile` —
  значит lint и тесты для этого PR **не запустятся**, хотя меняется продовый код. Исправление: добавить `pkg/**`, `tools.mk`, `Makefile`, `tests/**`
  (или убрать фильтр `paths` вовсе).
* **P2. `GO_TESTTAGS` — мёртвая переменная.** Workflow выполняет `GO_TESTTAGS='-failfast -shuffle=off' make coverage`, но Makefile использует
  `GO_TEST_ARGS` (`GO_TESTARGS`-подобной переменной нет). Флаги молча игнорируются, и `make test` всегда идёт с `-shuffle=on -v`.
  Исправление: либо пробросить `GO_TEST_ARGS` в workflow, либо поддержать `GO_TESTTAGS` в Makefile.
* **P3. Версия Go в lint-джобе** (`go-version: "1.27.2"`) выше, чем в `go.mod` и `.go-version` (`1.26.9`); матрица тестов проверяет обе версии,
  поэтому расхождение стоит зафиксировать осознанно (например, `GO_VERSION` из `.go-version` + отдельная джоба на «latest»).
* Прочее в порядке: проверки безопасности присутствуют (`codeql.yml`, `trivy-scan.yml`, `bearer` в lint-джобе, `govulncheck` — только через `make go-check`).

## 5. Линтинг

### 5.1 `make lint` — PASS

```text
$ golangci-lint run ./...      # без --fix: только проверка
0 issues.
EXIT=0

$ bearer s .
72 checks were run and no failures were detected. Great job! 👏
EXIT=0
```

Замечания (не влияют на вердикт): цель `lint` запускает `golangci-lint run --fix`, то есть в CI правит рабочую копию (полезно только локально,
а в пайплайне маскирует будущие ошибки — лучше `run` без `--fix`); `bearer s .` требует установленного бинарника и токена/аккаунта для части правил,
в чистом окружении `make lint` может упасть не по вине кода.

## 6. Правила логирования (`.clinerules/logs.md`)

### L.1 Санитизация ввода и секреты — FAIL (P2)

* Что сделано хорошо: `path` и `user_agent` в `accesslog.go:81,85` прогоняются через `SanitizeInputString`; пароль/токен из `Authorization`
  в логи не пишутся; `redactAddr` скрывает пароль Redis-адреса (`internal/cache/cache.go:33`, покрыт тестом); текст `401/403` фиксированный.
* **Нарушение 1 (P2).** `internal/servers/auth.go:268` — санитайзер применён к учётным данным (см. 1.4). По духу правила санитизация — это фильтр
  *перед логированием/передачей вовне*, а здесь она вклинилась в разбор креденшла.
* **Нарушение 2 (P2).** `internal/cache/cache.go:54` пишет содержимое кэша без санитизации и без ключей атрибутов:
  `slog.Debug("cache.Set", key, value)`. Туда попадут произвольные пользовательские значения (в т. ч. персональные данные и токены сессий).
  Ожидаемый вид:
  `slog.Debug("cache.Set", slog.String("key", tools.SanitizeInputString(key, "key")), slog.String("value", tools.SanitizeInputString(value, "value")))`
  — или, что надёжнее, вообще не логировать значение, только длину/факт записи.
* **Нарушение 3 (P3).** В access-логе санитизированы не все значения: `r.Method`, `r.RemoteAddr`, `info.FullMethod`, `peerAddr(ctx)` пишутся как есть.
  Сейчас риск низкий (значения формируются HTTP/2-стеком, а не телом запроса), но правило требует санитизировать любые данные, пришедшие от клиента,
  и стоит соблюдать его одинаково — либо явно задокументировать, почему эти поля считаются доверенными.

### L.2 Состояние OTel-экспорта только на `Debug` — FAIL (P2)

Правило прямо требует: «Categorize all informational data regarding OpenTelemetry exports (enabled/disabled states) strictly as `Debug` level logs».
В коде три записи о **включении** экспорта идут уровнем `Info` (строки затронуты этим коммитом):

* `pkg/observability/logs.go:48` — `slog.Info("otlp logs export enabled", ...)`
* `pkg/observability/metrics.go:51` — `slog.Info("otlp metrics export enabled", ...)`
* `pkg/observability/tracing.go:68` — `slog.Info("otlp traces export enabled", ...)`

При этом симметричные записи о **выключении** — на `Debug`. Исправление: заменить `slog.Info` на `slog.Debug` в трёх местах
(например, `slog.Info("otlp traces export enabled", slog.String("endpoint", endpoint), "protocol", "gRPC")` → `slog.Debug(...)`).
Тесты `pkg/observability/*_test.go` проверяют факт экспорта, а не уровень, поэтому правка безопасна; `docs/HOWTO.md` на уровень лога не опирается.

### L.3 `context.Context` в логгере — FAIL (P3)

* `pkg/observability/{logs,metrics,tracing}.go` — `slog.Debug/Info` без контекста (контекст есть в аргументах `setupTraces/setupMetrics/setupLogs`),
  поэтому `trace_id` в эти записи не попадёт.
* `internal/closer/closer.go:100,109,113,122,126` — `slog.Debug/Error` вместо `slog.ErrorContext(ctx, ...)`, хотя `ctx` есть в `closeAll`: логи
  завершения приложения теряют связь с трассой shutdown, которую сам код и открывает (`startShutdownSpan`).
* `internal/cache/cache.go` — методы интерфейса не принимают `ctx` вовсе; если кэш станет реальным Redis-клиентом, контекст понадобится и для вызовов, и для
  логов.

Исправление: `slog.DebugContext(ctx, ...)`, `slog.ErrorContext(ctx, ...)`, `slog.InfoContext(ctx, ...)`; отдельный `slog.Attr`/хелпер для `request_id`.

## 7. Сводка замечаний

| Приоритет | Замечание | Файлы |
| --- | --- | --- |
| P2 | Санитайзер на пути разбора `Authorization` (обрезка токена >500 байт → 401) | `internal/servers/auth.go:268` |
| P2 | Записи о включении OTel-экспорта на уровне `Info` вместо `Debug` | `pkg/observability/logs.go:48`, `metrics.go:51`, `tracing.go:68` |
| P2 | Нет Prometheus-экспорта и самих метрик (только OTLP-push, meter provider — no-op) | `pkg/observability/metrics.go`, `internal/servers/routes.go` |
| P2 | CI не запускается на изменения `pkg/**` и `Makefile` | `.github/workflows/go.yml:5-23` |
| P2 | `GO_TESTTAGS` из workflow игнорируется Makefile | `.github/workflows/go.yml:125`, `Makefile:41-47` |
| P2 | `cache.Set` логирует значение кэша без санитизации и ключей атрибутов | `internal/cache/cache.go:54` |
| P3 | Нет `request_id` в логах | `internal/servers/accesslog.go`, `pkg/logger` |
| P3 | Логи без контекста в observability и closer | `pkg/observability/*.go`, `internal/closer/closer.go` |
| P3 | `docs/specification/programming.md` — заглушка; правила в `.clinerules/` не версионируются (`.gitignore`) | `docs/specification/programming.md`, `.gitignore` |
| P3 | Изменён смысл `make build` (dev вместо release) без обновления документации | `Makefile:110-120` |
| P3 | Нет тестов на длинный `Authorization` и на санитизацию `path`/`user_agent` в access-логе | `pkg/tools/string_test.go`, `internal/servers/accesslog_test.go` |
| P4 | Смешанные типизированные/нетипизированные атрибуты в `slog` | `pkg/observability/*.go` |
| P4 | `make coverage` только считает процент, не регенерирует отчёт; порог считается через `bc` | `Makefile:50-58` |

## 8. Приложение: выполненные проверки

```text
go build ./...                    -> OK
go vet ./...                      -> без замечаний
make build                        -> OK (build/api, ~40 MB)
make test                         -> EXIT=0 (все пакеты ok)
make coverage                     -> 88.6 %, EXIT=0
golangci-lint run ./...           -> 0 issues, EXIT=0
bearer s .                        -> 72 checks, no failures, EXIT=0
govulncheck ./...                 -> 0 reachable vulnerabilities, EXIT=0
```

Окружение проверки: `go1.26.9 darwin/amd64`, ветка `fix/input_new_rules_for_sanitizeinputstring`, коммит `122e9b3`.
Локально без `PG`/`REDIS_HOST` интеграционные тесты БД/кэша пропускаются, поэтому `internal/database/postgres` локально даёт 59.8 % —
в CI эти переменные заданы.

## 9. План правок (для применения в интерактивном режиме)

1. `internal/servers/auth.go` — убрать `SanitizeInputString` из `parseBearer`, отвергать слишком длинный заголовок вместо обрезки;
   добавить тест на длинный и «грязный» заголовок.
2. `pkg/observability/{logs,metrics,tracing}.go` — понизить записи о включении экспорта до `Debug`, перейти на `*Context`-варианты логгера.
3. `internal/cache/cache.go` — не логировать значение кэша; использовать ключи атрибутов и санитизацию (или `ctx`-варианты логгера).
4. `.github/workflows/go.yml` — добавить `pkg/**`, `Makefile`, `tools.mk`, `tests/**` в `paths`; починить передачу тестовых флагов (`GO_TEST_ARGS`).
5. Метрики — решить, чей это скоуп (эндпоинт `/metrics` в приложении или Prometheus-экспортёр в коллекторе) и добавить ключевые счётчики/гистограммы.
6. Документация — убрать или наполнить `docs/specification/programming.md`, перенести правила из `.clinerules/` в версионируемый документ,
   описать новую семантику `make build` / `make build-release`.

## 10. Зафиксированные решения

### П.1 `parseBearer` — решено 2026-10-16

**Решение:** убрать `SanitizeInputString` из `parseBearer`; добавить явный *отказ* по длине значения (не обрезку) с лимитом 8 КиБ;
ограничить заголовки HTTP-сервера через `MaxHeaderBytes` (32 КиБ). `grpc.MaxHeaderListSize` — отдельным пунктом, вне этого фикса.

**Обоснование:** токен нигде не логируется, поэтому санитизация здесь ничего не защищает, а её единственный наблюдаемый эффект — обрезка значения
до 500 байт (JWT ≈ 355 байт при дефолтных claim'ах, запас небольшой). Защита от log-injection не нужна: `net/http` отклоняет значения заголовков
с управляющими символами (`server.go:1026`, `h2_bundle.go:3230`, `11256`). `MaxHeaderBytes` управляет и HTTP/1.1, и HTTP/2 в `net/http`
(`h2_bundle.go:4708-4714`), поэтому одно поле закрывает оба случая; сейчас поле не задано и действует дефолт 1 МБ, а gRPC-сервер без опций
допускает до 16 МБ заголовков (`defaultServerMaxHeaderListSize = 16 << 20`, grpc-go v1.84.0). Цифры: 8 КиБ — запас >20× к реальному токену
и совпадение с будущим дефолтом grpc-go и дефолтом nginx; 32 КиБ на все заголовки запроса безопасно.

**План правок:** `internal/servers/auth.go` — константа `maxAuthorizationLength = 8 << 10` и проверка перед разбором; `internal/servers/server.go` —
`MaxHeaderBytes: 32 << 10`; `strings.TrimSpace` сохраняется (OWS после схемы разрешает RFC 6750).

**Тесты:** новый табличный `TestParseBearer` — регистронезависимая схема, пустое значение, значение без схемы, пробелы вокруг токена,
регрессия «токен 600 символов возвращается без изменений», лимит «значение >8 КиБ → `false`».

### П.2 Записи о состоянии OTel — решено 2026-10-16

**Решение:** правило меняем, а не код. Одна запись «export enabled» на сигнал остаётся на уровне `Info`, всё остальное про экспорт — `Debug`.

**Внесено в `.clinerules/logs.md`:**

* §2: «enabled»-состояние сигнала разрешено на `Info` (ровно одна запись на сигнал при старте, с адресом коллектора); «disabled»-состояние,
  детали ошибок экспортёра, диагностика по батчам/экспортам и дампы атрибутов — только `Debug`, в Info/Warn/Error не направляются.
* §3: «Debug Level for Telemetry» переформулирован в «state *diagnostics*», ссылка на несуществующий `logger.DebugCtx()` заменена на
  `slog.DebugContext(ctx, ...)`; зафиксировано исключение про одну Info-запись (отсылка к §2).
* Пример в конце файла дополнен корректной Info-записью и оставшейся Debug-диагностикой.

**Следствие:** нарушение L.2 в отчёте считается снятым правкой правила (записи `logs.go:48`, `metrics.go:51`, `tracing.go:68` остаются как есть).
**Открыто:** `.clinerules/` перечислен в `.gitignore`, поэтому правка правила живёт только локально — нужно решение, куда переносить
версионируемую копию правил (см. п.6). Часть L.3 про `context.Context` (в `pkg/observability`, `internal/closer`, `internal/cache`) остаётся открытой.

### П.2.2 Метрики — решено 2026-10-16

**Решение:** в рамках этой работы добавляем только метрики в коде (пул БД, HTTP, gRPC); контур Prometheus выносим отдельной задачей.

**План правок:**

* Пул БД: `otelpgx.RecordStats(pool)` в `internal/database/postgres/postgres.go` (`defaultConnector`) — даёт 13 асинхронных инструментов
  (`pgxpool.acquires`, `acquire_duration`, `idle/max/total_connections`, `empty_acquire_wait_time` и др.). Вызывать строго после
  `SetupOTelSDK`: `RecordStats` берёт `otel.GetMeterProvider()` в момент вызова (`otelpgx/meter.go:48`), иначе забиндится на no-op супервайзер.
  В `internal/servers/server.go` порядок уже правильный (`SetupOTelSDK` → `initDeps`). Ошибку возврата обрабатывать.
* HTTP: middleware метрик рядом с `logMiddleware` в `internal/servers/routes.go` (цепочка `traceMiddleware(logMiddleware(h.authenticate(mux)))`).
* gRPC: новый интерцептор в цепочку `grpc.ChainUnaryInterceptor` (`internal/servers/grpc.go:64`), метка — `info.FullMethod`.
* Метки маршрутов: использовать нормализованный маршрут, а не `r.URL.Path` (UUID в пути → взрыв кардинальности). `net/http` проставляет
  `Request.Pattern` на тот же объект запроса (`server.go:2846`), поэтому внутри цепочки доступен шаблон; у gateway-ручек Pattern = `/api/v1/`,
  то есть нужен общий хелпер нормализации, переиспользуемый и в имени серверного спана (`internal/servers/tracing.go:35` — сейчас туда попадает сырой путь).

**Отложено отдельной задачей (не входит в этот фикс):** Prometheus-контур — либо экспортёр `otel/exporters/prometheus@v0.69.0` + `/metrics`
в приложении (версия точно совместима с `otel/sdk v1.47.0`, зависимости есть в кэше модулей), либо `prometheus` exporter в `otel/customconfig.yaml`.
До реализации контура требование «метрики в Prometheus» остаётся формально невыполненным — метрики уходят только по OTLP.

**Вердикт по 2.2 в отчёте:** понижается с FAIL до «частично» (метрики появляются в коде, контур осознанно отложен); пункт остаётся в сводке как открытый.

### П.4.3 CI — решено 2026-10-16

**Решение (вариант C):**

* убрать фильтр `paths` и оставить только `paths-ignore` (`docs/**`, `**.md`) — совмещать `paths` и `paths-ignore` для одного события нельзя,
  поэтому фильтр должен быть ровно один; список путей невозможно «забыть» обновить;
* зафиксировать версию линтера: `version: v2.14` у `golangci-lint-action` либо `version-file` с единым источником (`tools.mk`) —
  сейчас вход не задан, и по исходникам экшена (`src/version.ts` v9.3.0) пустое значение означает `latest`, что расходится с локальным
  `GOLANGCI_LINT_VERSION ?= v2.14.0`;
* добавить шаг `govulncheck` (`make go-check`) — теперь проверка CVE есть не только локально;
* починить передачу тестовых флагов: workflow передаёт `GO_TESTTAGS`, а Makefile читает `GO_TEST_ARGS` — сейчас `-failfast -shuffle=off` игнорируются.

**Дополнительно (P4, по возможности в этой же правке):** lint-джоба берёт Go-версию из `.go-version` (сейчас `1.27.2` при `go.mod` `1.26.9`),
а проверка на будущей версии выносится отдельной опциональной ногой; установку инструментов вынести из `make test` или кэшировать `GOPATH/bin`
(сейчас `go install` ~15 бинарей выполняется на каждой из 4 ног матрицы); добавить `concurrency` в `trivy-scan.yml`.

### П.5 A1 `cache.Set` — решено 2026-10-16

**Решение:** не логировать значение кэша. В `internal/cache/cache.go:54` вместо `slog.Debug("cache.Set", key, value)` —
`slog.Debug("cache.Set", slog.String("key", key), slog.Int("value_len", len(value)))`.

**Обоснование:** содержимое кэша может быть чем угодно (токены сессий, PII), а правило §1 требует не передавать наружу raw-данные;
`key` и длина значения сохраняют диагностическую ценность. Реализация сейчас — заглушка (`Set` ничего не пишет, методы не вызываются
из продового кода, кроме DI), поэтому правка не влияет на поведение и не требует изменения тестов (`cache_test.go` логи не проверяет).

### П.5 A2 Поля access-лога — решено 2026-10-16

**Решение (вариант 2):** не санитизировать `method`, `remote_addr`, `peer_addr`, `info.FullMethod`, но задокументировать их как доверенные;
формулировку правила уточнить.

**Обоснование (проверено по исходникам):** `r.Method` валидируется `net/http` по RFC 7230 (`validMethod`, `request.go:844` — токен без CTL и разделителей);
`r.RemoteAddr` и `peerAddr(ctx)` берутся из соединения (`h2_bundle.go:4492`), а не из запроса; `info.FullMethod` — `stream.Method()`, но интерцептор вызывается
только для метода, найденного точным поиском в реестре сервисов (`server.go:1562-1597`), то есть значение всегда совпадает с зарегистрированным нами.
Дополнительно HTTP/2 не допускает CR/LF в значениях заголовков. Значит сегодня лог-инъекция через эти поля невозможна.

**Что сделать:**

* `internal/servers/accesslog.go` — комментарий у набора полей: перечисленные значения формируются транспортом (валидированы стеком или взяты из соединения),
  поэтому не санитизируются; санитизации подлежат только значения, пришедшие от пользователя (`path`, `user_agent`).
* `.clinerules/logs.md` §1 — уточнено: санитизируются значения, пришедшие от пользователя и передаваемые наружу; доверенные значения транспорта/фреймворка
  (метод, адрес клиента, маршрут или RPC-метод из реестра) не требуют санитизации, но должны быть помечены в коде как доверенные с объяснением.

**Обязательство на будущее:** если access-лог начнёт писать IP клиента из `X-Forwarded-For` (`HTTP_HEADER_CLIENT_IP`, `internal/config/config.go:43`) —
значение станет пользовательским и санитизация будет обязательной.

### П.5 A3 `context.Context` в логах — решено 2026-10-16

**Решение (вариант 2):** перевести на `*Context`-варианты логгера «группу 1» — все места, где `ctx` уже в области видимости (~27 вызовов):
`pkg/observability/{observability,logs,metrics,tracing}.go` (9), `internal/closer/closer.go` (5), `internal/servers/server.go` (9),
`internal/database/migrate.go` (3), `internal/database/sqlite/sqlite.go` (1), `internal/servers/di.go` (1). Сигнатуры не меняются.

**Польза по местам:** в `closer` логи закрытия ресурсов получат `trace_id` трассы `app.shutdown` (спан открыт в `server.go:183`);
в `server.Run` логи старта пишутся через `startupCtx` (спан активен до `startupSpan.End()`), логи остановки — через `shutdownTraceCtx`;
в `pkg/observability` и `internal/database` правка нужна только для единообразия — активного спана там нет (`SetupOTelSDK` вызывается до `startStartupSpan`).

**Тест:** новый тест в `internal/closer` — подменить глобальный логгер на `pkg/logger.TraceHandler` с записью в буфер, вызвать `CloseAll` с контекстом,
содержащим активный спан, и проверить, что в записи есть `trace_id`.

**Не входит (группа 2, ~20 вызовов, требует смены сигнатур):** `writeJSON`/`writeProtoJSON`, `toStatusError`, `database.New`, конструкторы
репозиториев/сервисов/шины/кэша, `jwt.NewSigner`, `pkg/tools/env`, `stopGRPC`, `cmd/api.Main` — отдельной задачей.

### П.6 B1 Правила и документация — решено 2026-10-16

**Решение (вариант A):** `docs/specification/programming.md` становится версионируемым источником истины по правилам разработки;
`.clinerules/{code,logs}.md` сокращаются до ссылки на него; в `README.md` добавляется раздел «Документация» со ссылками на спецификации.

**Планируемое содержание `programming.md`:** принципы (KISS/SOLID, слои `servers → service → repository → database`, зависимости внутрь);
логирование (выбор уровня, `*Context`-варианты, санитизация пользовательских значений и список доверенных полей, запрет сырых секретов,
одна Info-запись на сигнал телеметрии при старте, корреляция `trace_id`/`span_id`/`request_id`); тесты (покрытие ≥80 % бизнес-логики,
табличные тесты, изоляция внешних зависимостей, интеграционные под `PG`/`REDIS_HOST`); ошибки (`%w`, `errors.Is`, фиксированные тексты наружу);
сборка и проверки (семантика `make build`/`build-release`, `make lint`, `make go-check`).

**Обязательно перенести:** правки, внесённые в `.clinerules/logs.md` по решениям П.2 и П.5 A2 (Info для «export enabled», уточнение
«sanitize user input» про доверенные поля транспорта) — иначе локальная копия и документ разойдутся.

### П.6 B2 Семантика `make build` — решено 2026-10-16

**Решение:** задокументировать новые цели в `README.md` — таблица «Сборка» с `build` (DEV-бинарник с ldflags), `build-release` (статическая релизная
сборка), `run` и `debug`, с явным упоминанием, что `make build` больше не релизная сборка, а `build-debug` удалён. Раздел «Сборка» появится
в `docs/specification/programming.md` (см. П.6 B1) со ссылкой из README.

### П.6 C1–C4 Мелкие пункты — решено 2026-10-16

* **C1 (request_id) — не реализуем.** Вместо реализации middleware правило приведено в соответствие с кодом: `.clinerules/code.md` теперь
  требует `trace_id`/`span_id` активного спана (их добавляет `pkg/logger.TraceHandler`), а отдельный request ID прямо назван необязательным,
  поскольку middleware отсутствует. Требование «Logs include context (trace ID, request ID)» снято как неактуальное.
* **C2 (тест санитизации access-лога) — делаем:** тест в `internal/servers/accesslog_test.go` — путь с переводом строки не создаёт вторую запись,
  `path`/`user_agent` очищены.
* **C3 (типизированные атрибуты) — делаем:** `slog.String("protocol", …)` в `pkg/observability/{logs,metrics,tracing}.go` (3 строки).
* **C4 (`make coverage`) — отдельной задачей:** порог считается через `bc`/`xargs`, отчёт не регенерируется; в этот фикс не берём.

## 11. Консолидированный план реализации (по итогам решений)

**Этап 1 — P2 (критичное).**

1. `internal/servers/auth.go` — убрать `SanitizeInputString` из `parseBearer`, добавить `maxAuthorizationLength = 8 << 10`;
   `internal/servers/server.go` — `MaxHeaderBytes: 32 << 10`; новый `auth_test.go` с табличным `TestParseBearer` (включая регрессию
   «токен 600 символов возвращается без изменений» и лимит 9 КиБ).
2. `.github/workflows/go.yml` — `paths-ignore` вместо `paths`; `version: v2.14` у линтера; шаг `govulncheck` (`make go-check`);
   тестовые флаги через `GO_TEST_ARGS`; Go-версия из `.go-version`; (по возможности) `concurrency` в `trivy-scan.yml` и кэш `GOPATH/bin`.
3. Метрики в коде: `otelpgx.RecordStats(pool)` в `internal/database/postgres/postgres.go`; HTTP-middleware метрик и хелпер нормализации
   маршрута (переиспользовать в имени серверного спана, `internal/servers/tracing.go:35`); gRPC-интерцептор метрик в `internal/servers/grpc.go`.

**Этап 2 — логирование.** `internal/cache/cache.go:54` (key + value_len); комментарий о доверенных полях в `internal/servers/accesslog.go`;
`*Context`-варианты в 27 местах (`pkg/observability` 9, `internal/closer` 5, `internal/servers/server.go` 9, `internal/database/migrate.go` 3,
`internal/database/sqlite/sqlite.go` 1, `internal/servers/di.go` 1) + тест на `trace_id` в логах `closer`.

**Этап 3 — документация.** Наполнить `docs/specification/programming.md`; добавить в `README.md` раздел «Документация» и таблицу целей сборки;
сократить `.clinerules/{code,logs}.md` до выжимки со ссылкой на документ; перенести в документ правки по П.2 и П.5 A2.

**Этап 4 — мелочи.** Тест санитизации в `accesslog_test.go` (C2); типизированные атрибуты в `pkg/observability` (C3).

**Отдельные задачи (вне этого фикса):** Prometheus-контур (П.2.2); request ID (C1 — снят правкой правила); `make coverage` (C4);
ctx-правки группы 2 (смена сигнатур); `grpc.MaxHeaderListSize`; root cause out-of-scope пунктов отчёта, не затронутых решениями.
