package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

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

func authMiddleware(next http.Handler, auth Authenticator, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := auth.Authenticate(r)
		if err != nil {
			if !errors.Is(err, ErrUnauthenticated) {
				log.WarnContext(r.Context(), "unexpected authentication error", "error", err)
			}
			writeError(w, http.StatusUnauthorized, "unauthenticated", "", "a valid credential is required")
			return
		}
		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
	})
}
