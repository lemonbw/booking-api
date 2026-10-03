package usecase

import (
	"context"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

type BookingRepository interface {
	Create(ctx context.Context, booking domain.Booking) (domain.Booking, error)

	CreateMany(ctx context.Context, bookings []domain.Booking) ([]domain.Booking, error)

	GetByID(ctx context.Context, id domain.BookingID) (domain.Booking, error)

	ListActiveByRoomAndRange(ctx context.Context, roomID domain.RoomID, r domain.TimeRange) ([]domain.Booking, error)

	FindActiveOverlapping(ctx context.Context, roomID domain.RoomID, r domain.TimeRange) ([]domain.Booking, error)

	Cancel(ctx context.Context, id domain.BookingID, at time.Time) (domain.Booking, error)
}
type RoomRepository interface {
	GetByID(ctx context.Context, id domain.RoomID) (domain.Room, error)
	List(ctx context.Context) ([]domain.Room, error)
}
type Clock interface {
	Now() time.Time
}
type IDGenerator interface {
	NewID() (string, error)
}
type Logger interface {
	InfoContext(ctx context.Context, msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}

type BookingUseCase interface {
	CreateBooking(ctx context.Context, cmd CreateBookingCommand) (domain.Booking, error)

	ListRoomBookings(ctx context.Context, q ListBookingsQuery) ([]domain.Booking, error)

	GetBooking(ctx context.Context, id domain.BookingID) (domain.Booking, error)

	CancelBooking(ctx context.Context, id domain.BookingID) (domain.Booking, error)

	ListRooms(ctx context.Context) ([]domain.Room, error)

	GetRoom(ctx context.Context, id domain.RoomID) (domain.Room, error)
}
