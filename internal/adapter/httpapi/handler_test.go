package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
	"github.com/lemonbw/booking-api/internal/usecase"
)

const createBookingBody = `{"room_id":7,"user_name":"ada","start_time":"2026-03-03T09:00:00Z","end_time":"2026-03-03T10:00:00Z"}`

func TestCreateBookingReturnsTheStoredBooking(t *testing.T) {
	svc := &stubUseCase{booking: fixtureBooking()}
	rec := serve(t, newTestHandler(svc), http.MethodPost, "/bookings", createBookingBody)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want json", got)
	}

	var got bookingResponse
	decodeBody(t, rec, &got)
	if got.ID != "b-1" || got.RoomID != 7 || got.UserName != "ada" || got.Status != "active" {
		t.Errorf("body = %+v, want the stored booking", got)
	}
	if !got.StartTime.Equal(time.Date(2026, time.March, 3, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("start_time = %s, want the stored start", got.StartTime)
	}
	if got.CancelledAt != nil {
		t.Errorf("cancelled_at = %s, want null for an active booking", got.CancelledAt)
	}
}

func TestCreateBookingForwardsTheAuthenticatedPrincipal(t *testing.T) {
	svc := &stubUseCase{booking: fixtureBooking()}
	h := newTestHandler(svc)

	principal := usecase.Principal{Subject: "u-9", DisplayName: "Ada", Authenticated: true}
	request := httptest.NewRequest(http.MethodPost, "/bookings", strings.NewReader(createBookingBody))
	rec := httptest.NewRecorder()
	h.CreateBooking(rec, request.WithContext(withPrincipal(request.Context(), principal)))

	if len(svc.created) != 1 {
		t.Fatalf("CreateBooking calls = %d, want 1", len(svc.created))
	}
	if got := svc.created[0].By; got != principal {
		t.Errorf("By = %+v, want %+v", got, principal)
	}
}

func TestCreateBookingIsAnonymousWithoutCredentials(t *testing.T) {
	svc := &stubUseCase{booking: fixtureBooking()}
	rec := serve(t, newTestHandler(svc), http.MethodPost, "/bookings", createBookingBody)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}
	if got := svc.created[0].By; got != usecase.Anonymous {
		t.Errorf("By = %+v, want the anonymous principal", got)
	}
}

func TestCreateBookingRejectsABrokenBody(t *testing.T) {
	svc := &stubUseCase{booking: fixtureBooking()}
	rec := serve(t, newTestHandler(svc), http.MethodPost, "/bookings", `{"room_id":`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if got := decodeError(t, rec); got.Error != "invalid_json" || got.Field != "body" {
		t.Errorf("body = %+v, want invalid_json on the body field", got)
	}
	if len(svc.created) != 0 {
		t.Errorf("CreateBooking calls = %d, want 0 for a broken body", len(svc.created))
	}
}

func TestCreateBookingSeriesSummarisesTheOccurrences(t *testing.T) {
	first := fixtureBooking()
	second := fixtureBooking()
	second.ID = "b-2"
	second.StartTime = first.StartTime.Add(24 * time.Hour)
	second.EndTime = first.EndTime.Add(24 * time.Hour)

	svc := &stubUseCase{seriesBookings: []domain.Booking{first, second}}
	body := `{"room_id":7,"user_name":"ada","start_time":"2026-03-03T09:00:00Z","end_time":"2026-03-03T10:00:00Z","interval":"24h","count":2}`
	rec := serve(t, newTestHandler(svc), http.MethodPost, "/bookings/series", body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var got bookingSeriesResponse
	decodeBody(t, rec, &got)
	if got.SeriesID != "s-1" || got.Count != 2 || got.RoomID != 7 || got.UserName != "ada" {
		t.Errorf("body = %+v, want the series summary", got)
	}
	if got.Interval != Duration(24*time.Hour) {
		t.Errorf("interval = %s, want 24h", time.Duration(got.Interval))
	}
	if !got.FirstStart.Equal(first.StartTime) || !got.LastEnd.Equal(second.EndTime) {
		t.Errorf("first_start = %s, last_end = %s, want the outer bounds of the run", got.FirstStart, got.LastEnd)
	}
	if len(got.Bookings) != 2 {
		t.Fatalf("bookings = %d, want 2", len(got.Bookings))
	}
	if len(svc.series) != 1 {
		t.Fatalf("CreateBookingSeries calls = %d, want 1", len(svc.series))
	}
	if got := svc.series[0]; got.Interval != 24*time.Hour || got.Count != 2 {
		t.Errorf("command = %+v, want the interval and count from the body", got)
	}
}

func TestCreateBookingSeriesFailsWhenNoOccurrenceComesBack(t *testing.T) {
	svc := &stubUseCase{seriesBookings: []domain.Booking{}}
	body := `{"room_id":7,"user_name":"ada","start_time":"2026-03-03T09:00:00Z","end_time":"2026-03-03T10:00:00Z","interval":"24h","count":2}`
	rec := serve(t, newTestHandler(svc), http.MethodPost, "/bookings/series", body)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusInternalServerError, rec.Body)
	}
	if got := decodeError(t, rec); got.Error != "internal_error" || got.Message != "internal error" {
		t.Errorf("body = %+v, want a bare internal_error", got)
	}
}

