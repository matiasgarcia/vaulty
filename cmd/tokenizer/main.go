package main

import (
	"context"
	"encoding/base64"
	"log"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/go-chi/chi/v5"

	"github.com/pci-vault/vault/config"
	"github.com/pci-vault/vault/internal/auth"
	"github.com/pci-vault/vault/internal/audit"
	"github.com/pci-vault/vault/internal/handler"
	"github.com/pci-vault/vault/internal/kms"
	vaultredis "github.com/pci-vault/vault/internal/redis"
	"github.com/pci-vault/vault/internal/repository"
	"github.com/pci-vault/vault/internal/server"
)

func main() {
	ctx := context.Background()

	// Structured JSON logging
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// PostgreSQL
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("init postgres: %v", err)
	}
	defer pool.Close()

	// Redis
	redisOpts, err := goredis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("parse redis url: %v", err)
	}
	rdb := goredis.NewClient(redisOpts)
	defer rdb.Close()

	// KMS
	kmsClient, err := kms.New(ctx, cfg.KMSKeyARN, cfg.AWSRegion, cfg.KMSEndpoint)
	if err != nil {
		log.Fatalf("init kms: %v", err)
	}

	// HMAC key
	hmacKey, err := base64.StdEncoding.DecodeString(cfg.HMACKey)
	if err != nil {
		log.Fatalf("decode hmac key: %v", err)
	}

	// Repositories
	tokenRepo := repository.NewTokenRepo(pool)
	vaultRepo := repository.NewVaultRepo(pool)
	auditRepo := repository.NewAuditRepo(pool)

	// Services
	cvvStore := vaultredis.NewCVVStore(rdb)
	auditLogger := audit.NewLogger(auditRepo)

	// Handlers
	tokenizeHandler := handler.NewTokenizeHandler(
		tokenRepo, vaultRepo, cvvStore, kmsClient, auditLogger,
		hmacKey, cfg.CVVTTL, cfg.KMSKeyARN,
	)
	healthHandler := handler.NewHealthHandler(pool, rdb, func(ctx context.Context) bool {
		_, _, err := kmsClient.GenerateDataKey(ctx)
		return err == nil
	})

	detokenizeHandler := handler.NewDetokenizeHandler(
		tokenRepo, vaultRepo, cvvStore, kmsClient, auditLogger,
	)

	tokenManageHandler := handler.NewTokenManageHandler(tokenRepo, auditRepo, auditLogger)

	// Router
	r := server.NewRouter()

	// Health — no auth
	r.Get("/health", healthHandler.ServeHTTP)

	// Authenticated routes
	r.Group(func(r chi.Router) {
		r.Use(auth.BearerAuth)

		// Tokenization — requires "tokenize" or "manage" role
		r.Post("/vault/tokenize", tokenizeHandler.ServeHTTP)

		// Token management — requires "manage" role
		r.Get("/vault/tokens/{token_id}", tokenManageHandler.GetStatus)
		r.Delete("/vault/tokens/{token_id}", tokenManageHandler.Deactivate)
		r.Get("/vault/tokens/{token_id}/audit", tokenManageHandler.GetAuditTrail)

		// Internal detokenize — requires mTLS cert from Proxy
		r.With(auth.RequireMTLSCert("proxy")).
			Post("/internal/detokenize", detokenizeHandler.ServeHTTP)
	})

	// Start
	if err := server.Run(r, cfg.PortTokenizer); err != nil {
		log.Fatalf("server: %v", err)
	}
}
