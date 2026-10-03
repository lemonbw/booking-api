package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
	"github.com/lemonbw/booking-api/internal/usecase"
)

var testNow = time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC)

type cancelCall struct {
	id domain.BookingID
	by usecase.Principal
}

type stubUseCase struct {
	created    []usecase.CreateBookingCommand
	series     []usecase.CreateSeriesCommand
	queries    []usecase.ListBookingsQuery
	bookingIDs []domain.BookingID
	roomIDs    []domain.RoomID
	cancels    []cancelCall

	booking        domain.Booking
	bookingErr     error
	seriesBookings []domain.Booking
	seriesErr      error
	bookings       []domain.Booking
	listErr        error
	cancelBooking  domain.Booking
	cancelErr      error
	rooms          []domain.Room
	roomsErr       error
	room           domain.Room
	roomErr        error
}

func (s *stubUseCase) CreateBooking(_ context.Context, cmd usecase.CreateBookingCommand) (domain.Booking, error) {
	s.created = append(s.created, cmd)
	return s.booking, s.bookingErr
}

func (s *stubUseCase) CreateBookingSeries(_ context.Context, cmd usecase.CreateSeriesCommand) ([]domain.Booking, error) {
	s.series = append(s.series, cmd)
	return s.seriesBookings, s.seriesErr
}

func (s *stubUseCase) ListRoomBookings(_ context.Context, q usecase.ListBookingsQuery) ([]domain.Booking, error) {
	s.queries = append(s.queries, q)
	return s.bookings, s.listErr
}

func (s *stubUseCase) GetBooking(_ context.Context, id domain.BookingID) (domain.Booking, error) {
	s.bookingIDs = append(s.bookingIDs, id)
	return s.booking, s.bookingErr
}

func (s *stubUseCase) CancelBooking(_ context.Context, id domain.BookingID, by usecase.Principal) (domain.Booking, error) {
	s.cancels = append(s.cancels, cancelCall{id: id, by: by})
	return s.cancelBooking, s.cancelErr
}

func (s *stubUseCase) ListRooms(context.Context) ([]domain.Room, error) {
	return s.rooms, s.roomsErr
}

func (s *stubUseCase) GetRoom(_ context.Context, id domain.RoomID) (domain.Room, error) {
	s.roomIDs = append(s.roomIDs, id)
	return s.room, s.roomErr
}

type stubHealth struct {
	err error
}

func (s stubHealth) Ping(context.Context) error { return s.err }

type recordedRequest struct {
	route  string
	method string
	status int
}

type recordingMetrics struct {
	requests []recordedRequest
	outcomes []string
	rendered string
}

func (m *recordingMetrics) ObserveRequest(route, method string, status int, _ time.Duration) {
	m.requests = append(m.requests, recordedRequest{route: route, method: method, status: status})
}

func (m *recordingMetrics) CountBooking(kind, outcome string) {
	m.outcomes = append(m.outcomes, kind+"/"+outcome)
}

func (m *recordingMetrics) Render() string { return m.rendered }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTextLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func newTestHandler(svc usecase.BookingUseCase) *Handler {
	return NewHandler(svc, discardLogger(), func() time.Time { return testNow })
}

func fixtureBooking() domain.Booking {
	start := time.Date(2026, time.March, 3, 9, 0, 0, 0, time.UTC)
	return domain.Booking{
		ID:        "b-1",
		SeriesID:  "s-1",
		RoomID:    7,
		UserName:  "ada",
		StartTime: start,
		EndTime:   start.Add(time.Hour),
		Status:    domain.BookingStatusActive,
		CreatedAt: testNow,
		UpdatedAt: testNow,
	}
}

func newTestMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /bookings", h.CreateBooking)
	mux.HandleFunc("POST /bookings/series", h.CreateBookingSeries)
	mux.HandleFunc("GET /bookings", h.ListRoomBookings)
	mux.HandleFunc("GET /bookings/{id}", h.GetBooking)
	mux.HandleFunc("POST /bookings/{id}/cancel", h.CancelBooking)
	mux.HandleFunc("GET /rooms", h.ListRooms)
	mux.HandleFunc("GET /rooms/{id}", h.GetRoom)
	return mux
}

func serve(t *testing.T, h *Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	newTestMux(h).ServeHTTP(rec, httptest.NewRequest(method, target, reader))
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()

	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("body %q is not valid json: %v", rec.Body.String(), err)
	}
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorResponse {
	t.Helper()

	var payload errorResponse
	decodeBody(t, rec, &payload)
	return payload
}
