package cache

import (
	"log/slog"
)

// Cache — интерфейс подключения к кэшу.
type Cache interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Close() error
}

// redisCache — конкретная реализация, скрыта от внешних пакетов.
type redisCache struct {
	addr string
}

// New создаёт подключение к кэшу.
func New(addr string) Cache {
	slog.Debug("подключились к кэшу", "addr", addr)
	return &redisCache{addr: addr}
}

// Get получает значение по ключу.
func (c *redisCache) Get(key string) (string, error) {
	return key, nil
}

// Set устанавливает значение по ключу.
func (c *redisCache) Set(key, value string) error {
	slog.Debug("cache.Set", key, value)
	return nil
}

// Close закрывает подключение к кэшу.
func (c *redisCache) Close() error {
	slog.Debug("подключение к кэшу закрыто")
	return nil
}
