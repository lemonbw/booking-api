package httpapi

import (
	"encoding/json"
	"fmt"
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

type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("duration must be a string such as \"24h\": %w", err)
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("duration %q is not valid, use a form such as \"24h\": %w", raw, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

type createBookingSeriesRequest struct {
	RoomID    int64     `json:"room_id"`
	UserName  string    `json:"user_name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Interval  Duration  `json:"interval"`
	Count     int       `json:"count"`
}
type bookingSeriesResponse struct {
	SeriesID   string            `json:"series_id"`
	RoomID     int64             `json:"room_id"`
	UserName   string            `json:"user_name"`
	Count      int               `json:"count"`
	Interval   Duration          `json:"interval"`
	FirstStart time.Time         `json:"first_start"`
	LastEnd    time.Time         `json:"last_end"`
	Bookings   []bookingResponse `json:"bookings"`
}

func newBookingListResponse(bookings []domain.Booking) []bookingResponse {
	out := make([]bookingResponse, 0, len(bookings))
	for _, b := range bookings {
		out = append(out, newBookingResponse(b))
	}
	return out
}

type roomResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func newRoomListResponse(rooms []domain.Room) []roomResponse {
	out := make([]roomResponse, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, newRoomResponse(room))
	}
	return out
}

func newRoomResponse(r domain.Room) roomResponse {
	return roomResponse{ID: int64(r.ID), Name: r.Name}
}
