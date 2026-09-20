package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"company.local/aegis/apps/aegis-api/internal/api/requestctx"
	"company.local/aegis/apps/aegis-api/internal/observability"
	"github.com/google/uuid"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if _, err := uuid.Parse(id); err != nil {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(requestctx.WithRequestID(r.Context(), id)))
	})
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		policy := "default-src 'none'; frame-ancestors 'none'"
		if strings.HasPrefix(r.URL.Path, "/docs") {
			policy = "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'"
		} else if strings.HasPrefix(r.URL.Path, "/test-console") {
			policy = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
		}
		w.Header().Set("Content-Security-Policy", policy)
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			body := fmt.Sprintf(`{"error":{"code":"COMMAND_TIMEOUT","message":"The request timed out.","request_id":%q}}`, requestctx.RequestID(r.Context()))
			http.TimeoutHandler(next, timeout, body).ServeHTTP(w, r)
		})
	}
}

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("request panic", "request_id", requestctx.RequestID(r.Context()), "panic", recovered, "stack", string(debug.Stack()))
					writeMiddlewareError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "The request could not be completed.", requestctx.RequestID(r.Context()))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func Logging(logger *slog.Logger, metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			metrics.Requests.Add(1)
			next.ServeHTTP(wrapped, r)
			if wrapped.status >= 500 {
				metrics.Failures.Add(1)
			}
			logger.Info("http request", "request_id", requestctx.RequestID(r.Context()), "method", r.Method, "path", r.URL.Path, "status", wrapped.status, "duration_ms", time.Since(started).Milliseconds())
		})
	}
}
