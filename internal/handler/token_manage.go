package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pci-vault/vault/internal/audit"
	"github.com/pci-vault/vault/internal/auth"
	"github.com/pci-vault/vault/internal/model"
	vaultredis "github.com/pci-vault/vault/internal/redis"
	"github.com/pci-vault/vault/internal/repository"
	"github.com/pci-vault/vault/internal/server"
)

type TokenStatusResponse struct {
	Token      string  `json:"token"`
	Status     string  `json:"status"`
	Type       string  `json:"type"`
	CreatedAt  string  `json:"created_at,omitempty"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	TTLSeconds *int    `json:"ttl_seconds,omitempty"`
}

type AuditTrailResponse struct {
	Entries []model.AuditLogEntry `json:"entries"`
	Total   int                   `json:"total"`
}

type TokenManageHandler struct {
	tokenRepo *repository.TokenRepo
	auditRepo *repository.AuditRepo
	cvvStore  *vaultredis.CVVStore
	audit     *audit.Logger
}

func NewTokenManageHandler(
	tokenRepo *repository.TokenRepo,
	auditRepo *repository.AuditRepo,
	cvvStore *vaultredis.CVVStore,
	auditLogger *audit.Logger,
) *TokenManageHandler {
	return &TokenManageHandler{
		tokenRepo: tokenRepo,
		auditRepo: auditRepo,
		cvvStore:  cvvStore,
		audit:     auditLogger,
	}
}

func (h *TokenManageHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)
	tokenID := chi.URLParam(r, "token_id")

	tenant := auth.GetTenant(ctx)
	if tenant == nil {
		server.BadRequest(w, "MISSING_TENANT", "Tenant context required", correlationID)
		return
	}

	// Try PAN token first (PostgreSQL)
	token, err := h.tokenRepo.FindByTokenID(ctx, tenant.TenantID, tokenID)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}

	if token != nil {
		h.audit.Log(ctx, &model.AuditLogEntry{
			TenantID:      tenant.TenantID,
			CorrelationID: correlationID,
			Operation:     model.AuditOpValidate,
			TokenIDMasked: audit.MaskTokenID(tokenID),
			Actor:         "api",
			Result:        model.AuditResultSuccess,
		})

		resp := TokenStatusResponse{
			Token:     token.TokenID,
			Status:    string(token.Status),
			Type:      "pan",
			CreatedAt: token.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if token.LastUsedAt != nil {
			ts := token.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
			resp.LastUsedAt = &ts
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Try CVV token (Redis)
	if ttl, exists := h.cvvStore.ExistsCVVToken(ctx, tenant.TenantID, tokenID); exists {
		h.audit.Log(ctx, &model.AuditLogEntry{
			TenantID:      tenant.TenantID,
			CorrelationID: correlationID,
			Operation:     model.AuditOpValidate,
			TokenIDMasked: audit.MaskTokenID(tokenID),
			Actor:         "api",
			Result:        model.AuditResultSuccess,
		})

		ttlSec := int(ttl / time.Second)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenStatusResponse{
			Token:      tokenID,
			Status:     "active",
			Type:       "cvv",
			TTLSeconds: &ttlSec,
		})
		return
	}

	server.NotFound(w, "TOKEN_NOT_FOUND", "Token not found", correlationID)
}

func (h *TokenManageHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)
	tokenID := chi.URLParam(r, "token_id")

	tenant := auth.GetTenant(ctx)
	if tenant == nil {
		server.BadRequest(w, "MISSING_TENANT", "Tenant context required", correlationID)
		return
	}

	token, err := h.tokenRepo.FindByTokenID(ctx, tenant.TenantID, tokenID)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}
	if token == nil {
		server.NotFound(w, "TOKEN_NOT_FOUND", "Token not found", correlationID)
		return
	}

	if err := h.tokenRepo.UpdateStatus(ctx, tenant.TenantID, tokenID, model.TokenStatusInactive); err != nil {
		server.InternalError(w, correlationID)
		return
	}

	h.audit.Log(ctx, &model.AuditLogEntry{
		TenantID:      tenant.TenantID,
		CorrelationID: correlationID,
		Operation:     model.AuditOpDeactivate,
		TokenIDMasked: audit.MaskTokenID(tokenID),
		Actor:         "api",
		Result:        model.AuditResultSuccess,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TokenStatusResponse{
		Token:     tokenID,
		Status:    string(model.TokenStatusInactive),
		CreatedAt: token.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *TokenManageHandler) GetAuditTrail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)
	tokenID := chi.URLParam(r, "token_id")

	tenant := auth.GetTenant(ctx)
	if tenant == nil {
		server.BadRequest(w, "MISSING_TENANT", "Tenant context required", correlationID)
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	maskedID := audit.MaskTokenID(tokenID)
	entries, total, err := h.auditRepo.FindByTokenID(ctx, tenant.TenantID, maskedID, limit, offset)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AuditTrailResponse{
		Entries: entries,
		Total:   total,
	})
}
