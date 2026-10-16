# Process Logging Specification

## 1. Security & Sanitization

- **Sanitize User Input:** You must sanitize all variable values that come from the user and are passed to external services (log sinks, collectors,
  third-party APIs). Stripping out special characters that could lead to injection or formatting errors is mandatory. Trusted values that the
  transport or framework forms itself (HTTP method, remote address, matched route or RPC method taken from the served registry) do not require
  sanitization — mark them as trusted in code with a comment explaining why.
- **Strip Credentials:** Never log, expose, or pass raw authentication data (tokens, passwords, API keys). Redact or remove them before any transmission.

## 2. Telemetry Logging

- **OpenTelemetry Logs:** The *enabled* state of a signal is allowed at `Info` — exactly one record per signal at startup, carrying the
  collector endpoint (e.g. "otlp traces export enabled"). Every other diagnostic about OTel exports — the *disabled* state, exporter
  errors details, per-batch or per-export diagnostics, attribute or payload dumps — must be logged at `Debug` and must not be routed
  to Info, Warn, or Error streams.

## 3. Usage & Implementation (Go `slog`)

- **Use Structured Logging:** Always use the standard `log/slog` package for process logging.
- **Debug Level for Telemetry:** Implement OpenTelemetry state *diagnostics* using `slog.Debug()` or `slog.DebugContext(ctx, ...)`.
  The single "export enabled" record per signal is the documented exception of section 2 and may use `Info`.
- **Context-Aware Logging:** Pass `context.Context` to loggers where possible to maintain trace IDs.
- **Sanitization Helper:** Before logging any user-provided string, pass it through a sanitization function (e.g., stripping special characters or using a custom `slog.LogValuer` to mask sensitive data).

**Example Pattern:**

```go
// Correct: the one "export enabled" record per signal may use Info (section 2).
logger.Info("otlp traces export enabled", slog.String("endpoint", endpoint), "protocol", "gRPC")

// Correct: every other exporter state diagnostic is Debug.
logger.Debug("OpenTelemetry exporter state", slog.Bool("enabled", isOtelEnabled))

// Correct Sanitized User Input Log
logger.Info("Processing user request", slog.Int("user_id", SanitizeInputString(req.UserID)))
```
