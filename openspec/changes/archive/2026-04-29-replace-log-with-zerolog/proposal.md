## Why

Migrate remaining standard library `log` usage to `github.com/rs/zerolog` for consistent structured logging across the entire project. Currently, `main.go` and `config.go` already use zerolog, while `redis.go` and `postgres.go` still use `log.Println()`, creating inconsistency in logging patterns.

## What Changes

- **Replace standard log import**: Change `"log"` to `"github.com/rs/zerolog/log"` in `internal/database/redis.go` and `internal/database/postgres.go`
- **Replace log.Println() calls**: Convert simple `log.Println()` statements to zerolog `log.Info().Msg()` calls

## Capabilities

### New Capabilities

- **consistent-logging**: Migrate std log usage to zerolog in all project files for uniform structured logging

### Modified Capabilities

