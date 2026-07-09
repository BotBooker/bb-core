// cmd/api/main.go
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"bookerbotapi/internal/availability"
	"bookerbotapi/internal/config"
	"bookerbotapi/internal/database"
	"bookerbotapi/internal/repository"
	"bookerbotapi/internal/server"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Initialize PostgreSQL database
	dbConfig := cfg.GetDatabaseConfig()
	postgresDB, err := database.NewPostgresDB(database.Config{
		Driver:          dbConfig.Driver,
		DSN:             dbConfig.DSN,
		MaxOpenConns:    dbConfig.MaxOpenConns,
		MaxIdleConns:    dbConfig.MaxIdleConns,
		ConnMaxLifetime: dbConfig.ConnMaxLifetime,
		ConnMaxIdleTime: dbConfig.ConnMaxIdleTime,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer func() {
		if err := postgresDB.Close(); err != nil {
			log.Warn().Err(err).Msg("Error closing database")
		}
	}()

	// Initialize Redis
	redisClient, err := database.NewRedisClient(database.RedisConfig{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize Redis")
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Warn().Err(err).Msg("Error closing Redis")
		}
	}()

	// Initialize admin repository
	adminRepo := repository.NewPostgresAdminRepository(postgresDB.DB)

	// Initialize availability manager
	availabilityManager := availability.NewAvailabilityManager(redisClient.Client, adminRepo)

	// Create server with dependencies
	srv := server.New(cfg, adminRepo, availabilityManager)

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on port %s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed to start")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server..")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited gracefully")
}
