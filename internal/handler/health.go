package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type HealthChecks struct {
	Database string `json:"database"`
	Redis    string `json:"redis"`
	KMS      string `json:"kms"`
}

type HealthResponse struct {
	Status string       `json:"status"`
	Checks HealthChecks `json:"checks"`
}

type HealthHandler struct {
	pool   *pgxpool.Pool
	rdb    *redis.Client
	kmsOK  func(ctx context.Context) bool
}

func NewHealthHandler(pool *pgxpool.Pool, rdb *redis.Client, kmsCheck func(ctx context.Context) bool) *HealthHandler {
	return &HealthHandler{pool: pool, rdb: rdb, kmsOK: kmsCheck}
}

// NewProxyHealthHandler creates a health handler for the Proxy (no DB, no Redis).
func NewProxyHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resp := HealthResponse{Status: "healthy"}
	allUp := true

	if h.pool != nil {
		if err := h.pool.Ping(ctx); err != nil {
			resp.Checks.Database = "down"
			allUp = false
		} else {
			resp.Checks.Database = "up"
		}
	}

	if h.rdb != nil {
		if err := h.rdb.Ping(ctx).Err(); err != nil {
			resp.Checks.Redis = "down"
			allUp = false
		} else {
			resp.Checks.Redis = "up"
		}
	}

	if h.kmsOK != nil {
		if h.kmsOK(ctx) {
			resp.Checks.KMS = "up"
		} else {
			resp.Checks.KMS = "down"
			allUp = false
		}
	}

	if !allUp {
		resp.Status = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
