package server

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServerConfig_Defaults(t *testing.T) {
	// Убеждаемся, что переменные окружения не установлены
	os.Unsetenv("SERVER_HOST")
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("SERVER_TRUSTED_PROXIES")
	os.Unsetenv("SERVER_TRUSTED_PLATFORM")

	cfg := GetServerConfig()

	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, "8080", cfg.Port)
	assert.Nil(t, cfg.TrustedProxies)
	assert.Equal(t, "X-Forwarded-For", cfg.TrustedPlatform)
}

func TestServerConfig_CustomHost(t *testing.T) {
	// Устанавливаем кастомный хост
	os.Setenv("SERVER_HOST", "127.0.0.1")
	os.Unsetenv("SERVER_PORT")
	defer os.Unsetenv("SERVER_HOST")

	cfg := GetServerConfig()

	assert.Equal(t, "127.0.0.1", cfg.Host)
	assert.Equal(t, "8080", cfg.Port)
}

func TestServerConfig_CustomPort(t *testing.T) {
	// Устанавливаем кастомный порт
	os.Unsetenv("SERVER_HOST")
	os.Setenv("SERVER_PORT", "3000")
	defer os.Unsetenv("SERVER_PORT")

	cfg := GetServerConfig()

	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, "3000", cfg.Port)
}

func TestServerConfig_CustomValues(t *testing.T) {
	// Устанавливаем оба значения
	os.Setenv("SERVER_HOST", "localhost")
	os.Setenv("SERVER_PORT", "9090")
	os.Setenv("SERVER_TRUSTED_PROXIES", "127.0.0.1,192.168.1.0/24")
	os.Setenv("SERVER_TRUSTED_PLATFORM", "X-Forwarded-For")
	defer func() {
		os.Unsetenv("SERVER_HOST")
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("SERVER_TRUSTED_PROXIES")
		os.Unsetenv("SERVER_TRUSTED_PLATFORM")
	}()

	cfg := GetServerConfig()

	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, []string{"127.0.0.1", "192.168.1.0/24"}, cfg.TrustedProxies)
	assert.Equal(t, "X-Forwarded-For", cfg.TrustedPlatform)
}

func TestServerConfig_StructFields(t *testing.T) {
	cfg := ServerConfig{
		Host:            "test-host",
		Port:            "test-port",
		TrustedProxies:  []string{"10.0.0.1"},
		TrustedPlatform: "X-Real-IP",
	}

	assert.Equal(t, "test-host", cfg.Host)
	assert.Equal(t, "test-port", cfg.Port)
	assert.Equal(t, []string{"10.0.0.1"}, cfg.TrustedProxies)
	assert.Equal(t, "X-Real-IP", cfg.TrustedPlatform)
}

func TestServerConfig_TrustedProxies(t *testing.T) {
	os.Unsetenv("SERVER_HOST")
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("SERVER_TRUSTED_PLATFORM")
	os.Setenv("SERVER_TRUSTED_PROXIES", "10.0.0.0/8,172.16.0.0/12")
	defer os.Unsetenv("SERVER_TRUSTED_PROXIES")

	cfg := GetServerConfig()

	assert.Equal(t, []string{"10.0.0.0/8", "172.16.0.0/12"}, cfg.TrustedProxies)
}

func TestServerConfig_TrustedPlatform(t *testing.T) {
	os.Unsetenv("SERVER_HOST")
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("SERVER_TRUSTED_PROXIES")
	os.Setenv("SERVER_TRUSTED_PLATFORM", "X-Forwarded-For")
	defer os.Unsetenv("SERVER_TRUSTED_PLATFORM")

	cfg := GetServerConfig()

	assert.Equal(t, "X-Forwarded-For", cfg.TrustedPlatform)
}

func TestParseLogLevel_ValidLevels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected slog.Level
	}{
		{"DEBUG", "DEBUG", slog.LevelDebug},
		{"INFO", "INFO", slog.LevelInfo},
		{"WARN", "WARN", slog.LevelWarn},
		{"ERROR", "ERROR", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLogLevel(tt.input, slog.LevelInfo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseLogLevel_InvalidLevel(t *testing.T) {
	result := parseLogLevel("INVALID", slog.LevelInfo)
	assert.Equal(t, slog.LevelInfo, result)
}

func TestParseLogLevel_EmptyLevel(t *testing.T) {
	result := parseLogLevel("", slog.LevelWarn)
	assert.Equal(t, slog.LevelWarn, result)
}

func TestParseLogLevel_CaseSensitive(t *testing.T) {
	// Lowercase should return default
	result := parseLogLevel("debug", slog.LevelInfo)
	assert.Equal(t, slog.LevelInfo, result)
}

func TestServerConfig_DefaultLogLevel(t *testing.T) {
	os.Unsetenv("SERVER_HOST")
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("SERVER_TRUSTED_PROXIES")
	os.Unsetenv("SERVER_TRUSTED_PLATFORM")
	os.Unsetenv("LOG_LEVEL")

	cfg := GetServerConfig()

	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
}

func TestServerConfig_CustomLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected slog.Level
	}{
		{"DEBUG", "DEBUG", slog.LevelDebug},
		{"INFO", "INFO", slog.LevelInfo},
		{"WARN", "WARN", slog.LevelWarn},
		{"ERROR", "ERROR", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("SERVER_HOST")
			os.Unsetenv("SERVER_PORT")
			os.Unsetenv("SERVER_TRUSTED_PROXIES")
			os.Unsetenv("SERVER_TRUSTED_PLATFORM")
			os.Setenv("LOG_LEVEL", tt.envValue)
			defer os.Unsetenv("LOG_LEVEL")

			cfg := GetServerConfig()

			assert.Equal(t, tt.expected, cfg.LogLevel)
		})
	}
}

func TestServerConfig_InvalidLogLevel(t *testing.T) {
	os.Unsetenv("SERVER_HOST")
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("SERVER_TRUSTED_PROXIES")
	os.Unsetenv("SERVER_TRUSTED_PLATFORM")
	os.Setenv("LOG_LEVEL", "TRACE")
	defer os.Unsetenv("LOG_LEVEL")

	cfg := GetServerConfig()

	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
}

func TestServerConfig_EmptyLogLevel(t *testing.T) {
	os.Unsetenv("SERVER_HOST")
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("SERVER_TRUSTED_PROXIES")
	os.Unsetenv("SERVER_TRUSTED_PLATFORM")
	os.Setenv("LOG_LEVEL", "")
	defer os.Unsetenv("LOG_LEVEL")

	cfg := GetServerConfig()

	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
}
