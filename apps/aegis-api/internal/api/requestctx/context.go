package requestctx

import (
	"context"

	"company.local/aegis/apps/aegis-api/internal/domain"
)

type key string

const (
	requestIDKey key = "request_id"
	principalKey key = "principal"
)

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}
func WithPrincipal(ctx context.Context, p domain.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}
func Principal(ctx context.Context) (domain.Principal, bool) {
	value, ok := ctx.Value(principalKey).(domain.Principal)
	return value, ok
}
