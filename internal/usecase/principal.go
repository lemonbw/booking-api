package usecase

import (
	"strings"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

type CreateBookingCommand struct {
	RoomID    domain.RoomID
	UserName  string
	StartTime time.Time
	EndTime   time.Time
	By        Principal
}

type CreateSeriesCommand struct {
	RoomID    domain.RoomID
	UserName  string
	StartTime time.Time
	EndTime   time.Time
	Interval  time.Duration
	Count     int
	By        Principal
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

func (s *BookingService) resolveOwner(by Principal, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if !by.Authenticated {
		if requested == "" {
			return Anonymous.Subject, nil
		}
		return requested, nil
	}
	if by.Admin {
		if requested == "" {
			return by.Identity(), nil
		}
		return requested, nil
	}
	if requested != "" && !by.CanActOn(requested) {
		return "", domain.NewFieldError("user_name", domain.ErrForbidden)
	}
	return by.Identity(), nil
}
