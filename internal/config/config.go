package config

import (
	"log/slog"
	"strings"
	"time"

	"github.com/botbooker/bb-core/pkg/logger"
	"github.com/botbooker/bb-core/pkg/tools"
)

// Config хранит конфигурацию приложения.
type Config struct {
	// HTTP client identification header X-Forwarded-For or X-Real-IP
	ClientIP string `mapstructure:"ip"`
	// HTTPAddr — адрес HTTP-сервера (в том числе grpc-gateway),
	// собирается из SERVER_HOST и SERVER_PORT.
	HTTPAddr string
	// GRPCAddr — адрес gRPC-сервера,
	// собирается из SERVER_GRPC_HOST и SERVER_GRPC_PORT.
	GRPCAddr string
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
	// MigrateOnStart — применять миграции при старте приложения.
	// Выключается MIGRATE_ON_START=false, если миграции раскатывает отдельный шаг.
	MigrateOnStart bool `mapstructure:"migrate_on_start"`
}

// appConfig — глобальный экземпляр конфигурации.
var appConfig = &Config{
	ClientIP: tools.GetEnvOrDefault("HTTP_HEADER_CLIENT_IP", "X-Forwarded-For"),
	HTTPAddr: strings.Join([]string{
		tools.GetEnvOrDefault("SERVER_HOST", "localhost"),
		tools.GetEnvOrDefault("SERVER_PORT", "8080"),
	}, ":"),
	GRPCAddr: strings.Join([]string{
		tools.GetEnvOrDefault("SERVER_GRPC_HOST", "localhost"),
		tools.GetEnvOrDefault("SERVER_GRPC_PORT", "50051"),
	}, ":"),
	LogFormat: tools.GetEnvOrDefault("LOG_FORMAT", "json"),
	LogLevel:  logger.ParseLogLevel(tools.GetEnvOrDefault("LOG_LEVEL", "INFO"), slog.LevelInfo),
	RedisAddr: tools.GetEnvOrDefault("REDIS_HOST", "localhost:6379"),
}

// AuthConfig хранит настройки авторизации.
type AuthConfig struct {
	// JWTSecret — секрет подписи токенов (HS256). Из окружения: JWT_SECRET.
	//
	// Пустое значение не ошибка: приложение сгенерирует случайный секрет на
	// время работы (см. internal/jwt.NewSigner) и предупредит в логе. В
	// промышленной среде секрет задают явно, иначе токены не переживут
	// перезапуск и не будут приниматься другими экземплярами приложения.
	JWTSecret string
	// TokenTTL — срок жизни токена доступа. Из окружения: AUTH_TOKEN_TTL (например, "720h").
	TokenTTL time.Duration
	// Issuer — кто выдал токен (claim iss). Из окружения: AUTH_JWT_ISSUER.
	Issuer string
}

// Auth возвращает настройки авторизации.
func Auth() *AuthConfig {
	return authConfig
}

// DefaultTokenTTL — срок жизни токена доступа по умолчанию: 30 дней.
const DefaultTokenTTL = 720 * time.Hour

var authConfig = &AuthConfig{
	JWTSecret: tools.GetEnvOrDefault("JWT_SECRET", ""),
	TokenTTL:  tools.GetEnvOrDefaultToDuration("AUTH_TOKEN_TTL", DefaultTokenTTL),
	Issuer:    tools.GetEnvOrDefault("AUTH_JWT_ISSUER", "bb-core"),
}

var databaseConfig = &DatabaseConfig{
	DSN:            tools.GetEnvOrDefault("DSN", "postgres://localhost:5432/demo"),
	MaxDBConnect:   tools.GetEnvOrDefaultToInt32("MAX_DB_CONNECT", 10),
	MigrateOnStart: tools.GetEnvOrDefaultToBool("MIGRATE_ON_START", true),
}

// AppConfig возвращает конфигурацию приложения.
func AppConfig() *Config {
	return appConfig
}

// DBConfig возвращает конфигурацию приложения.
func DBConfig() *DatabaseConfig {
	return databaseConfig
}
