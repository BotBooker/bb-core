package serverapi

import (
	"net/http"

	"github.com/botbooker/bb-core/pkg/api"
)

func initSwaggerUI() http.Handler {
	// Создаем файловый сервер для swagger-ui
	return http.FileServer(http.FS(api.FS))
}
