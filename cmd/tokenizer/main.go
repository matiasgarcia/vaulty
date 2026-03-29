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
	"github.com/pci-vault/vault/internal/audit"
	"github.com/pci-vault/vault/internal/auth"
	"github.com/pci-vault/vault/internal/handler"
	"github.com/pci-vault/vault/internal/kms"
	vaultredis "github.com/pci-vault/vault/internal/redis"
	"github.com/pci-vault/vault/internal/repository"
	"github.com/pci-vault/vault/internal/server"
)

func main() {
	ctx := context.Background()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("init postgres: %v", err)
	}
	defer pool.Close()

	redisOpts, err := goredis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("parse redis url: %v", err)
	}
	rdb := goredis.NewClient(redisOpts)
	defer rdb.Close()

	kmsClient, err := kms.New(ctx, cfg.KMSKeyARN, cfg.AWSRegion, cfg.KMSEndpoint)
	if err != nil {
		log.Fatalf("init kms: %v", err)
	}

	hmacKey, err := base64.StdEncoding.DecodeString(cfg.HMACKey)
	if err != nil {
		log.Fatalf("decode hmac key: %v", err)
	}

	// Repositories
	tokenRepo := repository.NewTokenRepo(pool)
	vaultRepo := repository.NewVaultRepo(pool)
	auditRepo := repository.NewAuditRepo(pool)
	tenantRepo := repository.NewTenantRepo(pool)

	// Services
	cvvStore := vaultredis.NewCVVStore(rdb)
	auditLogger := audit.NewLogger(auditRepo)

	// Handlers
	tokenizeHandler := handler.NewTokenizeHandler(
		tokenRepo, vaultRepo, cvvStore, kmsClient, auditLogger,
		hmacKey, cfg.CVVTTL, cfg.KMSKeyARN,
	)
	detokenizeHandler := handler.NewDetokenizeHandler(
		tokenRepo, vaultRepo, cvvStore, kmsClient, auditLogger,
	)
	tokenManageHandler := handler.NewTokenManageHandler(tokenRepo, auditRepo, cvvStore, auditLogger)
	healthHandler := handler.NewHealthHandler(pool, rdb, func(ctx context.Context) bool {
		_, _, err := kmsClient.GenerateDataKey(ctx)
		return err == nil
	})

	tenantHandler := handler.NewTenantHandler(tenantRepo, kmsClient)
	tenantMiddleware := auth.RequireTenant(tenantRepo)

	// Router
	r := server.NewRouter()
	r.Get("/health", healthHandler.ServeHTTP)

	// Admin routes — no tenant middleware (admin operates cross-tenant)
	r.Group(func(r chi.Router) {
		r.Use(auth.BearerAuth)
		r.Post("/admin/tenants", tenantHandler.Create)
		r.Get("/admin/tenants", tenantHandler.List)
		r.Get("/admin/tenants/{tenant_id}", tenantHandler.Get)
		r.Delete("/admin/tenants/{tenant_id}", tenantHandler.Deactivate)
	})

	// Authenticated + tenant-scoped routes
	r.Group(func(r chi.Router) {
		r.Use(auth.BearerAuth)
		r.Use(tenantMiddleware)

		r.Post("/vault/tokenize", tokenizeHandler.ServeHTTP)
		r.Get("/vault/tokens/{token_id}", tokenManageHandler.GetStatus)
		r.Delete("/vault/tokens/{token_id}", tokenManageHandler.Deactivate)
		r.Get("/vault/tokens/{token_id}/audit", tokenManageHandler.GetAuditTrail)

		r.With(auth.RequireMTLSCert("proxy")).
			Post("/internal/detokenize", detokenizeHandler.ServeHTTP)
	})

	if err := server.Run(r, cfg.PortTokenizer); err != nil {
		log.Fatalf("server: %v", err)
	}
}
