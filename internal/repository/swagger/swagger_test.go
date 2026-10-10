package swagger

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockUserService is a mock implementation of UserService for testing.
type mockUserService struct {
	getProfileFunc func(token string) string
}

// mockAuthService is a mock implementation of AuthService for testing.
type mockAuthService struct{}

// mockNotificationService is a mock implementation of NotificationService for testing.
type mockNotificationService struct{}

func TestSwagger_Routes(t *testing.T) {
	routes := Routes("/api/swagger")
	if routes == nil {
		t.Fatal("Routes() returned nil http.Handler")
	}
}

func TestSwagger_FileServer(t *testing.T) {
	prefix := "/api/swagger"
	mux := Routes(prefix)

	req, err := http.NewRequest(http.MethodGet, "/api/swagger/swagger-ui.html", nil)
	if err != nil {
		t.Fatalf("Не удалось создать запрос: %v", err)
	}

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// Если файл есть в api.FS, вернется 200 OK. Если файла нет — 404 Not Found.
	// Здесь мы проверяем, что отработал именно fileServer (не было редиректа)
	if rr.Code == http.StatusMovedPermanently {
		t.Errorf("Не ожидал редирект для пути /swagger-ui.html")
	}
}

func TestSwagger_RedirectRoot(t *testing.T) {
	prefix := "/api/swagger"
	mux := Routes(prefix)

	// Создаем тестовый запрос к корню "/"
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("Не удалось создать запрос: %v", err)
	}

	// ResponseRecorder выступает в роли заглушки для ответа
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// Проверяем статус код 301 (StatusMovedPermanently)
	if rr.Code != http.StatusMovedPermanently {
		t.Errorf("Ожидался статус %d, получили %d", http.StatusMovedPermanently, rr.Code)
	}

	// Проверяем правильность URL для перенаправления в заголовке Location
	expectedLocation := fmt.Sprintf("%s/swagger-ui.html", prefix)
	actualLocation := rr.Header().Get("Location")
	if actualLocation != expectedLocation {
		t.Errorf("Ожидался Location %q, получили %q", expectedLocation, actualLocation)
	}
}
