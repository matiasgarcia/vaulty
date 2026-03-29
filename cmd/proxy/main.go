package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pci-vault/vault/config"
	"github.com/pci-vault/vault/internal/auth"
	"github.com/pci-vault/vault/internal/handler"
	"github.com/pci-vault/vault/internal/proxy"
	"github.com/pci-vault/vault/internal/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	tokenizerBaseURL := os.Getenv("TOKENIZER_BASE_URL")
	if tokenizerBaseURL == "" {
		tokenizerBaseURL = "http://localhost:" + cfg.PortTokenizer
	}

	// HTTP client for Tokenizer internal API (mTLS in production)
	tokenizerClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// HTTP client for forwarding to 3rd party destinations
	forwardClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	proxyAuth := os.Getenv("PROXY_AUTH_HEADER")
	if proxyAuth == "" {
		proxyAuth = "Bearer proxy:secret"
	}
	revealer := proxy.NewRevealer(tokenizerClient, tokenizerBaseURL, proxyAuth)
	forwarder := proxy.NewForwarder(forwardClient)
	forwardHandler := handler.NewForwardHandler(revealer, forwarder)
	healthHandler := handler.NewProxyHealthHandler()

	r := server.NewRouter()

	// Health — no auth
	r.Get("/health", healthHandler.ServeHTTP)

	// Authenticated + tenant-scoped routes
	r.Group(func(r chi.Router) {
		r.Use(auth.BearerAuth)
		r.Use(auth.ExtractTenantHeader)
		r.Post("/proxy/forward", forwardHandler.ServeHTTP)
	})

	if err := server.Run(r, cfg.PortProxy); err != nil {
		log.Fatalf("server: %v", err)
	}
}
