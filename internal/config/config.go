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
	// PublicBaseURL — внешний адрес приложения для ссылок вида /t/{tenant_slug}.
	// Из окружения: PUBLIC_BASE_URL. Используется bootstrap и документацией.
	PublicBaseURL string
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
	// MigrationsDir — каталог с goose-миграциями: внутри него подкаталоги
	// postgres/ и sqlite/. Из окружения: MIGRATIONS_DIR. По умолчанию
	// ./migrations — рядом с рабочим каталогом процесса. В Docker-образе
	// миграции лежат в /app/migrations, а рабочий каталог — /app, поэтому
	// значение по умолчанию подходит и там.
	MigrationsDir string `mapstructure:"migrations_dir"`
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
	PublicBaseURL: tools.GetEnvOrDefault("PUBLIC_BASE_URL", "http://localhost:8080"),
	LogFormat:     tools.GetEnvOrDefault("LOG_FORMAT", "json"),
	LogLevel:      logger.ParseLogLevel(tools.GetEnvOrDefault("LOG_LEVEL", "INFO"), slog.LevelInfo),
	RedisAddr:     tools.GetEnvOrDefault("REDIS_HOST", "localhost:6379"),
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
	// TokenTTL — срок жизни токена доступа. Из окружения: AUTH_TOKEN_TTL (например, "720h" или "30d").
	TokenTTL time.Duration
	// Issuer — кто выдал токен (claim iss). Из окружения: AUTH_JWT_ISSUER.
	Issuer string
	// PasswordTTL — срок жизни пароля: сколько времени пароль действует с
	// момента последнего изменения. Из окружения: PASSWORD_TTL Ноль или
	// отрицательное значение отключает проверку —пароль не истекает.
	PasswordTTL time.Duration
}

// Auth возвращает настройки авторизации.
func Auth() *AuthConfig {
	return authConfig
}

// DefaultTokenTTL — срок жизни токена доступа по умолчанию: 30 дней.
const DefaultTokenTTL = 720 * time.Hour

// defaultPasswordTTLDays — срок жизни пароля по умолчанию.
const defaultPasswordTTLDays = 90 * 24 * time.Hour

var authConfig = &AuthConfig{
	JWTSecret:   tools.GetEnvOrDefault("JWT_SECRET", ""),
	TokenTTL:    tools.GetEnvOrDefaultToDuration("AUTH_TOKEN_TTL", DefaultTokenTTL),
	Issuer:      tools.GetEnvOrDefault("AUTH_JWT_ISSUER", "bb-core"),
	PasswordTTL: passwordTTLFromEnv(tools.GetEnvOrDefaultToDuration("PASSWORD_TTL", defaultPasswordTTLDays)),
}

// passwordTTLFromEnv переводит срок жизни пароля из дней в time.Duration.
// Ноль и отрицательные значения означают «пароль не истекает».
func passwordTTLFromEnv(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return 0
	}
	return ttl
}

// BootstrapConfig хранит данные первого суперпользователя, которого создаёт
// команда bootstrap (cmd/bootstrap).
type BootstrapConfig struct {
	// Email — адрес суперпользователя. Из окружения: SUPERUSER_EMAIL.
	Email string
	// Password — открытый пароль суперпользователя. Из окружения:
	// SUPERUSER_PASSWORD. В базе хранится только его хеш.
	Password string
	// FirstName, LastName — имя и фамилия. Из окружения: SUPERUSER_FIRST_NAME,
	// SUPERUSER_LAST_NAME.
	FirstName string
	LastName  string
}

// Bootstrap возвращает настройки первого суперпользователя.
func Bootstrap() *BootstrapConfig {
	return bootstrapConfig
}

var bootstrapConfig = &BootstrapConfig{
	Email:     tools.GetEnvOrDefault("SUPERUSER_EMAIL", ""),
	Password:  tools.GetEnvOrDefault("SUPERUSER_PASSWORD", ""),
	FirstName: tools.GetEnvOrDefault("SUPERUSER_FIRST_NAME", "Super"),
	LastName:  tools.GetEnvOrDefault("SUPERUSER_LAST_NAME", "Admin"),
}

var databaseConfig = &DatabaseConfig{
	DSN:            tools.GetEnvOrDefault("DSN", "postgres://localhost:5432/demo"),
	MaxDBConnect:   tools.GetEnvOrDefaultToInt32("MAX_DB_CONNECT", 10),
	MigrateOnStart: tools.GetEnvOrDefaultToBool("MIGRATE_ON_START", true),
	MigrationsDir:  tools.GetEnvOrDefault("MIGRATIONS_DIR", "./migrations"),
}

// AppConfig возвращает конфигурацию приложения.
func AppConfig() *Config {
	return appConfig
}

// DBConfig возвращает конфигурацию приложения.
func DBConfig() *DatabaseConfig {
	return databaseConfig
}
