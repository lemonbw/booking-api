package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

type ListBookingsQuery struct {
	RoomID domain.RoomID
	Day    time.Time
	Limit  int
	Offset int
}

type BookingService struct {
	bookings BookingRepository
	rooms    RoomRepository
	clock    Clock
	ids      IDGenerator
	log      Logger
	counter  OperationCounter

	maxOccurrences int
	pageSize       int
}

func NewBookingService(bookings BookingRepository, rooms RoomRepository, clock Clock, ids IDGenerator, log Logger, counter OperationCounter, maxOccurrences, pageSize int) *BookingService {
	if log == nil {
		log = nopLogger{}
	}
	return &BookingService{
		bookings: bookings,
		rooms:    rooms,
		clock:    clock,
		ids:      ids,
		log:      log,
		counter:  counter,

		maxOccurrences: maxOccurrences,
		pageSize:       pageSize,
	}
}

func (s *BookingService) CreateBooking(ctx context.Context, cmd CreateBookingCommand) (domain.Booking, error) {
	booking, err := s.createBooking(ctx, cmd)
	s.count("created", err)
	return booking, err
}
func (s *BookingService) createBooking(ctx context.Context, cmd CreateBookingCommand) (domain.Booking, error) {
	now := s.clock.Now().UTC()

	if cmd.RoomID <= 0 {
		return domain.Booking{}, domain.NewFieldError("room_id", domain.ErrInvalidRoomID)
	}
	owner, err := s.resolveOwner(cmd.By, cmd.UserName)
	if err != nil {
		return domain.Booking{}, err
	}
	if _, err := s.ensureRoomExists(ctx, cmd.RoomID); err != nil {
		return domain.Booking{}, err
	}
	id, err := s.ids.NewID()
	if err != nil {
		return domain.Booking{}, fmt.Errorf("generate booking id: %w", err)
	}
	booking, err := domain.NewBooking(domain.BookingID(id), "", cmd.RoomID, owner, cmd.StartTime, cmd.EndTime, now)
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

func (s *BookingService) CreateBookingSeries(ctx context.Context, cmd CreateSeriesCommand) ([]domain.Booking, error) {
	bookings, err := s.createBookingSeries(ctx, cmd)
	s.count("series_created", err)
	return bookings, err
}

func (s *BookingService) ListRoomBookings(ctx context.Context, q ListBookingsQuery) ([]domain.Booking, error) {
	if q.RoomID <= 0 {
		return nil, domain.NewFieldError("room_id", domain.ErrInvalidRoomID)
	}
	if _, err := s.ensureRoomExists(ctx, q.RoomID); err != nil {
		return nil, err
	}
	limit, offset := s.normalizePage(q.Limit, q.Offset)
	bookings, err := s.bookings.ListActiveByRoomAndRange(ctx, q.RoomID, domain.DayBounds(q.Day), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list bookings: %w", err)
	}
	if bookings == nil {
		bookings = []domain.Booking{}
	}
	return bookings, nil
}

const MaxPageSize = 500

func (s *BookingService) normalizePage(limit, offset int) (int, int) {
	switch {
	case limit <= 0:
		limit = s.pageSize
	case limit > MaxPageSize:
		limit = MaxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
func (s *BookingService) GetBooking(ctx context.Context, id domain.BookingID) (domain.Booking, error) {
	if id == "" {
		return domain.Booking{}, domain.NewFieldError("id", domain.ErrInvalidBookingID)
	}
	return s.bookings.GetByID(ctx, id)
}
func (s *BookingService) CancelBooking(ctx context.Context, id domain.BookingID, by Principal) (domain.Booking, error) {
	booking, err := s.cancelBooking(ctx, id, by)
	s.count("cancelled", err)
	return booking, err
}
func (s *BookingService) cancelBooking(ctx context.Context, id domain.BookingID, by Principal) (domain.Booking, error) {
	if id == "" {
		return domain.Booking{}, domain.NewFieldError("id", domain.ErrInvalidBookingID)
	}
	existing, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return domain.Booking{}, err
	}
	if !by.CanManage(existing.UserName) {
		return domain.Booking{}, domain.NewFieldError("id", domain.ErrForbidden)
	}
	booking, err := s.bookings.Cancel(ctx, id, s.clock.Now().UTC())
	if err != nil {
		return domain.Booking{}, err
	}
	s.log.InfoContext(ctx, "booking cancelled",
		"booking_id", booking.ID,
		"room_id", booking.RoomID,
		"by", by.Identity(),
	)
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
