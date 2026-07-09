// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/rs/zerolog/log"
)

// Config holds all configuration for the application.
type Config struct {
	Server   ServerConfig   `koanf:"server"`
	Auth     AuthConfig     `koanf:"auth"`
	Database DatabaseConfig `koanf:"database"`
	Redis    RedisConfig    `koanf:"redis"`
	Logging  LoggingConfig  `koanf:"logging"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port         string        `koanf:"port"`
	ReadTimeout  time.Duration `koanf:"read_timeout"`
	WriteTimeout time.Duration `koanf:"write_timeout"`
	IdleTimeout  time.Duration `koanf:"idle_timeout"`
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	APIKeys []string `koanf:"api_keys"`
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Driver          string `koanf:"driver"`
	DSN             string `koanf:"dsn"`
	MaxOpenConns    int    `koanf:"max_open_conns"`
	MaxIdleConns    int    `koanf:"max_idle_conns"`
	ConnMaxLifetime int    `koanf:"conn_max_lifetime_minutes"`
	ConnMaxIdleTime int    `koanf:"conn_max_idle_time_minutes"`
}

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Host     string `koanf:"host"`
	Port     string `koanf:"port"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level string `koanf:"level"`
	JSON  bool   `koanf:"json"`
}

// Load loads configuration from YAML files and environment variables.
func Load() (*Config, error) {
	k := koanf.New(".")

	instance := os.Getenv("INSTANCE")
	if instance == "" {
		instance = "local"
	}

	path := "config/" + instance + "/config.yaml"
	log.Info().Msgf("using instance: %s", instance)
	log.Info().Msgf("loading configuration from: %s", path)

	// Load YAML config file (non-fatal if missing)
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		log.Warn().Err(err).Msg("failed to load config file, using defaults")
	}

	// Load environment variables (without prefix, underscore as separator)
	if err := k.Load(env.Provider("", "_", nil), nil); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	// Unmarshal into config struct
	var config Config
	if err := k.Unmarshal("", &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Apply defaults for zero-value fields
	config.applyDefaults()

	return &config, nil
}

// applyDefaults sets sensible defaults for any unconfigured fields.
func (c *Config) applyDefaults() {
	if c.Server.Port == "" {
		c.Server.Port = "8080"
	}
	if c.Server.ReadTimeout <= 0 {
		c.Server.ReadTimeout = 30 * time.Second
	}
	if c.Server.WriteTimeout <= 0 {
		c.Server.WriteTimeout = 30 * time.Second
	}
	if c.Server.IdleTimeout <= 0 {
		c.Server.IdleTimeout = 60 * time.Second
	}
	if c.Database.Driver == "" {
		c.Database.Driver = "postgres"
	}
	if c.Database.MaxOpenConns <= 0 {
		c.Database.MaxOpenConns = 25
	}
	if c.Database.MaxIdleConns <= 0 {
		c.Database.MaxIdleConns = 10
	}
	if c.Database.ConnMaxLifetime <= 0 {
		c.Database.ConnMaxLifetime = 5
	}
	if c.Database.ConnMaxIdleTime <= 0 {
		c.Database.ConnMaxIdleTime = 10
	}
	if c.Redis.Host == "" {
		c.Redis.Host = "localhost"
	}
	if c.Redis.Port == "" {
		c.Redis.Port = "6379"
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
}

// GetDatabaseConfig returns database configuration with parsed durations.
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

// DatabaseConfigWithDuration holds database config with parsed time.Duration fields.
type DatabaseConfigWithDuration struct {
	Driver          string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}
