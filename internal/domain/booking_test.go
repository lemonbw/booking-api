package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

var (
	referenceNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tomorrow     = referenceNow.AddDate(0, 0, 1)
)

func validStart() time.Time { return tomorrow.Add(9 * time.Hour) }

func TestNewBookingAcceptsAValidRequest(t *testing.T) {
	t.Parallel()

	booking, err := domain.NewBooking("id-1", "", 7, "Ada", validStart(), validStart().Add(time.Hour), referenceNow)
	if err != nil {
		t.Fatalf("NewBooking() returned an unexpected error: %v", err)
	}
	if booking.Status != domain.BookingStatusActive {
		t.Errorf("Status = %q, want %q", booking.Status, domain.BookingStatusActive)
	}
	if booking.UserName != "Ada" {
		t.Errorf("UserName = %q, want %q", booking.UserName, "Ada")
	}
	if !booking.StartTime.Equal(validStart()) {
		t.Errorf("StartTime = %s, want %s", booking.StartTime, validStart())
	}
	if got, want := booking.StartTime.Location(), time.UTC; got != want {
		t.Errorf("StartTime location = %s, want %s", got, want)
	}
}

func TestNewBookingRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		id        domain.BookingID
		roomID    domain.RoomID
		userName  string
		start     time.Time
		end       time.Time
		wantField string
	}{
		{"empty id", "", 1, "Ada", validStart(), validStart().Add(time.Hour), "id"},
		{"zero room", "id-1", 0, "Ada", validStart(), validStart().Add(time.Hour), "room_id"},
		{"blank user", "id-1", 1, "   ", validStart(), validStart().Add(time.Hour), "user_name"},
		{"end before start", "id-1", 1, "Ada", validStart(), validStart(), "start_time"},
		{"end before start, far", "id-1", 1, "Ada", validStart(), validStart().Add(-time.Hour), "start_time"},
		{"in the past", "id-1", 1, "Ada", referenceNow.Add(-time.Hour), referenceNow, "start_time"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := domain.NewBooking(tc.id, "", tc.roomID, tc.userName, tc.start, tc.end, referenceNow)
			if err == nil {
				t.Fatal("NewBooking() returned nil error, want a field error")
			}
			var fieldErr *domain.FieldError
			if !errorAs(err, &fieldErr) {
				t.Fatalf("error type = %T, want *domain.FieldError", err)
			}
			if fieldErr.Field != tc.wantField {
				t.Errorf("Field = %q, want %q", fieldErr.Field, tc.wantField)
			}
		})
	}
}

func TestNewBookingRejectsAnOverlongUserName(t *testing.T) {
	t.Parallel()

	tooLong := strings.Repeat("я", domain.MaxUserNameLen+1)
	if _, err := domain.NewBooking("id-1", "", 1, tooLong, validStart(), validStart().Add(time.Hour), referenceNow); err == nil {
		t.Fatal("NewBooking() returned nil error, want an error for an overlong user_name")
	}

	atLimit := strings.Repeat("я", domain.MaxUserNameLen)
	if _, err := domain.NewBooking("id-1", "", 1, atLimit, validStart(), validStart().Add(time.Hour), referenceNow); err != nil {
		t.Fatalf("NewBooking() rejected a name of exactly %d runes: %v", domain.MaxUserNameLen, err)
	}
}

func TestBookingCancel(t *testing.T) {
	t.Parallel()

	future := validStart()
	booking, err := domain.NewBooking("id-1", "", 1, "Ada", future, future.Add(time.Hour), referenceNow)
	if err != nil {
		t.Fatalf("NewBooking() returned an unexpected error: %v", err)
	}

	if booking.IsCancellableAt(referenceNow) != true {
		t.Error("IsCancellableAt() = false, want true for a booking that has not started")
	}
	if err := booking.Cancel(referenceNow); err != nil {
		t.Fatalf("Cancel() returned an unexpected error: %v", err)
	}
	if booking.Status != domain.BookingStatusCancelled {
		t.Errorf("Status = %q, want %q", booking.Status, domain.BookingStatusCancelled)
	}
	if booking.CancelledAt == nil || !booking.CancelledAt.Equal(referenceNow) {
		t.Errorf("CancelledAt = %v, want %s", booking.CancelledAt, referenceNow)
	}
	if err := booking.Cancel(referenceNow.Add(time.Minute)); err != nil {
		t.Fatalf("second Cancel() returned an unexpected error: %v", err)
	}
	if !booking.CancelledAt.Equal(referenceNow) {
		t.Errorf("CancelledAt = %s after a second Cancel(), want the original %s", booking.CancelledAt, referenceNow)
	}
}

func TestBookingCancelIsRejectedOnceTheBookingStarted(t *testing.T) {
	t.Parallel()

	start := validStart()
	booking, err := domain.NewBooking("id-1", "", 1, "Ada", start, start.Add(time.Hour), referenceNow)
	if err != nil {
		t.Fatalf("NewBooking() returned an unexpected error: %v", err)
	}

	afterStart := start.Add(time.Minute)
	if booking.IsCancellableAt(afterStart) {
		t.Error("IsCancellableAt() = true, want false once the booking has started")
	}
	err = booking.Cancel(afterStart)
	if !errorIs(err, domain.ErrBookingAlreadyStarted) {
		t.Fatalf("Cancel() error = %v, want ErrBookingAlreadyStarted", err)
	}
	if booking.Status != domain.BookingStatusActive {
		t.Errorf("Status = %q, want the booking to stay %q", booking.Status, domain.BookingStatusActive)
	}
}
