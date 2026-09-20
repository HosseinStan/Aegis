package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"company.local/aegis/apps/aegis-api/internal/api/requestctx"
	"company.local/aegis/apps/aegis-api/internal/auth"
)

func Authenticate(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")) == "" {
				writeMiddlewareError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.", requestctx.RequestID(r.Context()))
				return
			}
			principal, err := service.Authenticate(r.Context(), strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
			if err != nil {
				writeMiddlewareError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.", requestctx.RequestID(r.Context()))
				return
			}
			next.ServeHTTP(w, r.WithContext(requestctx.WithPrincipal(r.Context(), principal)))
		})
	}
}

func AuthenticateDocs(service *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("aegis_docs_token")
			if err != nil || cookie.Value == "" {
				writeMiddlewareError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.", requestctx.RequestID(r.Context()))
				return
			}
			principal, err := service.Authenticate(r.Context(), cookie.Value)
			if err != nil {
				writeMiddlewareError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.", requestctx.RequestID(r.Context()))
				return
			}
			next.ServeHTTP(w, r.WithContext(requestctx.WithPrincipal(r.Context(), principal)))
		})
	}
}

func Require(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := requestctx.Principal(r.Context())
			if !ok {
				writeMiddlewareError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required.", requestctx.RequestID(r.Context()))
				return
			}
			if !principal.HasPermission(permission) {
				writeMiddlewareError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to perform this action.", requestctx.RequestID(r.Context()))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeMiddlewareError(w http.ResponseWriter, status int, code, message, requestID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":%q,"request_id":%q}}`, code, message, requestID)
}
