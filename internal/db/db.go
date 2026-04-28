package db

import (
	"database/sql"
	"fmt"

	"bookerbotapi/internal/config"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

func Init(cfg *config.Config) (*sql.DB, error) {
	// Create connection string
	connStr := fmt.Sprintf(cfg.Database.DSN)

	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info().Msg("Database connected successfully")

	return db, nil
}
