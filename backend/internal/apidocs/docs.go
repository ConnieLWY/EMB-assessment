package apidocs

import (
	_ "embed"
	"net/http"

	swaggerFiles "github.com/swaggo/files/v2"
)

//go:embed index.html
var index []byte

//go:embed swagger.json
var spec []byte

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /swagger/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
	mux.HandleFunc("GET /swagger/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(spec)
	})
	assets := http.StripPrefix("/swagger/", http.FileServerFS(swaggerFiles.FS))
	mux.Handle("GET /swagger/swagger-ui.css", assets)
	mux.Handle("GET /swagger/swagger-ui-bundle.js", assets)
}
