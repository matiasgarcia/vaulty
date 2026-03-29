package model

import (
	"encoding/json"
	"time"
)

type AuditOperation string

const (
	AuditOpTokenize    AuditOperation = "tokenize"
	AuditOpDetokenize  AuditOperation = "detokenize"
	AuditOpDeactivate  AuditOperation = "deactivate"
	AuditOpValidate    AuditOperation = "validate"
	AuditOpForward     AuditOperation = "forward"
)

type AuditResult string

const (
	AuditResultSuccess AuditResult = "success"
	AuditResultDenied  AuditResult = "denied"
	AuditResultError   AuditResult = "error"
)

type AuditLogEntry struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	CorrelationID string          `json:"correlation_id"`
	Operation     AuditOperation  `json:"operation"`
	TokenIDMasked string          `json:"token_id_masked,omitempty"`
	Actor         string          `json:"actor"`
	Result        AuditResult     `json:"result"`
	Detail        json.RawMessage `json:"detail,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}
