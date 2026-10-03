package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

type CreateBookingCommand struct {
	RoomID    domain.RoomID
	UserName  string
	StartTime time.Time
	EndTime   time.Time
}

type ListBookingsQuery struct {
	RoomID domain.RoomID
	Day    time.Time
}

type BookingService struct {
	bookings BookingRepository
	rooms    RoomRepository
	clock    Clock
	ids      IDGenerator
	log      Logger
}

func NewBookingService(bookings BookingRepository, rooms RoomRepository, clock Clock, ids IDGenerator, log Logger) *BookingService {
	if log == nil {
		log = nopLogger{}
	}
	return &BookingService{
		bookings: bookings,
		rooms:    rooms,
		clock:    clock,
		ids:      ids,
		log:      log,
	}
}
func (s *BookingService) CreateBooking(ctx context.Context, cmd CreateBookingCommand) (domain.Booking, error) {
	now := s.clock.Now().UTC()

	if cmd.RoomID <= 0 {
		return domain.Booking{}, domain.NewFieldError("room_id", domain.ErrInvalidRoomID)
	}
	if _, err := s.ensureRoomExists(ctx, cmd.RoomID); err != nil {
		return domain.Booking{}, err
	}
	id, err := s.ids.NewID()
	if err != nil {
		return domain.Booking{}, fmt.Errorf("generate booking id: %w", err)
	}
	booking, err := domain.NewBooking(domain.BookingID(id), cmd.RoomID, cmd.UserName, cmd.StartTime, cmd.EndTime, now)
	if err != nil {
		return domain.Booking{}, err
	}
	if err := s.preflightOverlap(ctx, booking.RoomID, booking.StartTime, booking.EndTime); err != nil {
		return domain.Booking{}, err
	}
	created, err := s.bookings.Create(ctx, booking)
	if err != nil {
		return domain.Booking{}, s.interpretCreateError(ctx, booking, err)
	}
	s.log.InfoContext(ctx, "booking created",
		"booking_id", created.ID,
		"room_id", created.RoomID,
		"user", created.UserName,
	)
	return created, nil
}
func (s *BookingService) ListRoomBookings(ctx context.Context, q ListBookingsQuery) ([]domain.Booking, error) {
	if q.RoomID <= 0 {
		return nil, domain.NewFieldError("room_id", domain.ErrInvalidRoomID)
	}
	if _, err := s.ensureRoomExists(ctx, q.RoomID); err != nil {
		return nil, err
	}
	bookings, err := s.bookings.ListActiveByRoomAndRange(ctx, q.RoomID, domain.DayBounds(q.Day))
	if err != nil {
		return nil, fmt.Errorf("list bookings: %w", err)
	}
	if bookings == nil {
		bookings = []domain.Booking{}
	}
	return bookings, nil
}
func (s *BookingService) GetBooking(ctx context.Context, id domain.BookingID) (domain.Booking, error) {
	if id == "" {
		return domain.Booking{}, domain.NewFieldError("id", domain.ErrInvalidBookingID)
	}
	return s.bookings.GetByID(ctx, id)
}
func (s *BookingService) CancelBooking(ctx context.Context, id domain.BookingID) (domain.Booking, error) {
	if id == "" {
		return domain.Booking{}, domain.NewFieldError("id", domain.ErrInvalidBookingID)
	}
	booking, err := s.bookings.Cancel(ctx, id, s.clock.Now().UTC())
	if err != nil {
		return domain.Booking{}, err
	}
	s.log.InfoContext(ctx, "booking cancelled", "booking_id", booking.ID, "room_id", booking.RoomID)
	return booking, nil
}
func (s *BookingService) ListRooms(ctx context.Context) ([]domain.Room, error) {
	rooms, err := s.rooms.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	if rooms == nil {
		rooms = []domain.Room{}
	}
	return rooms, nil
}
func (s *BookingService) GetRoom(ctx context.Context, id domain.RoomID) (domain.Room, error) {
	return s.ensureRoomExists(ctx, id)
}
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

type nopLogger struct{}

func (nopLogger) InfoContext(context.Context, string, ...any)  {}
func (nopLogger) WarnContext(context.Context, string, ...any)  {}
func (nopLogger) ErrorContext(context.Context, string, ...any) {}
