package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

func (s *BookingService) ensureRoomExists(ctx context.Context, id domain.RoomID) (domain.Room, error) {
	room, err := s.rooms.GetByID(ctx, id)
	switch {
	case errors.Is(err, domain.ErrRoomNotFound):
		return domain.Room{}, domain.NewFieldError("room_id", domain.ErrRoomNotFound)
	case err != nil:
		return domain.Room{}, fmt.Errorf("load room %d: %w", id, err)
	}
	return room, nil
}

func (s *BookingService) preflightOverlap(ctx context.Context, roomID domain.RoomID, start, end time.Time) error {
	conflicts, err := s.bookings.FindActiveOverlapping(ctx, roomID, domain.NewTimeRange(start, end))
	if err != nil {
		return fmt.Errorf("check overlapping bookings: %w", err)
	}
	if len(conflicts) > 0 {
		return &domain.ConflictError{RoomID: roomID, Conflicts: conflicts}
	}
	return nil
}

func (s *BookingService) interpretCreateError(ctx context.Context, booking domain.Booking, err error) error {
	if errors.Is(err, domain.ErrOverlap) {
		conflicts, cErr := s.bookings.FindActiveOverlapping(ctx, booking.RoomID, domain.NewTimeRange(booking.StartTime, booking.EndTime))
		if cErr != nil {
			s.log.WarnContext(ctx, "failed to load conflicting bookings after overlap", "error", cErr)
		}
		return &domain.ConflictError{RoomID: booking.RoomID, Conflicts: conflicts}
	}
	return fmt.Errorf("create booking: %w", err)
}

func (s *BookingService) count(kind string, err error) {
	if s.counter == nil {
		return
	}
	if err == nil {
		s.counter.CountBooking(kind, "ok")
		return
	}
	outcome := "error"
	for _, sentinel := range []struct {
		err  error
		name string
	}{
		{domain.ErrOverlap, "conflict"},
		{domain.ErrForbidden, "forbidden"},
		{domain.ErrBookingInPast, "in_past"},
		{domain.ErrBookingAlreadyStarted, "already_started"},
		{domain.ErrUnavailable, "unavailable"},
	} {
		if errors.Is(err, sentinel.err) {
			outcome = sentinel.name
			break
		}
	}
	s.counter.CountBooking(kind, outcome)
}

type nopLogger struct{}

func (nopLogger) InfoContext(context.Context, string, ...any)  {}
func (nopLogger) WarnContext(context.Context, string, ...any)  {}
func (nopLogger) ErrorContext(context.Context, string, ...any) {}
