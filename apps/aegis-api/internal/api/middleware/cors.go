package middleware

import (
	"net/http"
	"strings"

	"company.local/aegis/apps/aegis-api/internal/api/requestctx"
)

func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				if origin != "" {
					if _, ok := allowed[origin]; !ok {
						http.Error(w, "origin not allowed", http.StatusForbidden)
						return
					}
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if strings.Contains(r.Header.Get("Content-Type"), "text/plain") && r.Method != http.MethodGet {
				if riskPostRoute(r) {
					writeMiddlewareError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Unsupported request content type.", requestctx.RequestID(r.Context()))
					return
				}
				http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Match only the two POST route shapes, before chi has populated URL params.
// Use the same raw-path preference as the router; never clean paths or use a
// substring/prefix that could change unrelated endpoints' existing 415 body.
func riskPostRoute(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	path := r.URL.RawPath
	if path == "" {
		path = r.URL.Path
	}
	parts := strings.Split(path, "/")
	if len(parts) != 8 && len(parts) != 9 {
		return false
	}
	return parts[0] == "" && parts[1] == "api" && parts[2] == "v1" &&
		parts[3] == "devices" && parts[4] != "" && parts[5] == "change-events" &&
		parts[6] != "" && parts[7] == "risk-assessment" &&
		(len(parts) == 8 || parts[8] == "preview")
}
