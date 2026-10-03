package httpapi

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

func accessLogMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		attrs := []any{
			"method", r.Method,
			"route", routePatternFor(r.URL.Path),
			"status", statusOf(rec),
			"bytes", rec.bytes,
			"duration_ms", float64(time.Since(start).Microseconds()) / 1000,
			"remote_addr", clientIP(r),
		}
		if id := w.Header().Get(requestIDHeader); id != "" {
			attrs = append(attrs, "request_id", id)
		}
		if traceparent := TraceparentFromContext(r.Context()); traceparent != "" {
			attrs = append(attrs, "trace_id", traceparent[3:35])
		}
		log.InfoContext(r.Context(), "request", attrs...)
	})
}
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if first, _, found := strings.Cut(forwarded, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(forwarded)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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
