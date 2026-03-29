package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/pci-vault/vault/internal/audit"
	"github.com/pci-vault/vault/internal/auth"
	"github.com/pci-vault/vault/internal/crypto"
	"github.com/pci-vault/vault/internal/kms"
	"github.com/pci-vault/vault/internal/model"
	vaultredis "github.com/pci-vault/vault/internal/redis"
	"github.com/pci-vault/vault/internal/repository"
	"github.com/pci-vault/vault/internal/server"
)

type TokenizeRequest struct {
	PAN         string `json:"pan"`
	ExpiryMonth int    `json:"expiry_month"`
	ExpiryYear  int    `json:"expiry_year"`
	CVV         string `json:"cvv"`
}

type TokenizeResponse struct {
	Token      string `json:"token"`
	CVVStored  bool   `json:"cvv_stored"`
	IsExisting bool   `json:"is_existing"`
}

type TokenizeHandler struct {
	tokenRepo *repository.TokenRepo
	vaultRepo *repository.VaultRepo
	cvvStore  *vaultredis.CVVStore
	kmsClient *kms.Client
	audit     *audit.Logger
	hmacKey   []byte
	cvvTTL    time.Duration
	kmsKeyARN string
}

func NewTokenizeHandler(
	tokenRepo *repository.TokenRepo,
	vaultRepo *repository.VaultRepo,
	cvvStore *vaultredis.CVVStore,
	kmsClient *kms.Client,
	auditLogger *audit.Logger,
	hmacKey []byte,
	cvvTTL time.Duration,
	kmsKeyARN string,
) *TokenizeHandler {
	return &TokenizeHandler{
		tokenRepo: tokenRepo,
		vaultRepo: vaultRepo,
		cvvStore:  cvvStore,
		kmsClient: kmsClient,
		audit:     auditLogger,
		hmacKey:   hmacKey,
		cvvTTL:    cvvTTL,
		kmsKeyARN: kmsKeyARN,
	}
}

func (h *TokenizeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)

	var req TokenizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.BadRequest(w, "INVALID_JSON", "Invalid request body", correlationID)
		return
	}

	if !validatePAN(req.PAN) {
		server.BadRequest(w, "INVALID_PAN", "PAN failed Luhn validation", correlationID)
		return
	}

	if req.ExpiryMonth < 1 || req.ExpiryMonth > 12 {
		server.BadRequest(w, "INVALID_EXPIRY", "Expiry month must be 1-12", correlationID)
		return
	}

	if req.ExpiryYear < time.Now().Year() {
		server.BadRequest(w, "INVALID_EXPIRY", "Card is expired", correlationID)
		return
	}

	if len(req.CVV) < 3 || len(req.CVV) > 4 {
		server.BadRequest(w, "INVALID_CVV", "CVV must be 3-4 digits", correlationID)
		return
	}

	tenant := auth.GetTenant(ctx)
	if tenant == nil {
		server.BadRequest(w, "MISSING_TENANT", "Tenant context required", correlationID)
		return
	}
	tenantID := tenant.TenantID
	kmsKeyARN := tenant.KMSKeyARN

	blindIndex := crypto.ComputeBlindIndex(tenantID, req.PAN, h.hmacKey)

	existing, err := h.tokenRepo.FindByBlindIndex(ctx, tenantID, blindIndex)
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}

	if existing != nil {
		h.tokenRepo.UpdateExpiry(ctx, tenantID, existing.TokenID, req.ExpiryMonth, req.ExpiryYear)
		cvvStored := h.storeCVV(ctx, tenantID, existing.TokenID, req.CVV)

		h.logAudit(ctx, tenantID, correlationID, existing.TokenID, model.AuditResultSuccess)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(TokenizeResponse{
			Token:      existing.TokenID,
			CVVStored:  cvvStored,
			IsExisting: true,
		})
		return
	}

	tokenID, err := generateTokenID()
	if err != nil {
		server.InternalError(w, correlationID)
		return
	}

	ciphertext, iv, encryptedDEK, err := h.encryptPAN(ctx, req.PAN, kmsKeyARN)
	if err != nil {
		server.ServiceUnavailable(w, "Encryption service unavailable", correlationID)
		return
	}

	token := &model.Token{
		TenantID:      tenantID,
		TokenID:       tokenID,
		PANBlindIndex: blindIndex,
		Status:        model.TokenStatusActive,
		ExpiryMonth:   req.ExpiryMonth,
		ExpiryYear:    req.ExpiryYear,
	}

	if err := h.tokenRepo.Create(ctx, token); err != nil {
		server.InternalError(w, correlationID)
		return
	}

	tagSize := 16
	authTag := ciphertext[len(ciphertext)-tagSize:]
	panCiphertext := ciphertext[:len(ciphertext)-tagSize]

	vaultEntry := &model.VaultEntry{
		TenantID:      tenantID,
		TokenID:       tokenID,
		PANCiphertext: panCiphertext,
		IV:            iv,
		AuthTag:       authTag,
		DEKEncrypted:  encryptedDEK,
		KMSKeyID:      kmsKeyARN,
	}

	if err := h.vaultRepo.Create(ctx, vaultEntry); err != nil {
		server.InternalError(w, correlationID)
		return
	}

	cvvStored := h.storeCVV(ctx, tenantID, tokenID, req.CVV)

	h.logAudit(ctx, tenantID, correlationID, tokenID, model.AuditResultSuccess)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(TokenizeResponse{
		Token:      tokenID,
		CVVStored:  cvvStored,
		IsExisting: false,
	})
}

