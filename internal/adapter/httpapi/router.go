package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

func parseInt64(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}

type HealthChecker interface {
	Ping(ctx context.Context) error
}

func NewRouter(h *Handler, health HealthChecker, auth Authenticator, log *slog.Logger, metrics Metrics) http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("POST /bookings", h.CreateBooking)
	api.HandleFunc("POST /bookings/series", h.CreateBookingSeries)
	api.HandleFunc("GET /bookings", h.ListRoomBookings)
	api.HandleFunc("GET /bookings/{id}", h.GetBooking)
	api.HandleFunc("POST /bookings/{id}/cancel", h.CancelBooking)
	api.HandleFunc("GET /rooms", h.ListRooms)
	api.HandleFunc("GET /rooms/{id}", h.GetRoom)

	routes := http.NewServeMux()
	routes.Handle("/api/v1/", http.StripPrefix("/api/v1", authMiddleware(jsonFallback(api), auth, log)))

	if metrics != nil {
		routes.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(metrics.Render())); err != nil {
				log.Warn("failed to render metrics", "error", err)
			}
		})
	}

	routes.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	routes.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := health.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux := http.NewServeMux()
	mux.Handle("/", jsonFallback(routes))

	return requestIDMiddleware(traceMiddleware(accessLogMiddleware(metricsMiddleware(metrics)(mux), log), log))
}

var probeMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
}

func jsonFallback(routes *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := routes.Handler(r); pattern != "" {
			routes.ServeHTTP(w, r)
			return
		}
		if allowed := allowedMethods(routes, r); allowed != "" {
			w.Header().Set("Allow", allowed)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "", "the method is not supported for this resource")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "", "the requested resource does not exist")
	})
}

func allowedMethods(routes *http.ServeMux, r *http.Request) string {
	var found []string
	for _, method := range probeMethods {
		probe := r.Clone(r.Context())
		probe.Method = method
		if _, pattern := routes.Handler(probe); pattern != "" {
			found = append(found, method)
		}
	}
	return strings.Join(found, ", ")
}
