## 1. Update redis.go

- [x] 1.1 Replace `import "log"` with `import "github.com/rs/zerolog/log"` in `internal/database/redis.go`
- [x] 1.2 Replace `log.Println("Redis connected successfully")` with `log.Info().Msg("Redis connected successfully")`
- [x] 1.3 Replace `log.Println("Closing Redis connection")` with `log.Info().Msg("Closing Redis connection")`

## 2. Update postgres.go

- [x] 2.1 Replace `import "log"` with `import "github.com/rs/zerolog/log"` in `internal/database/postgres.go`
- [x] 2.2 Replace `log.Println("PostgreSQL database connected successfully")` with `log.Info().Msg("PostgreSQL database connected successfully")`
- [x] 2.3 Replace `log.Println("Closing PostgreSQL database connection")` with `log.Info().Msg("Closing PostgreSQL database connection")`

## 3. Verify Changes

- [x] 3.1 Build the project to ensure no compilation errors: `make build` (or `go build`)
- [x] 3.2 Run tests if applicable to ensure functional correctness: No tests present in project
- [x] 3.3 Review log output to verify structured logging is working

