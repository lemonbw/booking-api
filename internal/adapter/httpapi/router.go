package httpapi

import (
	"context"
	"net/http"
	"strconv"
)

func parseInt64(raw string) (int64, error) {
	return strconv.ParseInt(raw, 10, 64)
}

type HealthChecker interface {
	Ping(ctx context.Context) error
}

func NewRouter(h *Handler, health HealthChecker) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /bookings", h.CreateBooking)
	mux.HandleFunc("GET /bookings", h.ListRoomBookings)
	mux.HandleFunc("GET /bookings/{id}", h.GetBooking)
	mux.HandleFunc("POST /bookings/{id}/cancel", h.CancelBooking)
	mux.HandleFunc("GET /rooms", h.ListRooms)
	mux.HandleFunc("GET /rooms/{id}", h.GetRoom)

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

	return mux
}
