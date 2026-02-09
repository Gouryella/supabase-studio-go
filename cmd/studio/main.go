package main

import (
	"log"
	"net/http"
	"time"

	"github.com/Gouryella/supabase-studio-go/internal/config"
	"github.com/Gouryella/supabase-studio-go/internal/server"
)

func main() {
	cfg := config.Load()

	handler := server.New(cfg)

	addr := cfg.ListenAddress
	if addr == "" {
		addr = ":3000"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("supabase-studio-go listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}
