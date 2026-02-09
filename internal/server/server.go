package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Gouryella/supabase-studio-go/internal/api"
	"github.com/Gouryella/supabase-studio-go/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func New(cfg config.Config) http.Handler {
	static, err := staticFS()
	if err != nil {
		log.Printf("failed to load embedded static assets: %v", err)
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(120 * time.Second))
	router.Use(securityHeaders(cfg))
	router.Use(gzipMiddleware())

	registerRedirects(router, cfg)

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	router.Get("/env.js", envHandler(cfg))

	router.Mount("/api", api.NewRouter(cfg))

	if static != nil {
		router.NotFound(spaHandler(static, cfg))
	}

	if cfg.BasePath != "" {
		base := strings.TrimSuffix(cfg.BasePath, "/")
		wrapper := chi.NewRouter()
		wrapper.Mount(base, router)
		wrapper.Get("/", func(w http.ResponseWriter, r *http.Request) {
			target := base
			if !strings.HasSuffix(target, "/") {
				target += "/"
			}
			http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		})
		return wrapper
	}

	return router
}
