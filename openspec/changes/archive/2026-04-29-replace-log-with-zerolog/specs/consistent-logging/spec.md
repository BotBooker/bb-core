## ADDED Requirements

### Requirement: consistent-zerolog

All logging in the application SHALL use github.com/rs/zerolog instead of the standard log package to provide structured, consistent logging across the entire codebase.

#### Scenario: Replace Log Imports
- **WHEN** a Go file imports `"log"`
- **THEN** replace with `"github.com/rs/zerolog/log"`

#### Scenario: Standardize Log Calls
- **WHEN** a file uses `log.Println()` or `log.Print()`
- **THEN** replace with appropriate `log.Info().Msg()`, `log.Error().Msg()`, etc.
