package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/pci-vault/vault/internal/model"
	"github.com/pci-vault/vault/internal/repository"
)

type Logger struct {
	repo *repository.AuditRepo
}

func NewLogger(repo *repository.AuditRepo) *Logger {
	return &Logger{repo: repo}
}

// Log persists an audit entry and emits a structured log line.
// PAN is masked, CVV is never included. TenantID must be set on the entry.
func (l *Logger) Log(ctx context.Context, entry *model.AuditLogEntry) {
	if err := l.repo.Append(ctx, entry); err != nil {
		slog.ErrorContext(ctx, "audit log persist failed",
			"error", err,
			"correlation_id", entry.CorrelationID,
			"operation", entry.Operation,
		)
		return
	}

	slog.InfoContext(ctx, "audit",
		"correlation_id", entry.CorrelationID,
		"operation", entry.Operation,
		"token_id_masked", entry.TokenIDMasked,
		"actor", entry.Actor,
		"result", entry.Result,
	)
}

// MaskTokenID returns a masked version of the token ID for audit logs.
func MaskTokenID(tokenID string) string {
	if len(tokenID) <= 12 {
		return tokenID
	}
	return tokenID[:12] + strings.Repeat("*", len(tokenID)-12)
}

// MaskPAN masks a PAN for logging: first 6 + last 4 visible.
func MaskPAN(pan string) string {
	if len(pan) < 10 {
		return strings.Repeat("*", len(pan))
	}
	return pan[:6] + strings.Repeat("*", len(pan)-10) + pan[len(pan)-4:]
}

// SafeDetail creates a JSON detail payload ensuring no sensitive data leaks.
func SafeDetail(pairs map[string]interface{}) json.RawMessage {
	for key := range pairs {
		lower := strings.ToLower(key)
		if lower == "cvv" || lower == "pan" || lower == "dek" {
			delete(pairs, key)
		}
	}
	b, _ := json.Marshal(pairs)
	return b
}
