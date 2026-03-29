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

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		server.BadRequest(w, "INVALID_JSON", "Invalid request body", correlationID)
		return
	}

	_, hasPAN := body["pan"]
	_, hasCVV := body["cvv"]
	if !hasPAN && !hasCVV {
		server.BadRequest(w, "NO_SENSITIVE_FIELDS", "Request must contain at least one of: pan, cvv", correlationID)
		return
	}

	tenant := auth.GetTenant(ctx)
	if tenant == nil {
		server.BadRequest(w, "MISSING_TENANT", "Tenant context required", correlationID)
		return
	}
	tenantID := tenant.TenantID
	kmsKeyARN := tenant.KMSKeyARN

	// Track whether a new PAN token was created (for HTTP status code)
	newPANToken := false
	var panTokenID string

	// Process PAN if present
	if hasPAN {
		panStr, ok := body["pan"].(string)
		if !ok {
			server.BadRequest(w, "INVALID_PAN", "PAN must be a string", correlationID)
			return
		}
		if !validatePAN(panStr) {
			server.BadRequest(w, "INVALID_PAN", "PAN failed Luhn validation", correlationID)
			return
		}

		blindIndex := crypto.ComputeBlindIndex(tenantID, panStr, h.hmacKey)
		existing, err := h.tokenRepo.FindByBlindIndex(ctx, tenantID, blindIndex)
		if err != nil {
			server.InternalError(w, correlationID)
			return
		}

		if existing != nil {
			panTokenID = existing.TokenID
		} else {
			tokenID, err := generateTokenID()
			if err != nil {
				server.InternalError(w, correlationID)
				return
			}

			ciphertext, iv, encryptedDEK, err := h.encryptPAN(ctx, panStr, kmsKeyARN)
			if err != nil {
				server.ServiceUnavailable(w, "Encryption service unavailable", correlationID)
				return
			}

			token := &model.Token{
				TenantID:      tenantID,
				TokenID:       tokenID,
				PANBlindIndex: blindIndex,
				Status:        model.TokenStatusActive,
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

			panTokenID = tokenID
			newPANToken = true
		}

		// Replace pan value with token in the response body
		body["pan"] = panTokenID
		h.logAudit(ctx, tenantID, correlationID, panTokenID, model.AuditResultSuccess)
	}

	// Process CVV if present
	if hasCVV {
		cvvStr, ok := body["cvv"].(string)
		if !ok {
			server.BadRequest(w, "INVALID_CVV", "CVV must be a string", correlationID)
			return
		}
		if !validateCVV(cvvStr) {
			server.BadRequest(w, "INVALID_CVV", "CVV must be 3-4 digits", correlationID)
			return
		}

		cvvTokenID, err := generateTokenID()
		if err != nil {
			server.InternalError(w, correlationID)
			return
		}

		if ok := h.storeCVVToken(ctx, tenantID, cvvTokenID, cvvStr); !ok {
			server.InternalError(w, correlationID)
			return
		}

		// Invalidate previous CVV token if we have a PAN token association
		if panTokenID != "" {
			h.cvvStore.InvalidatePreviousCVVToken(ctx, tenantID, panTokenID, cvvTokenID, h.cvvTTL)
		}

		// Replace cvv value with token in the response body
		body["cvv"] = cvvTokenID
		h.logAudit(ctx, tenantID, correlationID, cvvTokenID, model.AuditResultSuccess)
	}

	// Echo response — all non-sensitive fields pass through unchanged
	w.Header().Set("Content-Type", "application/json")
	if newPANToken {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(body)
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

func (h *TokenizeHandler) storeCVVToken(ctx context.Context, tenantID, cvvTokenID, cvv string) bool {
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return false
	}
	defer clearBytes(dek)

	ciphertext, iv, err := crypto.EncryptWithDEK([]byte(cvv), dek)
	if err != nil {
		return false
	}

	combined := make([]byte, 0, len(dek)+len(iv)+len(ciphertext))
	combined = append(combined, dek...)
	combined = append(combined, iv...)
	combined = append(combined, ciphertext...)
	if err := h.cvvStore.StoreCVVToken(ctx, tenantID, cvvTokenID, combined, h.cvvTTL); err != nil {
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

func validateCVV(cvv string) bool {
	n := len(cvv)
	if n < 3 || n > 4 {
		return false
	}
	for _, c := range cvv {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

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
