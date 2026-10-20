package main

import (
	"net/http"
	"os"
	"time"

	"github.com/botbooker/bb-core/internal/config"
)

func main() {
	// Создаем клиента с жестким таймаутом на весь запрос (включая коннект и чтение тела)
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	// Выполняем HTTP-запрос к локальному серверу
	resp, err := client.Get("http://" + config.AppConfig().HTTPAddr + "/health")

	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1) // Контейнер болен
	}
	// Важно: закрываем тело сразу после успешной проверки err
	resp.Body.Close()

	os.Exit(0) // Контейнер здоров
}
