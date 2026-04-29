## Context

The `bookerbotapi` project is migrating from the standard Go `log` package to `github.com/rs/zerolog` for consistent structured logging. Currently, the project already uses zerolog in `main.go` and `config.go`, but two files in the `internal/database/` package (`redis.go` and `postgres.go`) still use the standard `log` package. This change will unify logging across the entire codebase.

## Goals / Non-Goals

**Goals:**
- Replace all standard library `log` usage with zerolog in the project
- Maintain consistent logging patterns with existing zerolog usage
- Preserve existing log messages while upgrading to structured logging

**Non-Goals:**
- Adding contextual fields to logging (this is future work)
- Changing log message text (unless specifically improving clarity)
- Modifying third-party dependencies (vendor files)

## Decisions

### Use zerolog/log.Logger directly

We will use the existing `zerolog/log` logger from the package (same as used in main.go and config.go) rather than creating new logger instances. This ensures consistent logging levels and configuration.

**Rationale:** The global `zerolog/log` logger is already configured by the application. Re-using the same logger maintains consistency with existing zerolog usage in the codebase.

### Migrate log.Println to log.Info().Msg()

Standard `log.Println()` statements will be converted to `log.Info().Msg()` calls with preserved message text.

**Rationale:** This maintains the informational level and message format while using the new logging framework. The `Info()` level matches the informational nature of connection success and shutdown messages.

### No Breaking Changes

This is a non-breaking change as it only affects internal logging output and does not change any public APIs or data contracts.

## Risks / Trade-offs

[Risk: Log output format may change] → The zerolog `Info().Msg()` output format differs slightly from `log.Println()`. This is acceptable as it provides structured logging benefits.

[Risk: Missing dependencies] → If zerolog is removed, logging would break. We must keep `github.com/rs/zerolog` as a dependency, which it already is.

