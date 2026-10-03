package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

type fakeClock struct {
	mu    sync.Mutex
	times []time.Time
	last  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.times) > 0 {
		next := c.times[0]
		c.times = c.times[1:]
		c.last = next
		return next
	}
	return c.last
}

type fakeIDs struct {
	mu     sync.Mutex
	seq    int
	err    error
	issued []string
}

func (g *fakeIDs) NewID() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.err != nil {
		return "", g.err
	}
	g.seq++
	id := fmt.Sprintf("id-%03d", g.seq)
	g.issued = append(g.issued, id)
	return id, nil
}

type fakeCounter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newFakeCounter() *fakeCounter {
	return &fakeCounter{counts: make(map[string]int)}
}

func (c *fakeCounter) CountBooking(kind, outcome string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts[kind+":"+outcome]++
}

func (c *fakeCounter) get(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.counts[key]
}

type fakeBookingRepository struct {
	mu       sync.Mutex
	bookings map[domain.BookingID]domain.Booking

	createErr   error
	createCalls int
	manyCalls   int

	purgedBefore []time.Time
	purgeResult  int64
}

func newFakeBookingRepository() *fakeBookingRepository {
	return &fakeBookingRepository{bookings: make(map[domain.BookingID]domain.Booking)}
}

func (r *fakeBookingRepository) Create(ctx context.Context, booking domain.Booking) (domain.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.createCalls++
	if r.createErr != nil {
		return domain.Booking{}, r.createErr
	}
	return r.insertLocked(booking)
}
func (r *fakeBookingRepository) CreateMany(ctx context.Context, bookings []domain.Booking) ([]domain.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.manyCalls++
	if r.createErr != nil {
		return nil, r.createErr
	}

	inserted := make([]domain.Booking, 0, len(bookings))
	for _, booking := range bookings {
		created, err := r.insertLocked(booking)
		if err != nil {
			return nil, err
		}
		inserted = append(inserted, created)
	}
	return inserted, nil
}

func (r *fakeBookingRepository) insertLocked(booking domain.Booking) (domain.Booking, error) {
	if err := r.checkOverlapLocked(booking); err != nil {
		return domain.Booking{}, err
	}
	r.bookings[booking.ID] = booking
	return booking, nil
}
func (r *fakeBookingRepository) checkOverlapLocked(candidate domain.Booking) error {
	window := domain.NewTimeRange(candidate.StartTime, candidate.EndTime)
	for _, existing := range r.bookings {
		if existing.RoomID != candidate.RoomID || !existing.IsActive() {
			continue
		}
		if domain.NewTimeRange(existing.StartTime, existing.EndTime).Overlaps(window) {
			return fmt.Errorf("%w: constraint bookings_room_no_overlap", domain.ErrOverlap)
		}
	}
	return nil
}
func (r *fakeBookingRepository) GetByID(_ context.Context, id domain.BookingID) (domain.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	booking, ok := r.bookings[id]
	if !ok {
		return domain.Booking{}, domain.ErrBookingNotFound
	}
	return booking, nil
}
func (r *fakeBookingRepository) ListActiveByRoomAndRange(_ context.Context, roomID domain.RoomID, rng domain.TimeRange, limit, offset int) ([]domain.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var found []domain.Booking
	for _, booking := range r.bookings {
		if booking.RoomID != roomID || !booking.IsActive() {
			continue
		}
		if domain.NewTimeRange(booking.StartTime, booking.EndTime).Overlaps(rng) {
			found = append(found, booking)
		}
	}
	sortBookings(found)

	if offset > len(found) {
		offset = len(found)
	}
	found = found[offset:]
	if limit > 0 && limit < len(found) {
		found = found[:limit]
	}
	return found, nil
}
func (r *fakeBookingRepository) FindActiveOverlapping(ctx context.Context, roomID domain.RoomID, rng domain.TimeRange) ([]domain.Booking, error) {
	return r.ListActiveByRoomAndRange(ctx, roomID, rng, 1000, 0)
}
func (r *fakeBookingRepository) Cancel(_ context.Context, id domain.BookingID, at time.Time) (domain.Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	booking, ok := r.bookings[id]
	if !ok {
		return domain.Booking{}, domain.ErrBookingNotFound
	}
	if err := booking.Cancel(at); err != nil {
		return booking, err
	}
	r.bookings[id] = booking
	return booking, nil
}
func (r *fakeBookingRepository) PurgeCancelledBefore(_ context.Context, before time.Time, _ int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.purgedBefore = append(r.purgedBefore, before)
	return r.purgeResult, nil
}
func (r *fakeBookingRepository) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.bookings)
}

type fakeRoomRepository struct {
	mu    sync.Mutex
	rooms map[domain.RoomID]domain.Room
	err   error
}

func newFakeRoomRepository(ids ...domain.RoomID) *fakeRoomRepository {
	repo := &fakeRoomRepository{rooms: make(map[domain.RoomID]domain.Room)}
	for _, id := range ids {
		repo.rooms[id] = domain.Room{ID: id, Name: fmt.Sprintf("Room %d", id)}
	}
	return repo
}
func (r *fakeRoomRepository) GetByID(_ context.Context, id domain.RoomID) (domain.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.err != nil {
		return domain.Room{}, r.err
	}
	room, ok := r.rooms[id]
	if !ok {
		return domain.Room{}, domain.ErrRoomNotFound
	}
	return room, nil
}
func (r *fakeRoomRepository) List(context.Context) ([]domain.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.err != nil {
		return nil, r.err
	}
	out := make([]domain.Room, 0, len(r.rooms))
	for _, room := range r.rooms {
		out = append(out, room)
	}
	sortRooms(out)
	return out, nil
}

type fakeLogger struct {
	mu     sync.Mutex
	errors []error
}

func (l *fakeLogger) InfoContext(context.Context, string, ...any) {}
func (l *fakeLogger) WarnContext(_ context.Context, _ string, args ...any) {
	l.record(args)
}
func (l *fakeLogger) ErrorContext(_ context.Context, _ string, args ...any) {
	l.record(args)
}
func (l *fakeLogger) record(args []any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, arg := range args {
		if err, ok := arg.(error); ok {
			l.errors = append(l.errors, err)
		}
	}
}

func sortBookings(bookings []domain.Booking) {
	for i := 1; i < len(bookings); i++ {
		for j := i; j > 0 && bookings[j].StartTime.Before(bookings[j-1].StartTime); j-- {
			bookings[j], bookings[j-1] = bookings[j-1], bookings[j]
		}
	}
}

func sortRooms(rooms []domain.Room) {
	for i := 1; i < len(rooms); i++ {
		for j := i; j > 0 && rooms[j].ID < rooms[j-1].ID; j-- {
			rooms[j], rooms[j-1] = rooms[j-1], rooms[j]
		}
	}
}

var errRepository = errors.New("repository is down")
