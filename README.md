This is backend API for booking service.

It is written in Go.
It uses go-chi for routing.
It uses zerolog for logging.
It uses go-chi prometheus integration for observability.
It uses pressly/goose for managing database migrations, which are stored in db/migrations folder. No migration should be edited, a new migration should be created instead.
It uses Postgresql as database and sqlx as postgres connector. Zerolog integration is used to log sql queries.
OpenAPI specification is stored in spec/ directory and sould be not changed by any means.
It has graceful shutdown.
It uses mockgen for mock creation.
It uses stretchr/testify to write tests. Table-style tests are preferred.

