package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/pci-vault/vault/internal/kms"
	"github.com/pci-vault/vault/internal/model"
	"github.com/pci-vault/vault/internal/repository"
	"github.com/pci-vault/vault/internal/server"
)

var tenantIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type CreateTenantRequest struct {
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
}

type TenantResponse struct {
	TenantID  string `json:"tenant_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	KMSKeyARN string `json:"kms_key_arn,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type TenantsListResponse struct {
	Tenants []TenantResponse `json:"tenants"`
	Total   int              `json:"total"`
}

type TenantHandler struct {
	tenantRepo *repository.TenantRepo
	kmsClient  *kms.Client
}

func NewTenantHandler(tenantRepo *repository.TenantRepo, kmsClient *kms.Client) *TenantHandler {
	return &TenantHandler{tenantRepo: tenantRepo, kmsClient: kmsClient}
}

func (h *TenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)

	var req CreateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.BadRequest(w, "INVALID_JSON", "Invalid request body", correlationID)
		return
	}

	if !tenantIDPattern.MatchString(req.TenantID) {
		server.BadRequest(w, "INVALID_TENANT_ID", "Tenant ID must be 1-64 alphanumeric/dash/underscore chars", correlationID)
		return
	}
	if req.Name == "" {
		server.BadRequest(w, "MISSING_NAME", "Tenant name is required", correlationID)
		return
	}

	existing, err := h.tenantRepo.FindByTenantID(ctx, req.TenantID)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "TENANT_EXISTS", "Tenant ID already exists", correlationID)
		return
	}

	keyARN, err := h.kmsClient.CreateKey(ctx, fmt.Sprintf("KEK for tenant %s", req.TenantID))
	if err != nil {
		server.ServiceUnavailable(w, "Failed to provision KMS key", correlationID)
		return
	}

	tenant := &model.Tenant{
		TenantID:  req.TenantID,
		Name:      req.Name,
		Status:    model.TenantStatusActive,
		KMSKeyARN: keyARN,
	}

	if err := h.tenantRepo.Create(ctx, tenant); err != nil {
		server.InternalError(w, correlationID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toTenantResponse(tenant))
}

func (h *TenantHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)
	tenantID := chi.URLParam(r, "tenant_id")

	tenant, err := h.tenantRepo.FindByTenantID(ctx, tenantID)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}
	if tenant == nil {
		server.NotFound(w, "TENANT_NOT_FOUND", "Tenant not found", correlationID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toTenantResponse(tenant))
}

func (h *TenantHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)

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

	tenants, total, err := h.tenantRepo.List(ctx, limit, offset)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}

	resp := TenantsListResponse{Total: total}
	for i := range tenants {
		resp.Tenants = append(resp.Tenants, *toTenantResponse(&tenants[i]))
	}
	if resp.Tenants == nil {
		resp.Tenants = []TenantResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *TenantHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)
	tenantID := chi.URLParam(r, "tenant_id")

	tenant, err := h.tenantRepo.FindByTenantID(ctx, tenantID)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}
	if tenant == nil {
		server.NotFound(w, "TENANT_NOT_FOUND", "Tenant not found", correlationID)
		return
	}

	if err := h.tenantRepo.UpdateStatus(ctx, tenantID, model.TenantStatusInactive); err != nil {
		server.InternalError(w, correlationID)
		return
	}

	tenant.Status = model.TenantStatusInactive
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toTenantResponse(tenant))
}

func toTenantResponse(t *model.Tenant) *TenantResponse {
	return &TenantResponse{
		TenantID:  t.TenantID,
		Name:      t.Name,
		Status:    string(t.Status),
		KMSKeyARN: t.KMSKeyARN,
		CreatedAt: t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func writeError(w http.ResponseWriter, status int, code, message, correlationID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(server.ErrorResponse{
		Error: server.ErrorBody{
			Code:          code,
			Message:       message,
			CorrelationID: correlationID,
		},
	})
}
