// cmd/api/main.go (updated)
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		log.Fatalf("Failed to load configuration: %v", err)
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
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() {
		if err := postgresDB.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
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
		log.Fatalf("Failed to initialize Redis: %v", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("Error closing Redis: %v", err)
		}
	}()

	// Initialize repositories
	bookingRepo := repository.NewPostgresBookingRepository(postgresDB.DB)

	// Initialize availability manager
	availabilityManager := availability.NewAvailabilityManager(redisClient.Client, bookingRepo)

	// Create server with dependencies
	srv := server.New(cfg, bookingRepo, availabilityManager)

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on port %s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
}
