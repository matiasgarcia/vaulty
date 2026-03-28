package auth

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"

	"github.com/pci-vault/vault/internal/server"
)

type contextKey string

const ServiceIdentityKey contextKey = "service_identity"

// BearerAuth extracts and validates a Bearer token from the Authorization header.
// In production, this would validate against a token store or JWT issuer.
// For now, the token format is "service_name:secret" and we extract service_name.
func BearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			correlationID := server.GetCorrelationID(r.Context())
			server.Unauthorized(w, correlationID)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		// Extract service identity from token (format: "service_name:secret")
		parts := strings.SplitN(token, ":", 2)
		identity := parts[0]

		ctx := context.WithValue(r.Context(), ServiceIdentityKey, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetServiceIdentity returns the authenticated service identity from context.
func GetServiceIdentity(ctx context.Context) string {
	if id, ok := ctx.Value(ServiceIdentityKey).(string); ok {
		return id
	}
	return ""
}

// RequireMTLSCert validates the client certificate CN matches the expected identity.
// Used for /internal/detokenize — only the Proxy service cert is accepted.
func RequireMTLSCert(expectedCN string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
				// In dev without TLS, fall back to Bearer auth identity check
				identity := GetServiceIdentity(r.Context())
				if identity == expectedCN {
					next.ServeHTTP(w, r)
					return
				}
				correlationID := server.GetCorrelationID(r.Context())
				server.Forbidden(w, correlationID)
				return
			}

			cn := r.TLS.PeerCertificates[0].Subject.CommonName
			if cn != expectedCN {
				correlationID := server.GetCorrelationID(r.Context())
				server.Forbidden(w, correlationID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TLSConfigWithClientAuth returns a TLS config that requires client certificates.
func TLSConfigWithClientAuth() *tls.Config {
	return &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS12,
	}
}