func (h *TokenizeHandler) encryptPAN(ctx context.Context, pan, keyARN string) (ciphertext, iv, encryptedDEK []byte, err error) {
	dekPlain, dekEncrypted, err := h.kmsClient.GenerateDataKeyWithARN(ctx, keyARN)
	if err != nil {
		return nil, nil, nil, err
	}
	defer clearBytes(dekPlain)

	ciphertext, iv, err = crypto.EncryptWithDEK([]byte(pan), dekPlain)
	if err != nil {
		return nil, nil, nil, err
	}
	return ciphertext, iv, dekEncrypted, nil
}

func (h *TokenizeHandler) storeCVV(ctx context.Context, tenantID, tokenID, cvv string) bool {
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return false
	}
	defer clearBytes(dek)

	ciphertext, iv, err := crypto.EncryptWithDEK([]byte(cvv), dek)
	if err != nil {
		return false
	}

	// Combined format: dek(32) + iv(12) + ciphertext_with_tag
	combined := make([]byte, 0, len(dek)+len(iv)+len(ciphertext))
	combined = append(combined, dek...)
	combined = append(combined, iv...)
	combined = append(combined, ciphertext...)
	if err := h.cvvStore.Store(ctx, tenantID, tokenID, combined, h.cvvTTL); err != nil {
		return false
	}
	return true
}

func (h *TokenizeHandler) logAudit(ctx context.Context, tenantID, correlationID, tokenID string, result model.AuditResult) {
	h.audit.Log(ctx, &model.AuditLogEntry{
		TenantID:      tenantID,
		CorrelationID: correlationID,
		Operation:     model.AuditOpTokenize,
		TokenIDMasked: audit.MaskTokenID(tokenID),
		Actor:         "tokenizer",
		Result:        result,
	})
}

// validatePAN checks digit count (13-19) and Luhn checksum.
func validatePAN(pan string) bool {
	n := len(pan)
	if n < 13 || n > 19 {
		return false
	}

	sum := 0
	alt := false
	for i := n - 1; i >= 0; i-- {
		d, err := strconv.Atoi(string(pan[i]))
		if err != nil {
			return false
		}
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

// generateTokenID produces a tok_ prefixed unique identifier.
func generateTokenID() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token id: %w", err)
	}
	return "tok_" + hex.EncodeToString(b), nil
}

func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
