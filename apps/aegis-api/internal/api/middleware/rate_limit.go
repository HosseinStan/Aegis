package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"company.local/aegis/apps/aegis-api/internal/api/requestctx"
	"company.local/aegis/apps/aegis-api/internal/security"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter *rate.Limiter
	seen    time.Time
}
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]visitor
	rate     rate.Limit
	burst    int
	trusted  []*net.IPNet
}

func NewRateLimiter(requestsPerSecond float64, burst int, trustedCIDRs ...string) *RateLimiter {
	trusted, _ := security.ParseCIDRs(trustedCIDRs)
	return &RateLimiter{visitors: make(map[string]visitor), rate: rate.Limit(requestsPerSecond), burst: burst, trusted: trusted}
}

func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := security.ClientIP(r, l.trusted)
		now := time.Now()
		l.mu.Lock()
		item, ok := l.visitors[host]
		if !ok {
			item.limiter = rate.NewLimiter(l.rate, l.burst)
		}
		item.seen = now
		allowed := item.limiter.Allow()
		l.visitors[host] = item
		if len(l.visitors) > 10000 {
			for key, value := range l.visitors {
				if now.Sub(value.seen) > 10*time.Minute {
					delete(l.visitors, key)
				}
			}
		}
		l.mu.Unlock()
		if !allowed {
			w.Header().Set("Retry-After", "1")
			writeMiddlewareError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again later.", requestctx.RequestID(r.Context()))
			return
		}
		next.ServeHTTP(w, r)
	})
}
