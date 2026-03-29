package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/pci-vault/vault/internal/audit"
	"github.com/pci-vault/vault/internal/auth"
	"github.com/pci-vault/vault/internal/crypto"
	"github.com/pci-vault/vault/internal/kms"
	"github.com/pci-vault/vault/internal/model"
	vaultredis "github.com/pci-vault/vault/internal/redis"
	"github.com/pci-vault/vault/internal/repository"
	"github.com/pci-vault/vault/internal/server"
)

type DetokenizeRequest struct {
	Token string `json:"token"`
}

type DetokenizeResponse struct {
	PAN         string  `json:"pan"`
	ExpiryMonth int     `json:"expiry_month"`
	ExpiryYear  int     `json:"expiry_year"`
	CVV         *string `json:"cvv"`
}

type DetokenizeHandler struct {
	tokenRepo *repository.TokenRepo
	vaultRepo *repository.VaultRepo
	cvvStore  *vaultredis.CVVStore
	kmsClient *kms.Client
	audit     *audit.Logger
}

func NewDetokenizeHandler(
	tokenRepo *repository.TokenRepo,
	vaultRepo *repository.VaultRepo,
	cvvStore *vaultredis.CVVStore,
	kmsClient *kms.Client,
	auditLogger *audit.Logger,
) *DetokenizeHandler {
	return &DetokenizeHandler{
		tokenRepo: tokenRepo,
		vaultRepo: vaultRepo,
		cvvStore:  cvvStore,
		kmsClient: kmsClient,
		audit:     auditLogger,
	}
}

func (h *DetokenizeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)

	tenant := auth.GetTenant(ctx)
	if tenant == nil {
		server.BadRequest(w, "MISSING_TENANT", "Tenant context required", correlationID)
		return
	}
	tenantID := tenant.TenantID

	var req DetokenizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.BadRequest(w, "INVALID_JSON", "Invalid request body", correlationID)
		return
	}

	token, err := h.tokenRepo.FindByTokenID(ctx, tenantID, req.Token)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}
	if token == nil || token.Status != model.TokenStatusActive {
		server.NotFound(w, "TOKEN_NOT_FOUND", "Token not found or inactive", correlationID)
		return
	}

	pan, err := h.decryptPAN(ctx, tenantID, req.Token)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}

	h.tokenRepo.UpdateLastUsed(ctx, tenantID, req.Token)

	var cvvPtr *string
	cvv := h.retrieveCVV(ctx, tenantID, req.Token)
	if cvv != "" {
		cvvPtr = &cvv
	}

	h.audit.Log(ctx, &model.AuditLogEntry{
		TenantID:      tenantID,
		CorrelationID: correlationID,
		Operation:     model.AuditOpDetokenize,
		TokenIDMasked: audit.MaskTokenID(req.Token),
		Actor:         "proxy",
		Result:        model.AuditResultSuccess,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DetokenizeResponse{
		PAN:         pan,
		ExpiryMonth: token.ExpiryMonth,
		ExpiryYear:  token.ExpiryYear,
		CVV:         cvvPtr,
	})
}

func (h *DetokenizeHandler) decryptPAN(ctx context.Context, tenantID, tokenID string) (string, error) {
	ve, err := h.vaultRepo.FindByTokenID(ctx, tenantID, tokenID)
	if err != nil || ve == nil {
		return "", err
	}

	dek, err := h.kmsClient.UnwrapKey(ctx, ve.DEKEncrypted)
	if err != nil {
		return "", err
	}
	defer clearBytes(dek)

	fullCiphertext := append(ve.PANCiphertext, ve.AuthTag...)
	plaintext, err := crypto.DecryptWithDEK(fullCiphertext, ve.IV, dek)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (h *DetokenizeHandler) retrieveCVV(ctx context.Context, tenantID, tokenID string) string {
	combined, err := h.cvvStore.Retrieve(ctx, tenantID, tokenID)
	if err != nil || combined == nil {
		return ""
	}

	dekSize := 32
	if len(combined) <= dekSize {
		return ""
	}

	dek := combined[:dekSize]
	encrypted := combined[dekSize:]
	defer clearBytes(dek)

	if len(encrypted) < 12 {
		return ""
	}
	iv := encrypted[:12]
	ciphertext := encrypted[12:]

	plaintext, err := crypto.DecryptWithDEK(ciphertext, iv, dek)
	if err != nil {
		return ""
	}

	return string(plaintext)
}
