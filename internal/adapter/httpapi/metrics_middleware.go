package httpapi

import (
	"net/http"
	"strings"
	"time"
)

type Metrics interface {
	ObserveRequest(route, method string, status int, d time.Duration)

	CountBooking(kind, outcome string)

	Render() string
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
