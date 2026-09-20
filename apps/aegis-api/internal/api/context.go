package api

import (
	"context"

	"company.local/aegis/apps/aegis-api/internal/api/requestctx"
	"company.local/aegis/apps/aegis-api/internal/domain"
)

func WithRequestID(ctx context.Context, id string) context.Context {
	return requestctx.WithRequestID(ctx, id)
}
func RequestID(ctx context.Context) string {
	return requestctx.RequestID(ctx)
}
func WithPrincipal(ctx context.Context, p domain.Principal) context.Context {
	return requestctx.WithPrincipal(ctx, p)
}
func Principal(ctx context.Context) (domain.Principal, bool) {
	return requestctx.Principal(ctx)
}
