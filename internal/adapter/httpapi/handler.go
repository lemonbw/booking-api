package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
	"github.com/lemonbw/booking-api/internal/usecase"
)

type Handler struct {
	svc usecase.BookingUseCase
	log *slog.Logger
}

func NewHandler(svc usecase.BookingUseCase, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}
func (h *Handler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	var req createBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "body", "request body must be valid JSON")
		return
	}
	booking, err := h.svc.CreateBooking(r.Context(), usecase.CreateBookingCommand{
		RoomID:    domain.RoomID(req.RoomID),
		UserName:  req.UserName,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newBookingResponse(booking))
}
func (h *Handler) ListRoomBookings(w http.ResponseWriter, r *http.Request) {
	roomID, err := requiredInt64Query(r, "room_id")
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	day, err := requiredDayQuery(r)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	bookings, err := h.svc.ListRoomBookings(r.Context(), usecase.ListBookingsQuery{
		RoomID: domain.RoomID(roomID),
		Day:    day,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	responses := make([]bookingResponse, 0, len(bookings))
	for _, b := range bookings {
		responses = append(responses, newBookingResponse(b))
	}
	writeJSON(w, http.StatusOK, listBookingsResponse{
		RoomID:   roomID,
		Day:      day.Format(time.DateOnly),
		Bookings: responses,
	})
}
func (h *Handler) GetBooking(w http.ResponseWriter, r *http.Request) {
	booking, err := h.svc.GetBooking(r.Context(), domain.BookingID(r.PathValue("id")))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newBookingResponse(booking))
}
func (h *Handler) CancelBooking(w http.ResponseWriter, r *http.Request) {
	booking, err := h.svc.CancelBooking(r.Context(), domain.BookingID(r.PathValue("id")))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newBookingResponse(booking))
}
func (h *Handler) ListRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.svc.ListRooms(r.Context())
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	responses := make([]roomResponse, 0, len(rooms))
	for _, room := range rooms {
		responses = append(responses, newRoomResponse(room))
	}
	writeJSON(w, http.StatusOK, listRoomsResponse{Rooms: responses})
}
func (h *Handler) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomID, err := parseInt64(r.PathValue("id"))
	if err != nil || roomID <= 0 {
		writeBadPathParam(w, "room_id", "must be a positive integer")
		return
	}
	room, err := h.svc.GetRoom(r.Context(), domain.RoomID(roomID))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newRoomResponse(room))
}
func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var fieldErr *domain.FieldError

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "timeout", "", "the request took too long to complete")

	case errors.Is(err, domain.ErrInvalidTimeRange), errors.Is(err, domain.ErrInvalidUserName),
		errors.Is(err, domain.ErrInvalidRoomID), errors.Is(err, domain.ErrInvalidBookingID),
		errors.Is(err, domain.ErrBookingInPast):
		writeError(w, http.StatusBadRequest, "validation_failed", "", err.Error())

	case errors.Is(err, domain.ErrOverlap):
		writeError(w, http.StatusConflict, "booking_conflict", "time_range", "the room is already booked for the requested interval")

	case errors.Is(err, domain.ErrBookingAlreadyStarted):
		writeError(w, http.StatusConflict, "booking_already_started", "start_time", err.Error())

	case errors.Is(err, domain.ErrRoomNotFound), errors.Is(err, domain.ErrBookingNotFound):
		writeError(w, http.StatusNotFound, "not_found", "", "the requested resource does not exist")

	case errors.Is(err, domain.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "", "the service is temporarily unavailable, retry later")

	case errors.Is(err, domain.ErrIntegrity):
		writeError(w, http.StatusInternalServerError, "internal_error", "", "internal error")

	case errors.As(err, &fieldErr):
		writeError(w, http.StatusBadRequest, "validation_failed", fieldErr.Field, err.Error())

	default:
		h.log.ErrorContext(r.Context(), "unhandled request error", "error", err, "path", r.URL.Path)
		writeError(w, http.StatusInternalServerError, "internal_error", "", "internal error")
	}
}

func requiredInt64Query(r *http.Request, key string) (int64, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, domain.NewFieldError(key, errors.New("query parameter is required"))
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, domain.NewFieldError(key, errors.New("must be a positive integer"))
	}
	return value, nil
}

func requiredDayQuery(r *http.Request) (time.Time, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("day"))
	if raw == "" {
		return time.Time{}, domain.NewFieldError("day", errors.New("query parameter is required"))
	}
	day, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, domain.NewFieldError("day", errors.New("must be formatted as YYYY-MM-DD"))
	}
	return day.UTC(), nil
}

func writeBadPathParam(w http.ResponseWriter, field, message string) {
	writeError(w, http.StatusBadRequest, "validation_failed", field, message)
}