func TestCreateBookingSeriesRejectsAnUnusableInterval(t *testing.T) {
	svc := &stubUseCase{}
	body := `{"room_id":7,"user_name":"ada","start_time":"2026-03-03T09:00:00Z","end_time":"2026-03-03T10:00:00Z","interval":3600,"count":2}`
	rec := serve(t, newTestHandler(svc), http.MethodPost, "/bookings/series", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if got := decodeError(t, rec); got.Error != "invalid_json" {
		t.Errorf("body = %+v, want invalid_json for a numeric interval", got)
	}
	if len(svc.series) != 0 {
		t.Errorf("CreateBookingSeries calls = %d, want 0", len(svc.series))
	}
}

func TestListRoomBookingsDefaultsToTheCurrentDay(t *testing.T) {
	svc := &stubUseCase{}
	rec := serve(t, newTestHandler(svc), http.MethodGet, "/bookings?room_id=7", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}
	if len(svc.queries) != 1 {
		t.Fatalf("ListRoomBookings calls = %d, want 1", len(svc.queries))
	}
	got := svc.queries[0]
	if got.RoomID != 7 {
		t.Errorf("RoomID = %d, want 7", got.RoomID)
	}
	if !got.Day.Equal(testNow) {
		t.Errorf("Day = %s, want the injected clock %s", got.Day, testNow)
	}
	if got.Limit != 0 || got.Offset != 0 {
		t.Errorf("Limit = %d, Offset = %d, want both unset", got.Limit, got.Offset)
	}
	if got := rec.Body.String(); got != "[]\n" {
		t.Errorf("body = %q, want an empty json array", got)
	}
}

func TestListRoomBookingsParsesTheQuery(t *testing.T) {
	svc := &stubUseCase{bookings: []domain.Booking{fixtureBooking()}}
	rec := serve(t, newTestHandler(svc), http.MethodGet, "/bookings?room_id=7&date=2026-03-03&limit=25&offset=50", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}
	if len(svc.queries) != 1 {
		t.Fatalf("ListRoomBookings calls = %d, want 1", len(svc.queries))
	}
	got := svc.queries[0]
	if want := time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC); !got.Day.Equal(want) {
		t.Errorf("Day = %s, want %s", got.Day, want)
	}
	if got.Limit != 25 || got.Offset != 50 {
		t.Errorf("Limit = %d, Offset = %d, want 25 and 50", got.Limit, got.Offset)
	}

	var listed []bookingResponse
	decodeBody(t, rec, &listed)
	if len(listed) != 1 || listed[0].ID != "b-1" {
		t.Errorf("body = %+v, want the stored booking", listed)
	}
}

