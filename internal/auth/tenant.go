package auth

import (
	"context"
	"net/http"
	"regexp"

	"github.com/pci-vault/vault/internal/model"
	"github.com/pci-vault/vault/internal/repository"
	"github.com/pci-vault/vault/internal/server"
)

type tenantContextKey string

const TenantKey tenantContextKey = "tenant"

var tenantIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// RequireTenant extracts X-Tenant-ID from the request header,
// validates format, looks up the tenant, and injects it into context.
// Rejects requests with missing, invalid, or inactive tenants.
func RequireTenant(tenantRepo *repository.TenantRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			correlationID := server.GetCorrelationID(r.Context())

			tenantID := r.Header.Get("X-Tenant-ID")
			if tenantID == "" {
				server.BadRequest(w, "MISSING_TENANT", "X-Tenant-ID header is required", correlationID)
				return
			}

			if !tenantIDPattern.MatchString(tenantID) {
				server.BadRequest(w, "INVALID_TENANT", "Invalid tenant ID format", correlationID)
				return
			}

			tenant, err := tenantRepo.FindByTenantID(r.Context(), tenantID)
			if err != nil {
				server.InternalError(w, correlationID)
				return
			}
			if tenant == nil {
				server.NotFound(w, "TENANT_NOT_FOUND", "Tenant not found", correlationID)
				return
			}
			if tenant.Status != model.TenantStatusActive {
				server.Forbidden(w, correlationID)
				return
			}

			ctx := context.WithValue(r.Context(), TenantKey, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetTenant returns the tenant from request context.
func GetTenant(ctx context.Context) *model.Tenant {
	if t, ok := ctx.Value(TenantKey).(*model.Tenant); ok {
		return t
	}
	return nil
}

// ExtractTenantHeader is a lightweight middleware for services that don't
// have DB access (e.g., Proxy). It extracts X-Tenant-ID from the header
// and injects a minimal Tenant into context without DB validation.
// The downstream service (Tokenizer) validates the tenant.
func ExtractTenantHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := server.GetCorrelationID(r.Context())

		tenantID := r.Header.Get("X-Tenant-ID")
		if tenantID == "" {
			server.BadRequest(w, "MISSING_TENANT", "X-Tenant-ID header is required", correlationID)
			return
		}

		tenant := &model.Tenant{TenantID: tenantID}
		ctx := context.WithValue(r.Context(), TenantKey, tenant)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
