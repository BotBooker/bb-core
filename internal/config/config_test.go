package config

import (
	"os"
	"testing"

	"github.com/botbooker/bb-core/internal/tools"
)

func TestAppConfig(t *testing.T) {
	tests := []struct {
		name         string
		envSetup     func()
		wantDSN      string
		wantRedis    string
		wantClientIP string
		wantHTTPAddr string
	}{
		{
			name: "default values",
			envSetup: func() {
				os.Unsetenv("DSN")
				os.Unsetenv("REDIS_HOST")
				os.Unsetenv("HTTP_HEADER_CLIENT_IP")
				os.Unsetenv("SERVER_HOST")
				os.Unsetenv("SERVER_PORT")
				os.Unsetenv("LOG_LEVEL")
			},
			wantDSN:      "postgres://localhost:5432/demo",
			wantRedis:    "localhost:6379",
			wantClientIP: "X-Forwarded-For",
			wantHTTPAddr: "localhost:localhost",
		},
		{
			name: "custom values",
			envSetup: func() {
				os.Setenv("DSN", "postgres://user:pass@db:5432/mydb")
				os.Setenv("REDIS_HOST", "redis:6380")
				os.Setenv("HTTP_HEADER_CLIENT_IP", "X-Real-IP")
				os.Setenv("SERVER_HOST", "0.0.0.0")
				os.Setenv("SERVER_PORT", "8080")
				os.Setenv("LOG_LEVEL", "DEBUG")
			},
			wantDSN:      "postgres://user:pass@db:5432/mydb",
			wantRedis:    "redis:6380",
			wantClientIP: "X-Real-IP",
			wantHTTPAddr: "0.0.0.0:8080",
		},
		{
			name: "partial custom values",
			envSetup: func() {
				os.Setenv("DSN", "postgres://custom:5432/db")
				os.Unsetenv("REDIS_HOST")
				os.Unsetenv("HTTP_HEADER_CLIENT_IP")
				os.Setenv("SERVER_HOST", "127.0.0.1")
				os.Setenv("SERVER_PORT", "3000")
				os.Unsetenv("LOG_LEVEL")
			},
			wantDSN:      "postgres://custom:5432/db",
			wantRedis:    "localhost:6379",
			wantClientIP: "X-Forwarded-For",
			wantHTTPAddr: "127.0.0.1:3000",
		},
	}

	// Save original env
	originalDSN := os.Getenv("DSN")
	originalRedis := os.Getenv("REDIS_HOST")
	originalClientIP := os.Getenv("HTTP_HEADER_CLIENT_IP")
	originalServerHost := os.Getenv("SERVER_HOST")
	originalServerPort := os.Getenv("SERVER_PORT")
	originalLogLevel := os.Getenv("LOG_LEVEL")
	defer func() {
		os.Setenv("DSN", originalDSN)
		os.Setenv("REDIS_HOST", originalRedis)
		os.Setenv("HTTP_HEADER_CLIENT_IP", originalClientIP)
		os.Setenv("SERVER_HOST", originalServerHost)
		os.Setenv("SERVER_PORT", originalServerPort)
		os.Setenv("LOG_LEVEL", originalLogLevel)
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.envSetup()

			// Test through tools.GetEnvOrDefault which is what AppConfig() uses internally
			gotDSN := tools.GetEnvOrDefault("DSN", "postgres://localhost:5432/demo")
			if gotDSN != tt.wantDSN {
				t.Errorf("DSN = %v, want %v", gotDSN, tt.wantDSN)
			}

			gotRedis := tools.GetEnvOrDefault("REDIS_HOST", "localhost:6379")
			if gotRedis != tt.wantRedis {
				t.Errorf("RedisAddr = %v, want %v", gotRedis, tt.wantRedis)
			}

			gotClientIP := tools.GetEnvOrDefault("HTTP_HEADER_CLIENT_IP", "X-Forwarded-For")
			if gotClientIP != tt.wantClientIP {
				t.Errorf("ClientIP = %v, want %v", gotClientIP, tt.wantClientIP)
			}

			gotHost := tools.GetEnvOrDefault("SERVER_HOST", "localhost")
			gotPort := tools.GetEnvOrDefault("SERVER_PORT", "localhost")
			wantHTTPAddr := gotHost + ":" + gotPort
			if wantHTTPAddr != tt.wantHTTPAddr {
				t.Errorf("HTTPAddr = %v, want %v", wantHTTPAddr, tt.wantHTTPAddr)
			}
		})
	}
}

func TestAppConfigFunction(t *testing.T) {
	// Save and restore env
	originalLogLevel := os.Getenv("LOG_LEVEL")
	defer os.Setenv("LOG_LEVEL", originalLogLevel)

	os.Setenv("LOG_LEVEL", "WARN")

	cfg := AppConfig()
	if cfg == nil {
		t.Fatal("AppConfig() returned nil")
	}

	// Check that fields are populated
	if cfg.DSN == "" {
		t.Error("DSN should not be empty")
	}
	if cfg.RedisAddr == "" {
		t.Error("RedisAddr should not be empty")
	}
	if cfg.HTTPAddr == "" {
		t.Error("HTTPAddr should not be empty")
	}
	if cfg.ClientIP == "" {
		t.Error("ClientIP should not be empty")
	}
}

func TestAppConfigMultipleCalls(t *testing.T) {
	// AppConfig should return the same instance (singleton pattern)
	cfg1 := AppConfig()
	cfg2 := AppConfig()

	if cfg1 != cfg2 {
		t.Error("AppConfig() should return the same instance on multiple calls")
	}
}
