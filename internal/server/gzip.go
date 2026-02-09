package server

import (
	"net/http"
	"strings"

	"github.com/NYTimes/gziphandler"
)

func gzipMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		gzipHandler := gziphandler.GzipHandler(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldBypassGzip(r) {
				next.ServeHTTP(w, r)
				return
			}
			gzipHandler.ServeHTTP(w, r)
		})
	}
}

func shouldBypassGzip(r *http.Request) bool {
	path := r.URL.Path
	accept := strings.ToLower(r.Header.Get("Accept"))

	// SSE endpoints should not be compressed to avoid buffering and delayed flushes.
	if strings.Contains(path, "/api/ai/") {
		return true
	}
	if strings.Contains(accept, "text/event-stream") {
		return true
	}
	return false
}
