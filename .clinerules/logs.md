# Process Logging Specification

Полная спецификация — в
[docs/specification/programming.md](../../docs/specification/programming.md) (§2); здесь краткая выжимка. При
расхождении источник истины — документ.

## 1. Security & Sanitization

- **Sanitize user input.** Значения, пришедшие от пользователя и передаваемые
  наружу (log sinks, коллекторы, сторонние API), прогоняй через
  `tools.SanitizeInputString` (замена переводов строк, ограничение длины).
- **Trusted values.** Значения, которые формирует транспорт или фреймворк
  (HTTP-метод, адрес клиента, RPC-метод из реестра сервисов, шаблон маршрута),
  санитизировать не нужно — но помечай их в коде комментарием с объяснением,
  почему значение доверенное.
- **Strip credentials.** Никогда не логируй и не передавай наружу токены,
  пароли и ключи API. Маскируй значение ключом
  (`SanitizeInputString(value, "authorization")` вернёт `***`); пароль в адресе
  подключения скрывает `redactAddr`.
- Не логируй содержимое кэша, тела запросов и ответов с персональными данными:
  пиши только ключ/длину/факт операции.

## 2. Telemetry Logging

- **Enabled state — `Info`.** Ровно одна запись на сигнал при старте, с адресом
  коллектора (например, «otlp traces export enabled»).
- **Остальное — `Debug`.** Состояние *выключения*, детали ошибок экспортёра,
  диагностика по батчам/экспортам и дампы атрибутов — только `Debug`; в потоки
  `Info`, `Warn`, `Error` они не направляются.

## 3. Usage & Implementation (Go `slog`)

- **Structured logging.** Только стандартный `log/slog`.
- **Context-aware.** Логируй через `slog.InfoContext`, `slog.DebugContext`,
  `slog.WarnContext`, `slog.ErrorContext`: `trace_id`/`span_id` активного спана
  добавляет `pkg/logger.TraceHandler` из контекста записи.
- **Sanitization helper.** Перед записью пользовательской строки — санитизация
  через `tools.SanitizeInputString` или `slog.LogValuer`.

**Пример:**

```go
// Одна запись «export enabled» на сигнал может быть на Info (§2).
logger.InfoContext(ctx, "otlp traces export enabled",
 slog.String("endpoint", endpoint), slog.String("protocol", "gRPC"))

// Любая другая диагностика состояния экспортёра — Debug.
logger.DebugContext(ctx, "OpenTelemetry exporter state", slog.Bool("enabled", isOtelEnabled))

// Пользовательское значение — через санитайзер.
logger.InfoContext(ctx, "Processing user request",
 slog.String("user_id", tools.SanitizeInputString(req.UserID)))
```
