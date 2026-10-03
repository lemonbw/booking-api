package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

func (s *BookingService) createBookingSeries(ctx context.Context, cmd CreateSeriesCommand) ([]domain.Booking, error) {
	now := s.clock.Now().UTC()

	if cmd.RoomID <= 0 {
		return nil, domain.NewFieldError("room_id", domain.ErrInvalidRoomID)
	}
	owner, err := s.resolveOwner(cmd.By, cmd.UserName)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureRoomExists(ctx, cmd.RoomID); err != nil {
		return nil, err
	}
	seriesID, err := s.ids.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate series id: %w", err)
	}
	series, err := domain.NewSeries(
		domain.SeriesID(seriesID), cmd.RoomID, owner,
		domain.NewTimeRange(cmd.StartTime, cmd.EndTime),
		cmd.Interval, cmd.Count, s.maxOccurrences,
		now,
	)
	if err != nil {
		return nil, err
	}
	bookings, err := s.buildOccurrences(series, now)
	if err != nil {
		return nil, err
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

func (s *BookingService) buildOccurrences(series domain.Series, now time.Time) ([]domain.Booking, error) {
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
	return bookings, nil
}

func (s *BookingService) preflightSeriesOverlap(ctx context.Context, series domain.Series) error {
	conflicts, err := s.bookings.FindActiveOverlapping(ctx, series.RoomID, series.Bounds())
	if err != nil {
		return fmt.Errorf("check overlapping bookings: %w", err)
	}
	if len(conflicts) == 0 {
		return nil
	}
	return &domain.ConflictError{RoomID: series.RoomID, Conflicts: conflicts}
}
