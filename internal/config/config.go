package config

import (
	"strings"

	"github.com/nil-go/konf"
	"github.com/nil-go/konf/provider/env"
)

type Config struct {
	Server   ServerConfig   `konf:"server"`
	Auth     AuthConfig     `konf:"auth"`
	Database DatabaseConfig `konf:"database"`
	Logging  LoggingConfig  `konf:"logging"`
}

type ServerConfig struct {
	Port         string `konf:"port,default=8080"`
	ReadTimeout  int    `konf:"read_timeout,default=30"`
	WriteTimeout int    `konf:"write_timeout,default=30"`
	IdleTimeout  int    `konf:"idle_timeout,default=60"`
}

type AuthConfig struct {
	APIKeys []string `konf:"api_keys"`
}

type DatabaseConfig struct {
	Driver   string `konf:"driver,default=postgres"`
	DSN      string `konf:"dsn"`
	MaxConns int    `konf:"max_conns,default=25"`
}

type LoggingConfig struct {
	Level string `konf:"level,default=info"`
	JSON  bool   `konf:"json,default=false"`
}

func Load() (*Config, error) {
	k := konf.New()
	cfg := &Config{}

	// load .env file
	if err := k.Load(env.New(env.WithNameSplitter(func(in string) []string {
		return strings.Split(in, "_")
	}))); err != nil {
		return nil, err
	}

	konf.SetDefault(k)
	// Unmarshal to cfg
	if err := konf.Unmarshal("server", &cfg); err != nil {
		// Handle error here.
		return nil, err
	}
	return cfg, nil
}
