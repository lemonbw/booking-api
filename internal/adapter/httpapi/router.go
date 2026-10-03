package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
)

func parseInt64(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}

type HealthChecker interface {
	Ping(ctx context.Context) error
}

func NewRouter(h *Handler, health HealthChecker, auth Authenticator, log *slog.Logger, metrics Metrics) http.Handler {
	mux := http.NewServeMux()

	api := http.NewServeMux()
	api.HandleFunc("POST /bookings", h.CreateBooking)
	api.HandleFunc("POST /bookings/series", h.CreateBookingSeries)
	api.HandleFunc("GET /bookings", h.ListRoomBookings)
	api.HandleFunc("GET /bookings/{id}", h.GetBooking)
	api.HandleFunc("POST /bookings/{id}/cancel", h.CancelBooking)
	api.HandleFunc("GET /rooms", h.ListRooms)
	api.HandleFunc("GET /rooms/{id}", h.GetRoom)

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", authMiddleware(api, auth, log)))

	if metrics != nil {
		mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(metrics.Render())); err != nil {
				log.Warn("failed to render metrics", "error", err)
			}
		})
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := health.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	return requestIDMiddleware(metricsMiddleware(metrics)(mux))
}