func TestListRoomBookingsRejectsABadQuery(t *testing.T) {
	cases := map[string]struct {
		target string
		field  string
	}{
		"missing room":  {target: "/bookings", field: "room_id"},
		"zero room":     {target: "/bookings?room_id=0", field: "room_id"},
		"negative room": {target: "/bookings?room_id=-3", field: "room_id"},
		"word room":     {target: "/bookings?room_id=seven", field: "room_id"},
		"dotted date":   {target: "/bookings?room_id=7&date=03.03.2026", field: "date"},
		"negative size": {target: "/bookings?room_id=7&limit=-1", field: "limit"},
		"negative page": {target: "/bookings?room_id=7&offset=-1", field: "offset"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &stubUseCase{}
			rec := serve(t, newTestHandler(svc), http.MethodGet, tc.target, "")

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
			if got := decodeError(t, rec); got.Error != "validation_failed" || got.Field != tc.field {
				t.Errorf("body = %+v, want validation_failed on %s", got, tc.field)
			}
			if len(svc.queries) != 0 {
				t.Errorf("ListRoomBookings calls = %d, want 0 for a rejected query", len(svc.queries))
			}
		})
	}
}

func TestGetBookingUsesThePathValue(t *testing.T) {
	svc := &stubUseCase{booking: fixtureBooking()}
	rec := serve(t, newTestHandler(svc), http.MethodGet, "/bookings/b-1", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}
	if len(svc.bookingIDs) != 1 || svc.bookingIDs[0] != "b-1" {
		t.Errorf("ids = %v, want [b-1]", svc.bookingIDs)
	}

	var got bookingResponse
	decodeBody(t, rec, &got)
	if got.ID != "b-1" {
		t.Errorf("id = %q, want b-1", got.ID)
	}
}

func TestCancelBookingForwardsThePathValueAndPrincipal(t *testing.T) {
	cancelled := fixtureBooking()
	cancelled.Status = domain.BookingStatusCancelled
	cancelledAt := testNow.Add(time.Minute)
	cancelled.CancelledAt = &cancelledAt

	svc := &stubUseCase{cancelBooking: cancelled}
	h := newTestHandler(svc)
	request := httptest.NewRequest(http.MethodPost, "/bookings/b-1/cancel", nil)
	request.SetPathValue("id", "b-1")
	principal := usecase.Principal{Subject: "u-9", Authenticated: true}
	rec := httptest.NewRecorder()
	h.CancelBooking(rec, request.WithContext(withPrincipal(request.Context(), principal)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}
	if len(svc.cancels) != 1 {
		t.Fatalf("CancelBooking calls = %d, want 1", len(svc.cancels))
	}
	if got := svc.cancels[0]; got.id != "b-1" || got.by != principal {
		t.Errorf("call = %+v, want b-1 for %+v", got, principal)
	}

	var got bookingResponse
	decodeBody(t, rec, &got)
	if got.Status != "cancelled" || got.CancelledAt == nil {
		t.Errorf("body = %+v, want the cancelled booking", got)
	}
}

func TestGetRoomRejectsAnUnusablePathValue(t *testing.T) {
	cases := map[string]string{
		"word":     "/rooms/seven",
		"zero":     "/rooms/0",
		"negative": "/rooms/-2",
	}

	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &stubUseCase{}
			h := newTestHandler(svc)
			request := httptest.NewRequest(http.MethodGet, target, nil)
			request.SetPathValue("id", strings.TrimPrefix(target, "/rooms/"))
			rec := httptest.NewRecorder()
			h.GetRoom(rec, request)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
			if got := decodeError(t, rec); got.Field != "room_id" {
				t.Errorf("body = %+v, want a field error on room_id", got)
			}
			if len(svc.roomIDs) != 0 {
				t.Errorf("GetRoom calls = %d, want 0", len(svc.roomIDs))
			}
		})
	}
}

