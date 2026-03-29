package auth

import (
	"net/http"

	"github.com/pci-vault/vault/internal/server"
)

// Role represents a service role for RBAC.
type Role string

const (
	RoleTokenize   Role = "tokenize"
	RoleForward    Role = "forward"
	RoleManage     Role = "manage"
	RoleDetokenize Role = "detokenize"
	RoleAdmin      Role = "admin"
)

// Service-to-role mapping. In production, this would come from a
// configuration store or identity provider.
var serviceRoles = map[string][]Role{
	"tokenizer": {RoleTokenize, RoleManage, RoleDetokenize},
	"proxy":     {RoleForward, RoleDetokenize},
	"admin":     {RoleTokenize, RoleManage, RoleAdmin},
	"client":    {RoleTokenize, RoleForward},
}

// RequireRole returns middleware that checks the authenticated service
// has the required role. Must be used after BearerAuth middleware.
func RequireRole(role Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			identity := GetServiceIdentity(req.Context())
			if identity == "" {
				correlationID := server.GetCorrelationID(req.Context())
				server.Unauthorized(w, correlationID)
				return
			}

			roles, ok := serviceRoles[identity]
			if !ok {
				correlationID := server.GetCorrelationID(req.Context())
				server.Forbidden(w, correlationID)
				return
			}

			for _, r := range roles {
				if r == role {
					next.ServeHTTP(w, req)
					return
				}
			}

			correlationID := server.GetCorrelationID(req.Context())
			server.Forbidden(w, correlationID)
		})
	}
}
