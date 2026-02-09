package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/Gouryella/supabase-studio-go/internal/config"
)

func securityHeaders(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-Content-Type-Options", "no-sniff")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			if cfg.IsPlatform && os.Getenv("VERCEL") == "1" {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
			}

			w.Header().Set("Content-Security-Policy", "frame-ancestors 'none';")

			next.ServeHTTP(w, r)
		})
	}
}

func cacheControlForPath(path string) string {
	if strings.HasPrefix(path, "/_next/static/") {
		return "public, max-age=31536000, immutable"
	}
	if strings.HasPrefix(path, "/img/") {
		return "public, max-age=2592000"
	}
	if strings.HasPrefix(path, "/favicon/") {
		return "public, max-age=86400"
	}
	return "no-cache"
}
