// internal/config/config.go
package config

import (
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/nil-go/konf"
	"github.com/nil-go/konf/provider/env"
	"github.com/nil-go/konf/provider/file"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

var configFiles embed.FS

type Config struct {
	Server   ServerConfig   `konf:"server"`
	Auth     AuthConfig     `konf:"auth"`
	Database DatabaseConfig `konf:"database"`
	Redis    RedisConfig    `konf:"redis"`
	Logging  LoggingConfig  `konf:"logging"`
}

type ServerConfig struct {
	Port         string        `konf:"port,default=8080"`
	ReadTimeout  time.Duration `konf:"read_timeout,default=30s"`
	WriteTimeout time.Duration `konf:"write_timeout,default=30s"`
	IdleTimeout  time.Duration `konf:"idle_timeout,default=60s"`
}

type AuthConfig struct {
	APIKeys []string `konf:"api_keys"`
}

type DatabaseConfig struct {
	Driver          string `konf:"driver,default=postgres"`
	DSN             string `konf:"dsn"`
	MaxOpenConns    int    `konf:"max_open_conns,default=25"`
	MaxIdleConns    int    `konf:"max_idle_conns,default=10"`
	ConnMaxLifetime int    `konf:"conn_max_lifetime_minutes,default=5"`
	ConnMaxIdleTime int    `konf:"conn_max_idle_time_minutes,default=10"`
}

type RedisConfig struct {
	Host     string `konf:"host,default=localhost"`
	Port     string `konf:"port,default=6379"`
	Password string `konf:"password,default="`
	DB       int    `konf:"db,default=0"`
}

type LoggingConfig struct {
	Level string `konf:"level,default=info"`
	JSON  bool   `konf:"json,default=false"`
}

func Load() (*Config, error) {
	// Create a new konf configuration instance
	cfg := konf.New()

	instance := os.Getenv("INSTANCE")
	if instance == "" {
		instance = "local"
	}

	path := "config/" + instance + "/config.yaml"
	log.Info().Msgf("using %s instance", instance)
	log.Info().Msgf("loading configuration at %s", path)
	// Load configuration
	if err := cfg.Load(file.New(path, file.WithUnmarshal(yaml.Unmarshal))); err != nil {
		// Handle error here.
		log.Fatal().Err(err)
	}

	// Load from environment variables
	// Using WithPrefix("") to load all environment variables without a prefix
	if err := cfg.Load(env.New(env.WithPrefix(""))); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	// FIXME update from mkk file loading
	// Optionally load from embedded config file as fallback defaults
	// Uncomment if you want to use embedded JSON config files
	/*
		if err := cfg.Load(fs.New(configFiles, "configs/config.json")); err != nil {
			// Silently continue - embedded config might not exist
		}
	*/

	// Set as default global configuration
	konf.SetDefault(cfg)

	// Unmarshal the configuration into our struct
	var config Config
	if err := cfg.Unmarshal("", &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	return &config, nil
}

// GetDatabaseConfig returns database configuration with parsed durations
func (c *Config) GetDatabaseConfig() DatabaseConfigWithDuration {
	return DatabaseConfigWithDuration{
		Driver:          c.Database.Driver,
		DSN:             c.Database.DSN,
		MaxOpenConns:    c.Database.MaxOpenConns,
		MaxIdleConns:    c.Database.MaxIdleConns,
		ConnMaxLifetime: time.Duration(c.Database.ConnMaxLifetime) * time.Minute,
		ConnMaxIdleTime: time.Duration(c.Database.ConnMaxIdleTime) * time.Minute,
	}
}

type DatabaseConfigWithDuration struct {
	Driver          string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}
