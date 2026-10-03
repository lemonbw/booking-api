package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
	"github.com/lemonbw/booking-api/internal/usecase"
)

var testNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func fixture(t *testing.T) (*usecase.BookingService, *fakeBookingRepository, *fakeRoomRepository, *fakeCounter) {
	t.Helper()

	now := testNow
	clock := &fakeClock{last: now}
	ids := &fakeIDs{}
	bookings := newFakeBookingRepository()
	rooms := newFakeRoomRepository(1, 2, 3)
	counter := newFakeCounter()

	svc := usecase.NewBookingService(bookings, rooms, clock, ids, &fakeLogger{}, counter, 52, 100)
	return svc, bookings, rooms, counter
}

func at(hour int) time.Time {
	return testNow.AddDate(0, 0, 1).Add(time.Duration(hour) * time.Hour)
}

func createCmd(roomID int64, user string, startHour, endHour int) usecase.CreateBookingCommand {
	return usecase.CreateBookingCommand{
		RoomID:    domain.RoomID(roomID),
		UserName:  user,
		StartTime: at(startHour),
		EndTime:   at(endHour),
	}
}

func TestCreateBookingStoresTheBooking(t *testing.T) {
	t.Parallel()

	svc, bookings, _, counter := fixture(t)

	created, err := svc.CreateBooking(context.Background(), createCmd(1, "Ada", 10, 11))
	if err != nil {
		t.Fatalf("CreateBooking() returned an unexpected error: %v", err)
	}
	if created.RoomID != 1 || created.UserName != "Ada" {
		t.Errorf("created = %+v, want room 1 owned by Ada", created)
	}
	if created.Status != domain.BookingStatusActive {
		t.Errorf("Status = %q, want %q", created.Status, domain.BookingStatusActive)
	}
	if bookings.count() != 1 {
		t.Errorf("stored %d bookings, want 1", bookings.count())
	}
	if got := counter.get("created:ok"); got != 1 {
		t.Errorf("counter[created:ok] = %d, want 1", got)
	}
}

func TestCreateBookingRejectsAnOverlap(t *testing.T) {
	t.Parallel()

	svc, _, _, counter := fixture(t)
	ctx := context.Background()

	if _, err := svc.CreateBooking(ctx, createCmd(1, "Ada", 10, 11)); err != nil {
		t.Fatalf("first CreateBooking() returned an unexpected error: %v", err)
	}

	tests := []struct {
		name       string
		start, end int
	}{
		{"identical", 10, 11},
		{"contained", 10, 12},
		{"containing", 9, 12},
		{"overlapping start", 10, 12},
		{"overlapping end", 9, 11},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateBooking(ctx, createCmd(1, "Bob", tc.start, tc.end))
			if !errors.Is(err, domain.ErrOverlap) {
				t.Fatalf("CreateBooking() error = %v, want ErrOverlap", err)
			}
			var conflict *domain.ConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("error type = %T, want *domain.ConflictError", err)
			}
			if conflict.RoomID != 1 || len(conflict.Conflicts) == 0 {
				t.Errorf("conflict = %+v, want room 1 with at least one blocking booking", conflict)
			}
		})
	}

	if got, want := counter.get("created:conflict"), len(tests); got != want {
		t.Errorf("counter[created:conflict] = %d, want %d", got, len(tests))
	}
}

func TestCreateBookingAllowsAdjacentSlots(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := fixture(t)
	ctx := context.Background()

	if _, err := svc.CreateBooking(ctx, createCmd(1, "Ada", 10, 11)); err != nil {
		t.Fatalf("first CreateBooking() returned an unexpected error: %v", err)
	}
	if _, err := svc.CreateBooking(ctx, createCmd(1, "Bob", 11, 12)); err != nil {
		t.Fatalf("adjacent CreateBooking() returned an unexpected error: %v", err)
	}
}

func TestCreateBookingIgnoresCancelledBookings(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := fixture(t)
	ctx := context.Background()

	first, err := svc.CreateBooking(ctx, createCmd(1, "Ada", 10, 11))
	if err != nil {
		t.Fatalf("CreateBooking() returned an unexpected error: %v", err)
	}
	if _, err := svc.CancelBooking(ctx, first.ID, usecase.Anonymous); err != nil {
		t.Fatalf("CancelBooking() returned an unexpected error: %v", err)
	}
	if _, err := svc.CreateBooking(ctx, createCmd(1, "Bob", 10, 11)); err != nil {
		t.Fatalf("CreateBooking() into a cancelled slot returned an unexpected error: %v", err)
	}
}

