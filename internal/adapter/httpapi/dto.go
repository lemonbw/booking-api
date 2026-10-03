package httpapi

import (
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

type createBookingRequest struct {
	RoomID    int64     `json:"room_id"`
	UserName  string    `json:"user_name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}
type bookingResponse struct {
	ID          string     `json:"id"`
	RoomID      int64      `json:"room_id"`
	UserName    string     `json:"user_name"`
	StartTime   time.Time  `json:"start_time"`
	EndTime     time.Time  `json:"end_time"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
}

func newBookingResponse(b domain.Booking) bookingResponse {
	return bookingResponse{
		ID:          string(b.ID),
		RoomID:      int64(b.RoomID),
		UserName:    b.UserName,
		StartTime:   b.StartTime,
		EndTime:     b.EndTime,
		Status:      string(b.Status),
		CreatedAt:   b.CreatedAt,
		UpdatedAt:   b.UpdatedAt,
		CancelledAt: b.CancelledAt,
	}
}

type listBookingsResponse struct {
	RoomID   int64             `json:"room_id"`
	Day      string            `json:"day"`
	Bookings []bookingResponse `json:"bookings"`
}

type roomResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type listRoomsResponse struct {
	Rooms []roomResponse `json:"rooms"`
}

func newRoomResponse(r domain.Room) roomResponse {
	return roomResponse{ID: int64(r.ID), Name: r.Name}
}
