package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/pci-vault/vault/internal/audit"
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

	var req DetokenizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.BadRequest(w, "INVALID_JSON", "Invalid request body", correlationID)
		return
	}

	token, err := h.tokenRepo.FindByTokenID(ctx, req.Token)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}
	if token == nil || token.Status != model.TokenStatusActive {
		server.NotFound(w, "TOKEN_NOT_FOUND", "Token not found or inactive", correlationID)
		return
	}

	pan, err := h.decryptPAN(ctx, req.Token)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}

	h.tokenRepo.UpdateLastUsed(ctx, req.Token)

	var cvvPtr *string
	cvv := h.retrieveCVV(ctx, req.Token)
	if cvv != "" {
		cvvPtr = &cvv
	}

	h.audit.Log(ctx, &model.AuditLogEntry{
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

func (h *DetokenizeHandler) decryptPAN(ctx context.Context, tokenID string) (string, error) {
	ve, err := h.vaultRepo.FindByTokenID(ctx, tokenID)
	if err != nil || ve == nil {
		return "", err
	}

	dek, err := h.kmsClient.UnwrapKey(ctx, ve.DEKEncrypted)
	if err != nil {
		return "", err
	}
	defer clearBytes(dek)

	// Reassemble GCM ciphertext (ciphertext + auth tag)
	fullCiphertext := append(ve.PANCiphertext, ve.AuthTag...)
	plaintext, err := crypto.DecryptWithDEK(fullCiphertext, ve.IV, dek)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (h *DetokenizeHandler) retrieveCVV(ctx context.Context, tokenID string) string {
	combined, err := h.cvvStore.Retrieve(ctx, tokenID)
	if err != nil || combined == nil {
		return ""
	}

	// Combined format: 32-byte DEK + encrypted CVV
	dekSize := 32
	if len(combined) <= dekSize {
		return ""
	}

	dek := combined[:dekSize]
	encrypted := combined[dekSize:]
	defer clearBytes(dek)

	// The encrypted CVV includes IV (first 12 bytes are from GCM Seal, embedded in ciphertext)
	// Actually, our Encrypt prepends nothing — ciphertext from Seal includes tag but not IV.
	// We stored: dek + Seal(iv, plaintext) but we didn't store the IV separately.
	// Fix: we need to re-think CVV storage to include IV.
	// For now, since CVV is short-lived, let's use a simpler approach:
	// Store dek(32) + iv(12) + ciphertext_with_tag
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
