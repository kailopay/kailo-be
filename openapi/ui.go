package openapi

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed index.html openapi.yaml
var documentationFiles embed.FS

// UIHandler serves the Swagger UI page and the OpenAPI document it renders.
func UIHandler() http.Handler {
	files := http.FileServer(http.FS(documentationFiles))

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path := strings.TrimPrefix(request.URL.Path, "/")
		switch path {
		case "", "index.html":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		case "openapi.yaml":
			writer.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		default:
			http.NotFound(writer, request)
			return
		}

		files.ServeHTTP(writer, request)
	})
}
