package cache

import (
	"log/slog"
	"strings"
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
	slog.Debug("connected to the cache", "addr", redactAddr(addr))
	return &redisCache{addr: addr}
}

// redactAddr скрывает учётные данные из адреса подключения: в логах
// не должно быть пароля (например, redis://default:secret@localhost:6379/0).
//
// Разбор строки намеренно простой: адрес — произвольная строка из конфигурации,
// а не гарантированно корректный URL. Например, в redis://default:re#d@host/0
// пароль содержит "#", из-за чего url.Parse считает остаток фрагментом
// и не находит userinfo. Поэтому отбрасываем всё до последней "@".
func redactAddr(addr string) string {
	at := strings.LastIndex(addr, "@")
	if at == -1 {
		return addr
	}

	schemeEnd := strings.Index(addr, "://")
	if schemeEnd == -1 {
		return addr[at+1:]
	}

	return addr[:schemeEnd+3] + addr[at+1:]
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
	slog.Debug("connection to the cache closed")
	return nil
}
