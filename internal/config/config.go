package config

import (
	"log/slog"
	"strings"

	"github.com/botbooker/bb-core/pkg/logger"
	"github.com/botbooker/bb-core/pkg/tools"
)

// Config хранит конфигурацию приложения.
type Config struct {
	// HTTP client identification header X-Forwarded-For or X-Real-IP
	ClientIP string `mapstructure:"ip"`
	// Listen ip/host:port
	HTTPAddr string
	// format: plain or json
	LogFormat string `mapstructure:"logformat"`
	// LogLevel: DEBUG, INFO, WARN, ERROR
	LogLevel slog.Level `mapstructure:"loglevel"`
	// Connection Strings to cache
	RedisAddr string
}

type DatabaseConfig struct {
	// DSN — строка подключения. Диалект не задаётся отдельно:
	// он определяется по DSN (database.DialectFromDSN).
	DSN string `mapstructure:"dsn"`
	// MaxDBConnect
	MaxDBConnect int32 `mapstructure:"max_connections"`
}

// appConfig — глобальный экземпляр конфигурации.
var appConfig = &Config{
	ClientIP: tools.GetEnvOrDefault("HTTP_HEADER_CLIENT_IP", "X-Forwarded-For"),
	HTTPAddr: strings.Join([]string{
		tools.GetEnvOrDefault("SERVER_HOST", "localhost"),
		tools.GetEnvOrDefault("SERVER_PORT", "8080"),
	}, ":"),
	LogFormat: tools.GetEnvOrDefault("LOG_FORMAT", "json"),
	LogLevel:  logger.ParseLogLevel(tools.GetEnvOrDefault("LOG_LEVEL", "INFO"), slog.LevelInfo),
	RedisAddr: tools.GetEnvOrDefault("REDIS_HOST", "localhost:6379"),
}

var databaseConfig = &DatabaseConfig{
	DSN:          tools.GetEnvOrDefault("DSN", "postgres://localhost:5432/demo"),
	MaxDBConnect: tools.GetEnvOrDefaultToInt32("MAX_DB_CONNECT", 10),
}

// AppConfig возвращает конфигурацию приложения.
func AppConfig() *Config {
	return appConfig
}

// DBConfig возвращает конфигурацию приложения.
func DBConfig() *DatabaseConfig {
	return databaseConfig
}
