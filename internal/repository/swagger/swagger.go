package swagger

import (
	"net/http"

	"github.com/botbooker/bb-core/pkg/api"
)

func Routes(prefix string) *http.ServeMux {
	// Swagger UI эндпоинты
	mux := http.NewServeMux()
	fileServer := http.FileServer(http.FS(api.FS))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, prefix+"/swagger-ui.html", http.StatusMovedPermanently)
			return
		}
		fileServer.ServeHTTP(w, r)
	}))

	return mux
}