func TestCreateBookingRejectsAnUnknownRoom(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := fixture(t)

	_, err := svc.CreateBooking(context.Background(), createCmd(99, "Ada", 10, 11))
	if !errors.Is(err, domain.ErrRoomNotFound) {
		t.Fatalf("CreateBooking() error = %v, want ErrRoomNotFound", err)
	}
}

func TestCreateBookingIsAtomicOnAStorageConflict(t *testing.T) {
	t.Parallel()

	svc, bookings, _, _ := fixture(t)
	bookings.createErr = domain.ErrOverlap

	_, err := svc.CreateBooking(context.Background(), createCmd(1, "Ada", 10, 11))
	if !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("CreateBooking() error = %v, want ErrOverlap", err)
	}
	if !errors.As(err, new(*domain.ConflictError)) {
		t.Errorf("error type = %T, want the error to be a *domain.ConflictError", err)
	}
}

func TestListRoomBookingsReturnsOnlyTheRequestedDay(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := fixture(t)
	ctx := context.Background()

	if _, err := svc.CreateBooking(ctx, createCmd(1, "Ada", 10, 11)); err != nil {
		t.Fatalf("CreateBooking() returned an unexpected error: %v", err)
	}

	day := testNow.AddDate(0, 0, 1)
	bookings, err := svc.ListRoomBookings(ctx, usecase.ListBookingsQuery{RoomID: 1, Day: day})
	if err != nil {
		t.Fatalf("ListRoomBookings() returned an unexpected error: %v", err)
	}
	if len(bookings) != 1 {
		t.Fatalf("len(bookings) = %d, want 1", len(bookings))
	}

	other, err := svc.ListRoomBookings(ctx, usecase.ListBookingsQuery{RoomID: 1, Day: day.AddDate(0, 0, 1)})
	if err != nil {
		t.Fatalf("ListRoomBookings() returned an unexpected error: %v", err)
	}
	if len(other) != 0 {
		t.Errorf("len(bookings) on another day = %d, want 0", len(other))
	}
}

func TestListRoomBookingsAlwaysReturnsASlice(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := fixture(t)

	bookings, err := svc.ListRoomBookings(context.Background(), usecase.ListBookingsQuery{
		RoomID: 1,
		Day:    testNow,
	})
	if err != nil {
		t.Fatalf("ListRoomBookings() returned an unexpected error: %v", err)
	}
	if bookings == nil {
		t.Error("bookings = nil, want an empty slice so the api renders [] instead of null")
	}
}

func TestNormalizePageAppliesTheConfiguredBounds(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := fixture(t)

	bookings, err := svc.CreateBooking(context.Background(), createCmd(1, "Ada", 10, 11))
	if err != nil {
		t.Fatalf("CreateBooking() returned an unexpected error: %v", err)
	}
	day := testNow.AddDate(0, 0, 1)

	page, err := svc.ListRoomBookings(context.Background(), usecase.ListBookingsQuery{
		RoomID: 1, Day: day, Limit: 1, Offset: 500,
	})
	if err != nil {
		t.Fatalf("ListRoomBookings() returned an unexpected error: %v", err)
	}
	if len(page) != 0 {
		t.Errorf("len(page) = %d, want 0", len(page))
	}

	all, err := svc.ListRoomBookings(context.Background(), usecase.ListBookingsQuery{RoomID: 1, Day: day})
	if err != nil {
		t.Fatalf("ListRoomBookings() returned an unexpected error: %v", err)
	}
	if len(all) != 1 || all[0].ID != bookings.ID {
		t.Errorf("page = %+v, want the single stored booking", all)
	}
}

