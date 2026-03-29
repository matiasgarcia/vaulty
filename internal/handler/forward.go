package handler

import (
	"encoding/json"
	"net/http"

	"github.com/pci-vault/vault/internal/auth"
	"github.com/pci-vault/vault/internal/proxy"
	"github.com/pci-vault/vault/internal/server"
)

type ForwardRequest struct {
	Destination string            `json:"destination"`
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Payload     interface{}       `json:"payload"`
}

type ForwardResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       json.RawMessage   `json:"body"`
}

type ForwardHandler struct {
	revealer  *proxy.Revealer
	forwarder *proxy.Forwarder
}

func NewForwardHandler(revealer *proxy.Revealer, forwarder *proxy.Forwarder) *ForwardHandler {
	return &ForwardHandler{
		revealer:  revealer,
		forwarder: forwarder,
	}
}

func (h *ForwardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := server.GetCorrelationID(ctx)

	var req ForwardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.BadRequest(w, "INVALID_JSON", "Invalid request body", correlationID)
		return
	}

	if req.Destination == "" {
		server.BadRequest(w, "MISSING_DESTINATION", "Destination URL is required", correlationID)
		return
	}

	if req.Method == "" {
		req.Method = http.MethodPost
	}

	// Extract tenant for reveal calls
	tenantID := ""
	if tenant := auth.GetTenant(ctx); tenant != nil {
		tenantID = tenant.TenantID
	}

	// Scan payload for tokens and reveal them
	revealed, err := h.revealer.ScanAndReveal(ctx, tenantID, req.Payload)
	if err != nil {
		server.NotFound(w, "DETOKENIZE_FAILED", err.Error(), correlationID)
		return
	}

	// Forward revealed payload to destination
	result, err := h.forwarder.Forward(ctx, req.Destination, req.Method, req.Headers, revealed)
	if err != nil {
		server.BadGateway(w, "Destination provider error: "+err.Error(), correlationID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ForwardResponse{
		StatusCode: result.StatusCode,
		Headers:    result.Headers,
		Body:       result.Body,
	})
}
