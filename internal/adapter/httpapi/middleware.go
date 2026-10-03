package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lemonbw/booking-api/internal/usecase"
)

type Metrics interface {
	ObserveRequest(route, method string, status int, d time.Duration)

	CountBooking(kind, outcome string)

	Render() string
}

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

const requestIDHeader = "X-Request-Id"

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if id == "" || len(id) > 128 || !isPrintableASCII(id) {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r)
	})
}
func isPrintableASCII(s string) bool {
	for _, c := range s {
		if c < 0x21 || c > 0x7e {
			return false
		}
	}
	return len(s) > 0
}
func newRequestID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

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

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func statusOf(rec *statusRecorder) int {
	if rec.status == 0 {
		return http.StatusOK
	}
	return rec.status
}

const routeOther = "other"

func routePatternFor(path string) string {
	switch {
	case path == "/healthz", path == "/readyz", path == "/metrics":
		return path
	case path == "/api/v1/bookings", path == "/api/v1/bookings/series", path == "/api/v1/rooms":
		return path
	case strings.HasPrefix(path, "/api/v1/bookings/"):
		return "/api/v1/bookings/{id}"
	case strings.HasPrefix(path, "/api/v1/rooms/"):
		return "/api/v1/rooms/{roomID}"
	default:
		return routeOther
	}
}

func metricsMiddleware(registry Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if registry == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			registry.ObserveRequest(routePatternFor(r.URL.Path), r.Method, statusOf(rec), time.Since(start))
		})
	}
}
