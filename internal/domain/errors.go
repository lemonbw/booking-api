package domain

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidTimeRange = errors.New("invalid time range: start_time must be strictly before end_time")

	ErrBookingInPast = errors.New("booking cannot be placed in the past")

	ErrOverlap = errors.New("requested interval overlaps an existing active booking")

	ErrRoomNotFound = errors.New("room not found")

	ErrBookingNotFound = errors.New("booking not found")

	ErrInvalidBookingID = errors.New("booking id must not be empty")

	ErrInvalidUserName = errors.New("user_name must be a non-empty string of at most 255 characters")

	ErrInvalidRoomID = errors.New("room_id must be a positive integer")

	ErrInvalidInterval = errors.New("interval must be a positive duration")

	ErrSeriesTooLong = errors.New("too many occurrences requested")

	ErrBookingAlreadyStarted = errors.New("booking has already started and can no longer be cancelled")

	ErrUnavailable = errors.New("dependency is temporarily unavailable")

	ErrIntegrity = errors.New("data integrity violation")
)

type FieldError struct {
	Field   string
	Message string
	err     error
}

func (e *FieldError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
func (e *FieldError) Unwrap() error { return e.err }

func NewFieldError(field string, err error) *FieldError {
	return &FieldError{Field: field, Message: err.Error(), err: err}
}

type ConflictError struct {
	RoomID    RoomID
	Conflicts []Booking
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s: room=%d conflicts=%d", ErrOverlap.Error(), e.RoomID, len(e.Conflicts))
}
func (e *ConflictError) Unwrap() error { return ErrOverlap }
