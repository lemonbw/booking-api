package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

func TestDurationDecodesFromAString(t *testing.T) {
	var req createBookingSeriesRequest
	body := `{"room_id":7,"interval":"1h30m","count":3}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if req.Interval != Duration(90*time.Minute) {
		t.Errorf("Interval = %s, want 1h30m", time.Duration(req.Interval))
	}
	if req.RoomID != 7 || req.Count != 3 {
		t.Errorf("request = %+v, want the room and the count decoded", req)
	}
}

func TestDurationRejectsAnythingButAString(t *testing.T) {
	cases := map[string]string{
		"number":    `{"interval":3600}`,
		"empty":     `{"interval":""}`,
		"unitless":  `{"interval":"60"}`,
		"bare word": `{"interval":"daily"}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var req createBookingSeriesRequest
			if err := json.Unmarshal([]byte(body), &req); err == nil {
				t.Fatalf("Unmarshal(%s) = nil error, want a rejection", body)
			}
		})
	}
}

func TestDurationRendersAsAString(t *testing.T) {
	encoded, err := json.Marshal(Duration(24 * time.Hour))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(encoded) != `"24h0m0s"` {
		t.Errorf("Marshal() = %s, want %q", encoded, "24h0m0s")
	}
}

func TestEmptyCollectionsAreRenderedAsArrays(t *testing.T) {
	if got := mustJSON(t, newBookingListResponse(nil)); got != "[]" {
		t.Errorf("newBookingListResponse(nil) = %s, want []", got)
	}
	if got := mustJSON(t, newRoomListResponse(nil)); got != "[]" {
		t.Errorf("newRoomListResponse(nil) = %s, want []", got)
	}
}

func TestBookingResponseCarriesTheCancelledStamp(t *testing.T) {
	booking := fixtureBooking()
	booking.Status = domain.BookingStatusCancelled
	booking.CancelledAt = &testNow

	if got := mustJSON(t, newBookingResponse(booking)); got == "" {
		t.Fatal("newBookingResponse() rendered nothing")
	}

	var got bookingResponse
	if err := json.Unmarshal([]byte(mustJSON(t, newBookingResponse(booking))), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.CancelledAt == nil || !got.CancelledAt.Equal(testNow) {
		t.Errorf("cancelled_at = %v, want %s", got.CancelledAt, testNow)
	}
	if got.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", got.Status)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal(%v) error = %v", value, err)
	}
	return string(encoded)
}