func TestCancelBookingOwnership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		by        usecase.Principal
		wantError error
	}{
		{"owner", usecase.Principal{Subject: "ada", Authenticated: true}, nil},
		{"owner differing case", usecase.Principal{Subject: "ADA", Authenticated: true}, nil},
		{"display name match", usecase.Principal{Subject: "u-1", DisplayName: "Ada", Authenticated: true}, nil},
		{"admin", usecase.Principal{Subject: "root", Admin: true, Authenticated: true}, nil},
		{"anonymous", usecase.Anonymous, nil},
		{"another user", usecase.Principal{Subject: "bob", Authenticated: true}, domain.ErrForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc, _, _, _ := fixture(t)
			ctx := context.Background()

			created, err := svc.CreateBooking(ctx, createCmd(1, "Ada", 10, 11))
			if err != nil {
				t.Fatalf("CreateBooking() returned an unexpected error: %v", err)
			}
			cancelled, err := svc.CancelBooking(ctx, created.ID, tc.by)
			switch {
			case tc.wantError != nil && !errors.Is(err, tc.wantError):
				t.Fatalf("CancelBooking() error = %v, want %v", err, tc.wantError)
			case tc.wantError != nil && cancelled.Status == domain.BookingStatusCancelled:
				t.Error("booking was cancelled even though the caller was not allowed to")
			case tc.wantError == nil && err != nil:
				t.Fatalf("CancelBooking() returned an unexpected error: %v", err)
			case tc.wantError == nil && cancelled.Status != domain.BookingStatusCancelled:
				t.Errorf("Status = %q, want %q", cancelled.Status, domain.BookingStatusCancelled)
			}
		})
	}
}

func TestCancelBookingIsRejectedForAnUnknownID(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := fixture(t)

	if _, err := svc.CancelBooking(context.Background(), "missing", usecase.Anonymous); !errors.Is(err, domain.ErrBookingNotFound) {
		t.Fatalf("CancelBooking() error = %v, want ErrBookingNotFound", err)
	}
}

func TestCreateBookingOwnerResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		by        usecase.Principal
		requested string
		wantOwner string
		wantError error
	}{
		{"anonymous without a name", usecase.Anonymous, "", usecase.Anonymous.Subject, nil},
		{"anonymous with a name", usecase.Anonymous, "Ada", "Ada", nil},
		{"authenticated books for itself", usecase.Principal{Subject: "u-1", Authenticated: true}, "", "u-1", nil},
		{"authenticated repeats its own name", usecase.Principal{Subject: "Ada", Authenticated: true}, "Ada", "Ada", nil},
		{"authenticated claims somebody else", usecase.Principal{Subject: "mallory", Authenticated: true}, "Ada", "", domain.ErrForbidden},
		{"admin names somebody else", usecase.Principal{Subject: "root", Admin: true, Authenticated: true}, "Ada", "Ada", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc, _, _, _ := fixture(t)
			cmd := createCmd(1, tc.requested, 10, 11)
			cmd.By = tc.by

			created, err := svc.CreateBooking(context.Background(), cmd)
			if tc.wantError != nil {
				if !errors.Is(err, tc.wantError) {
					t.Fatalf("CreateBooking() error = %v, want %v", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateBooking() returned an unexpected error: %v", err)
			}
			if created.UserName != tc.wantOwner {
				t.Errorf("UserName = %q, want %q", created.UserName, tc.wantOwner)
			}
		})
	}
}

func TestListRoomsAndGetRoom(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := fixture(t)
	ctx := context.Background()

	rooms, err := svc.ListRooms(ctx)
	if err != nil {
		t.Fatalf("ListRooms() returned an unexpected error: %v", err)
	}
	if len(rooms) != 3 {
		t.Fatalf("len(rooms) = %d, want 3", len(rooms))
	}
	for i := 1; i < len(rooms); i++ {
		if rooms[i-1].ID > rooms[i].ID {
			t.Errorf("rooms are not ordered by id: %+v", rooms)
			break
		}
	}

	room, err := svc.GetRoom(ctx, 2)
	if err != nil {
		t.Fatalf("GetRoom() returned an unexpected error: %v", err)
	}
	if room.ID != 2 {
		t.Errorf("room = %+v, want room 2", room)
	}
	if _, err := svc.GetRoom(ctx, 99); !errors.Is(err, domain.ErrRoomNotFound) {
		t.Errorf("GetRoom() error = %v, want ErrRoomNotFound", err)
	}
}

func TestRoomsAreAlwaysReturnedAsASlice(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := fixture(t)

	rooms, err := svc.ListRooms(context.Background())
	if err != nil {
		t.Fatalf("ListRooms() returned an unexpected error: %v", err)
	}
	if rooms == nil {
		t.Error("rooms = nil, want an empty slice so the api renders [] instead of null")
	}
}
