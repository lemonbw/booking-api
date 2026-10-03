package httpapi

import (
	"context"

	"github.com/lemonbw/booking-api/internal/usecase"
)

type contextKey string

const principalContextKey contextKey = "principal"

func PrincipalFromContext(ctx context.Context) usecase.Principal {
	principal, ok := ctx.Value(principalContextKey).(usecase.Principal)
	if !ok {
		return usecase.Anonymous
	}
	return principal
}

func withPrincipal(ctx context.Context, p usecase.Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, p)
}
