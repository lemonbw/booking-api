package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreateBookingRejectsABodyOverTheLimit(t *testing.T) {
	svc := &stubUseCase{booking: fixtureBooking()}
	h := newTestHandlerWithLimit(svc, 128)

	rec := serve(t, h, http.MethodPost, "/bookings", createBookingBody+strings.Repeat(" ", 512))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body)
	}
	if got := decodeError(t, rec); got.Error != "body_too_large" || got.Field != "body" {
		t.Errorf("body = %+v, want body_too_large on the body field", got)
	}
	if len(svc.created) != 0 {
		t.Errorf("CreateBooking calls = %d, want 0 for an oversized body", len(svc.created))
	}
}

func TestCreateBookingAcceptsABodyUnderTheLimit(t *testing.T) {
	svc := &stubUseCase{booking: fixtureBooking()}
	h := newTestHandlerWithLimit(svc, int64(len(createBookingBody)+1))

	rec := serve(t, h, http.MethodPost, "/bookings", createBookingBody)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}
}

func TestCreateBookingSeriesRejectsABodyOverTheLimit(t *testing.T) {
	svc := &stubUseCase{}
	h := newTestHandlerWithLimit(svc, 32)
	body := `{"room_id":7,"user_name":"ada","start_time":"2026-03-03T09:00:00Z","end_time":"2026-03-03T10:00:00Z","interval":"24h","count":2}`

	rec := serve(t, h, http.MethodPost, "/bookings/series", body)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body)
	}
}

func TestHandlerFallsBackToTheDefaultBodyLimit(t *testing.T) {
	for _, limit := range []int64{0, -1} {
		h := NewHandler(&stubUseCase{}, discardLogger(), nil, limit)
		if h.maxBody != defaultMaxBodyBytes {
			t.Errorf("NewHandler(limit=%d).maxBody = %d, want %d", limit, h.maxBody, defaultMaxBodyBytes)
		}
	}
}

func TestCreateBookingRejectsAnUnknownField(t *testing.T) {
	svc := &stubUseCase{booking: fixtureBooking()}
	body := `{"room_id":7,"user_name":"ada","start_time":"2026-03-03T09:00:00Z","end_time":"2026-03-03T10:00:00Z","who":"ada"}`

	rec := serve(t, newTestHandler(svc), http.MethodPost, "/bookings", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if got := decodeError(t, rec); got.Error != "invalid_json" || got.Field != "body" {
		t.Errorf("body = %+v, want invalid_json on the body field", got)
	}
	if len(svc.created) != 0 {
		t.Errorf("CreateBooking calls = %d, want 0 for an unknown field", len(svc.created))
	}
}

func TestCreateBookingRejectsATrailingValue(t *testing.T) {
	svc := &stubUseCase{booking: fixtureBooking()}

	rec := serve(t, newTestHandler(svc), http.MethodPost, "/bookings", createBookingBody+`{"room_id":8}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if len(svc.created) != 0 {
		t.Errorf("CreateBooking calls = %d, want 0 for a body with two objects", len(svc.created))
	}
}

func TestCreateBookingSeriesRejectsAnUnknownField(t *testing.T) {
	body := `{"room_id":7,"user_name":"ada","start_time":"2026-03-03T09:00:00Z","end_time":"2026-03-03T10:00:00Z","interval":"24h","count":2,"every":"day"}`

	rec := serve(t, newTestHandler(&stubUseCase{}), http.MethodPost, "/bookings/series", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

func TestCreateBookingStillRejectsATypeMismatch(t *testing.T) {
	body := `{"room_id":"seven","user_name":"ada","start_time":"2026-03-03T09:00:00Z","end_time":"2026-03-03T10:00:00Z"}`

	rec := serve(t, newTestHandler(&stubUseCase{}), http.MethodPost, "/bookings", body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if got := decodeError(t, rec); got.Error != "invalid_json" {
		t.Errorf("body = %+v, want invalid_json for a type mismatch", got)
	}
}

func TestAnOversizedBodyIsNotReadIntoTheRequest(t *testing.T) {
	rows := &countingReader{Reader: strings.NewReader(createBookingBody)}
	request := newRequest(http.MethodPost, "/bookings", rows)
	rec := serveRaw(t, NewHandler(&stubUseCase{}, discardLogger(), nil, 16), request)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body)
	}
	if rows.read >= int64(len(createBookingBody)) {
		t.Errorf("read %d bytes, want the reader stopped before the whole body", rows.read)
	}
}
