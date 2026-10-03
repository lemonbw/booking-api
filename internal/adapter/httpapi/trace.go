package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

const traceparentHeader = "traceparent"

func traceMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if traceparent := strings.TrimSpace(r.Header.Get(traceparentHeader)); validTraceparent(traceparent) {
			r = r.WithContext(withTraceparent(r.Context(), traceparent))
		}
		next.ServeHTTP(w, r)
	})
}

type traceparentContextKey struct{}

func withTraceparent(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, traceparentContextKey{}, value)
}

func TraceparentFromContext(ctx context.Context) string {
	value, _ := ctx.Value(traceparentContextKey{}).(string)
	return value
}

func validTraceparent(v string) bool {
	if len(v) != 55 || v[2] != '-' || v[35] != '-' || v[52] != '-' {
		return false
	}
	for i, c := range v {
		switch i {
		case 2, 35, 52:
			continue
		}
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			return false
		}
	}
	return true
}
