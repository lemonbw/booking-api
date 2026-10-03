package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type BookingStatus string

const (
	BookingStatusActive BookingStatus = "active"

	BookingStatusCancelled BookingStatus = "cancelled"
)

type BookingID string

const MaxUserNameLen = 255

type Booking struct {
	ID          BookingID
	RoomID      RoomID
	UserName    string
	StartTime   time.Time
	EndTime     time.Time
	Status      BookingStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CancelledAt *time.Time
}

func (b Booking) IsActive() bool { return b.Status == BookingStatusActive }

func (b Booking) Duration() time.Duration { return b.EndTime.Sub(b.StartTime) }

func (b Booking) IsOwnedBy(user string) bool {
	return strings.EqualFold(b.UserName, strings.TrimSpace(user))
}
func (b Booking) IsCancellableAt(now time.Time) bool {
	return b.StartTime.After(now.UTC())
}
func (b *Booking) Cancel(now time.Time) error {
	switch b.Status {
	case BookingStatusCancelled:
		return nil
	case BookingStatusActive:
	default:
		return fmt.Errorf("cancel booking %s: booking is in unknown state %q", b.ID, b.Status)
	}
	now = now.UTC()
	if !b.IsCancellableAt(now) {
		return NewFieldError("start_time", ErrBookingAlreadyStarted)
	}
	b.Status = BookingStatusCancelled
	b.CancelledAt = &now
	b.UpdatedAt = now
	return nil
}

type TimeRange struct {
	Start time.Time
	End   time.Time
}

func NewTimeRange(start, end time.Time) TimeRange {
	return TimeRange{Start: start.UTC(), End: end.UTC()}
}
func (r TimeRange) Duration() time.Duration { return r.End.Sub(r.Start) }

func (r TimeRange) Overlaps(other TimeRange) bool {
	return r.Start.Before(other.End) && other.Start.Before(r.End)
}
func DayBounds(day time.Time) TimeRange {
	start := day.UTC()
	midnight := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	return TimeRange{Start: midnight, End: midnight.AddDate(0, 0, 1)}
}

func NewBooking(id BookingID, roomID RoomID, userName string, start, end, now time.Time) (Booking, error) {
	if strings.TrimSpace(string(id)) == "" {
		return Booking{}, NewFieldError("id", ErrInvalidBookingID)
	}
	if roomID <= 0 {
		return Booking{}, NewFieldError("room_id", ErrInvalidRoomID)
	}
	if err := validateUserName(userName); err != nil {
		return Booking{}, err
	}
	start, end = start.UTC(), end.UTC()
	if !start.Before(end) {
		return Booking{}, NewFieldError("start_time", ErrInvalidTimeRange)
	}
	if start.Before(now.UTC()) {
		return Booking{}, NewFieldError("start_time", ErrBookingInPast)
	}
	createdAt := now.UTC()
	return Booking{
		ID:        id,
		RoomID:    roomID,
		UserName:  strings.TrimSpace(userName),
		StartTime: start,
		EndTime:   end,
		Status:    BookingStatusActive,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}, nil
}

func validateUserName(userName string) error {
	userName = strings.TrimSpace(userName)
	switch {
	case userName == "":
		return NewFieldError("user_name", ErrInvalidUserName)
	case !utf8.ValidString(userName):
		return NewFieldError("user_name", fmt.Errorf("%w: value is not valid UTF-8", ErrInvalidUserName))
	case utf8.RuneCountInString(userName) > MaxUserNameLen:
		return NewFieldError("user_name", fmt.Errorf("%w: got %d characters", ErrInvalidUserName, utf8.RuneCountInString(userName)))
	}
	return nil
}
