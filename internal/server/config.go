package server

import (
	"log/slog"

	"github.com/botbooker/bb-core/internal/tools"
)

// ServerConfig содержит конфигурацию HTTP-сервера.
type ServerConfig struct {
	Host            string
	Port            string
	TrustedProxies  []string
	TrustedPlatform string
	LogLevel        slog.Level
}

// parseLogLevel parses a slog.Level from a string.
// Returns the default level if the input is invalid.
func parseLogLevel(level string, defaultLevel slog.Level) slog.Level {
	switch level {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return defaultLevel
	}
}

// GetServerConfig создаёт конфигурацию сервера из переменных окружения.
func GetServerConfig() ServerConfig {
	return ServerConfig{
		Host:            tools.GetEnvOrDefault("SERVER_HOST", "localhost"),
		Port:            tools.GetEnvOrDefault("SERVER_PORT", "8080"),
		TrustedProxies:  tools.GetEnvList("SERVER_TRUSTED_PROXIES", nil),
		TrustedPlatform: tools.GetEnvOrDefault("SERVER_TRUSTED_PLATFORM", "X-Forwarded-For"),
		LogLevel:        parseLogLevel(tools.GetEnvOrDefault("LOG_LEVEL", "INFO"), slog.LevelInfo),
	}
}