func TestListRoomsRendersEveryRoom(t *testing.T) {
	svc := &stubUseCase{rooms: []domain.Room{{ID: 7, Name: "Red"}}}
	rec := serve(t, newTestHandler(svc), http.MethodGet, "/rooms", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}
	var got []roomResponse
	decodeBody(t, rec, &got)
	if len(got) != 1 || got[0].ID != 7 || got[0].Name != "Red" {
		t.Errorf("body = %+v, want the stored room", got)
	}
}

func TestListRoomsReturnsAnEmptyArrayWhenNothingIsStored(t *testing.T) {
	rec := serve(t, newTestHandler(&stubUseCase{}), http.MethodGet, "/rooms", "")

	if got := rec.Body.String(); got != "[]\n" {
		t.Errorf("body = %q, want an empty json array", got)
	}
}

func TestServiceErrorsMapToStatusCodes(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		code   string
		field  string
	}{
		"invalid range":   {err: domain.ErrInvalidTimeRange, status: http.StatusBadRequest, code: "validation_failed"},
		"invalid name":    {err: domain.ErrInvalidUserName, status: http.StatusBadRequest, code: "validation_failed"},
		"invalid room":    {err: domain.ErrInvalidRoomID, status: http.StatusBadRequest, code: "validation_failed"},
		"invalid booking": {err: domain.ErrInvalidBookingID, status: http.StatusBadRequest, code: "validation_failed"},
		"in the past":     {err: domain.ErrBookingInPast, status: http.StatusBadRequest, code: "validation_failed"},
		"overlap":         {err: domain.ErrOverlap, status: http.StatusConflict, code: "booking_conflict", field: "time_range"},
		"already started": {err: domain.ErrBookingAlreadyStarted, status: http.StatusConflict, code: "booking_already_started", field: "start_time"},
		"room deleted":    {err: domain.ErrRoomDeleted, status: http.StatusConflict, code: "room_deleted", field: "room_id"},
		"forbidden":       {err: domain.ErrForbidden, status: http.StatusForbidden, code: "forbidden"},
		"room not found":  {err: domain.ErrRoomNotFound, status: http.StatusNotFound, code: "not_found"},
		"booking missing": {err: domain.ErrBookingNotFound, status: http.StatusNotFound, code: "not_found"},
		"unavailable":     {err: domain.ErrUnavailable, status: http.StatusServiceUnavailable, code: "service_unavailable"},
		"integrity":       {err: domain.ErrIntegrity, status: http.StatusInternalServerError, code: "internal_error"},
		"deadline":        {err: fmt.Errorf("list bookings: %w", context.DeadlineExceeded), status: http.StatusGatewayTimeout, code: "timeout"},
		"wrapped field":   {err: domain.NewFieldError("count", domain.ErrSeriesTooLong), status: http.StatusBadRequest, code: "validation_failed", field: "count"},
		"unknown failure": {err: errors.New("boom"), status: http.StatusInternalServerError, code: "internal_error"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &stubUseCase{bookingErr: tc.err}
			rec := serve(t, newTestHandler(svc), http.MethodPost, "/bookings", createBookingBody)

			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			if got := decodeError(t, rec); got.Error != tc.code || got.Field != tc.field {
				t.Errorf("body = %+v, want code %s on field %q", got, tc.code, tc.field)
			}
		})
	}
}

func TestAnUnknownErrorNeverLeaksItsDetail(t *testing.T) {
	rec := serve(t, newTestHandler(&stubUseCase{bookingErr: errors.New("connection to 10.0.0.5 refused")}),
		http.MethodPost, "/bookings", createBookingBody)

	if got := decodeError(t, rec); strings.Contains(got.Message, "10.0.0.5") {
		t.Errorf("message = %q, want the internal detail hidden", got.Message)
	}
}
