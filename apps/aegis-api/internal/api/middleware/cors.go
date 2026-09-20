package middleware

import (
	"net/http"
	"strings"
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
				http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
