// Проба готовности для Docker HEALTHCHECK.
//
// Логика живёт в бинарнике api, а не в отдельной команде: HEALTHCHECK запускает
// новый процесс того же образа (/app/api --readycheck), поэтому в scratch-образе
// не нужен второй бинарник, а проба видит ровно тот же конфиг окружения, что и
// сервер. Запрос идёт на /ready, а не на /health: readiness проверяет доступность
// базы, поэтому контейнер не считается здоровым, пока приложение не может
// обслуживать запросы. Liveness (перезапуск пода) остаётся за /health — падение
// базы перезапуском контейнера не лечится.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/botbooker/bb-core/internal/config"
)

// readyPath — ручка готовности приложения.
const readyPath = "/ready"

// readycheckTimeout — бюджет запроса: HEALTHCHECK Docker даёт 3 с, дольше
// ждать смысла нет.
const readycheckTimeout = 3 * time.Second

// loopback — адрес, по которому проба подключается к серверу, слушающему все
// интерфейсы: 0.0.0.0 и :: — адреса для bind, а не для dial.
const loopback = "127.0.0.1"

// readycheckAddr — адрес HTTP-сервера, который опрашивает проба. Это
// переменная, а не прямой вызов config.AppConfig(): тесты подставляют адрес
// httptest-сервера и не зависят от переменных окружения.
var readycheckAddr = func() string {
	return config.AppConfig().HTTPAddr
}

// readycheckExitCode выполняет пробу и возвращает код возврата процесса:
// 0 — экземпляр готов, 1 — не готов или недоступен.
func readycheckExitCode() int {
	if err := runReadycheck(readycheckAddr()); err != nil {
		slog.ErrorContext(context.Background(), "readiness check failed", "error", err)

		return exitCodeFailure
	}

	return 0
}

// runReadycheck опрашивает GET /ready и возвращает ошибку, если экземпляр не
// готов. Код возврата выставляет вызывающий: так os.Exit не обходит отложенное
// закрытие тела ответа.
func runReadycheck(addr string) error {
	target := readycheckURL(addr)

	client := &http.Client{Timeout: readycheckTimeout}

	resp, err := client.Get(target)
	if err != nil {
		return fmt.Errorf("get %s: %w", target, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %s", target, resp.Status)
	}

	return nil
}

// readycheckURL собирает адрес ручки готовности из адреса HTTP-сервера.
//
// Приложение по умолчанию слушает все интерфейсы (в Docker и Kubernetes —
// SERVER_HOST=0.0.0.0), а подключаться к 0.0.0.0/:: как к адресу назначения
// нельзя: на Linux это работает по случайности, на других ОС — нет. Поэтому
// unspecified-хост заменяется на loopback: проба всегда идёт в свой контейнер.
func readycheckURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Адрес без порта не угадываем: клиент вернёт осмысленную ошибку,
		// вместо того чтобы молча пойти на чужой порт.
		return "http://" + addr + readyPath
	}

	if host == "" || isUnspecifiedHost(host) {
		host = loopback
	}

	return "http://" + net.JoinHostPort(host, port) + readyPath
}

// isUnspecifiedHost сообщает, слушает ли хост все интерфейсы (0.0.0.0, ::).
func isUnspecifiedHost(host string) bool {
	ip := net.ParseIP(host)

	return ip != nil && ip.IsUnspecified()
}
