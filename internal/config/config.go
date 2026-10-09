package config

import (
	"log/slog"
	"strings"

	"github.com/botbooker/bb-core/pkg/logger"
	"github.com/botbooker/bb-core/pkg/tools"
)

// Config хранит конфигурацию приложения.
type Config struct {
	DSN       string
	RedisAddr string
	HTTPAddr  string
	ClientIP  string
	LogLevel  slog.Level
	LogFormat string
}

// appConfig — глобальный экземпляр конфигурации.
var appConfig = &Config{
	DSN:       tools.GetEnvOrDefault("DSN", "postgres://localhost:5432/demo"),
	RedisAddr: tools.GetEnvOrDefault("REDIS_HOST", "localhost:6379"),
	HTTPAddr: strings.Join([]string{
		tools.GetEnvOrDefault("SERVER_HOST", "localhost"),
		tools.GetEnvOrDefault("SERVER_PORT", "8080"),
	}, ":"),
	ClientIP:  tools.GetEnvOrDefault("HTTP_HEADER_CLIENT_IP", "X-Forwarded-For"),
	LogLevel:  logger.ParseLogLevel(tools.GetEnvOrDefault("LOG_LEVEL", "INFO"), slog.LevelInfo),
	LogFormat: tools.GetEnvOrDefault("LOG_FORMAT", "json"),
}

// AppConfig возвращает конфигурацию приложения.
func AppConfig() *Config {
	return appConfig
}
