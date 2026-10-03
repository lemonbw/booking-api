package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

type CreateBookingCommand struct {
	RoomID    domain.RoomID
	UserName  string
	StartTime time.Time
	EndTime   time.Time
}

type CreateSeriesCommand struct {
	RoomID    domain.RoomID
	UserName  string
	StartTime time.Time
	EndTime   time.Time
	Interval  time.Duration
	Count     int
}

type Principal struct {
	Subject string

	DisplayName string

	Admin bool

	Authenticated bool
}

var Anonymous = Principal{Subject: "anonymous"}

func (p Principal) Identity() string {
	if subject := strings.TrimSpace(p.Subject); subject != "" {
		return subject
	}
	if name := strings.TrimSpace(p.DisplayName); name != "" {
		return name
	}
	return Anonymous.Subject
}

func (p Principal) CanActOn(owner string) bool {
	if p.Admin || !p.Authenticated {
		return true
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return false
	}
	if strings.EqualFold(owner, p.Identity()) {
		return true
	}
	display := strings.TrimSpace(p.DisplayName)
	return display != "" && strings.EqualFold(owner, display)
}

func (p Principal) CanManage(owner string) bool {
	if !p.Authenticated {
		return true
	}
	return p.CanActOn(owner)
}

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

	maxOccurrences int
	pageSize       int
}

func NewBookingService(bookings BookingRepository, rooms RoomRepository, clock Clock, ids IDGenerator, log Logger, maxOccurrences, pageSize int) *BookingService {
	if log == nil {
		log = nopLogger{}
	}
	return &BookingService{
		bookings: bookings,
		rooms:    rooms,
		clock:    clock,
		ids:      ids,
		log:      log,

		maxOccurrences: maxOccurrences,
		pageSize:       pageSize,
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
	booking, err := domain.NewBooking(domain.BookingID(id), "", cmd.RoomID, cmd.UserName, cmd.StartTime, cmd.EndTime, now)
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
	now := s.clock.Now().UTC()

	if cmd.RoomID <= 0 {
		return nil, domain.NewFieldError("room_id", domain.ErrInvalidRoomID)
	}
	if _, err := s.ensureRoomExists(ctx, cmd.RoomID); err != nil {
		return nil, err
	}
	seriesID, err := s.ids.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate series id: %w", err)
	}
	series, err := domain.NewSeries(
		domain.SeriesID(seriesID), cmd.RoomID, cmd.UserName,
		domain.NewTimeRange(cmd.StartTime, cmd.EndTime),
		cmd.Interval, cmd.Count, s.maxOccurrences,
		now,
	)
	if err != nil {
		return nil, err
	}
	occurrences := series.Occurrences()
	bookings := make([]domain.Booking, 0, len(occurrences))
	for _, occurrence := range occurrences {
		id, err := s.ids.NewID()
		if err != nil {
			return nil, fmt.Errorf("generate booking id: %w", err)
		}
		booking, err := domain.NewBooking(domain.BookingID(id), series.ID, series.RoomID, series.UserName,
			occurrence.Start, occurrence.End, now)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, booking)
	}
	if err := s.preflightSeriesOverlap(ctx, series); err != nil {
		return nil, err
	}
	created, err := s.bookings.CreateMany(ctx, bookings)
	if err != nil {
		return nil, s.interpretCreateError(ctx, bookings[0], err)
	}
	s.log.InfoContext(ctx, "booking series created",
		"series_id", series.ID,
		"room_id", series.RoomID,
		"user", series.UserName,
		"occurrences", len(created),
	)
	return created, nil
}

func (s *BookingService) preflightSeriesOverlap(ctx context.Context, series domain.Series) error {
	conflicts, err := s.bookings.FindActiveOverlapping(ctx, series.RoomID, series.Bounds())
	if err != nil {
		return fmt.Errorf("check overlapping bookings: %w", err)
	}
	if len(conflicts) == 0 {
		return nil
	}
	occurrences := series.Occurrences()
	var blocked []domain.Booking
	for _, booking := range conflicts {
		window := domain.NewTimeRange(booking.StartTime, booking.EndTime)
		for _, occurrence := range occurrences {
			if window.Overlaps(occurrence) {
				blocked = append(blocked, booking)
				break
			}
		}
	}
	if len(blocked) > 0 {
		return &domain.ConflictError{RoomID: series.RoomID, Conflicts: blocked}
	}
	return nil
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
