package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lemonbw/booking-api/internal/domain"
	"github.com/lemonbw/booking-api/internal/platform/idgen"
)

var testIDs = idgen.NewUUIDGenerator()

func nextID(t *testing.T) string {
	t.Helper()

	id, err := testIDs.NewID()
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	return id
}

func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Skipf("database is not reachable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("database is not reachable: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newTestBooking(t *testing.T, roomID domain.RoomID, start time.Time, user string) domain.Booking {
	t.Helper()

	booking, err := domain.NewBooking(domain.BookingID(nextID(t)), "", roomID, user, start, start.Add(time.Hour), start.Add(-time.Hour))
	if err != nil {
		t.Fatalf("NewBooking() error = %v", err)
	}
	return booking
}

func firstRoom(t *testing.T, ctx context.Context, pool *pgxpool.Pool) domain.RoomID {
	t.Helper()

	row := pool.QueryRow(ctx, `SELECT id FROM rooms ORDER BY id LIMIT 1`)
	var id int64
	if err := row.Scan(&id); err != nil {
		t.Skipf("no room in the catalogue: %v", err)
	}
	return domain.RoomID(id)
}

func TestBookingRepositoryStoresAndReadsABooking(t *testing.T) {
	pool := openTestDB(t)
	repo := NewBookingRepository(pool)
	ctx := context.Background()
	room := firstRoom(t, ctx, pool)
	booking := newTestBooking(t, room, time.Now().Add(48*time.Hour).Truncate(time.Second), "store")

	created, err := repo.Create(ctx, booking)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != booking.ID || created.UserName != "store" {
		t.Errorf("Create() = %+v, want the stored booking", created)
	}

	found, err := repo.GetByID(ctx, booking.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if found.ID != booking.ID || !found.StartTime.Equal(booking.StartTime.UTC()) {
		t.Errorf("GetByID() = %+v, want the stored booking", found)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM bookings WHERE id = $1`, booking.ID); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func TestCreateManyInsertsEveryOccurrenceInOrder(t *testing.T) {
	pool := openTestDB(t)
	repo := NewBookingRepository(pool)
	ctx := context.Background()
	room := firstRoom(t, ctx, pool)
	first := time.Now().Add(96 * time.Hour).Truncate(time.Second)

	seriesID := domain.SeriesID(nextID(t))
	bookings := make([]domain.Booking, 0, 12)
	for i := range 12 {
		booking := newTestBooking(t, room, first.Add(time.Duration(i)*24*time.Hour), "many")
		booking.SeriesID = seriesID
		bookings = append(bookings, booking)
	}

	inserted, err := repo.CreateMany(ctx, bookings)
	if err != nil {
		t.Fatalf("CreateMany() error = %v", err)
	}
	if len(inserted) != len(bookings) {
		t.Fatalf("CreateMany() returned %d rows, want %d", len(inserted), len(bookings))
	}
	for i, booking := range inserted {
		if booking.ID != bookings[i].ID {
			t.Errorf("row %d = %s, want %s in start order", i, booking.ID, bookings[i].ID)
		}
		if booking.SeriesID != seriesID {
			t.Errorf("row %d series = %q, want %q", i, booking.SeriesID, seriesID)
		}
	}

	var stored int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE series_id = $1`, string(seriesID)).Scan(&stored); err != nil {
		t.Fatalf("count series rows: %v", err)
	}
	if stored != len(bookings) {
		t.Errorf("stored rows = %d, want %d", stored, len(bookings))
	}

	if _, err := pool.Exec(ctx, `DELETE FROM bookings WHERE series_id = $1`, string(seriesID)); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func TestCreateManyRollsBackTheWholeRunOnConflict(t *testing.T) {
	pool := openTestDB(t)
	repo := NewBookingRepository(pool)
	ctx := context.Background()
	room := firstRoom(t, ctx, pool)
	start := time.Now().Add(120 * time.Hour).Truncate(time.Second)

	blocker := newTestBooking(t, room, start, "blocker")
	if _, err := repo.Create(ctx, blocker); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.WithoutCancel(ctx), `DELETE FROM bookings WHERE id = $1`, blocker.ID) })

	seriesID := domain.SeriesID(nextID(t))
	bookings := make([]domain.Booking, 0, 4)
	for i := range 4 {
		booking := newTestBooking(t, room, start.Add(time.Duration(i)*24*time.Hour), "clash")
		booking.SeriesID = seriesID
		bookings = append(bookings, booking)
	}

	if _, err := repo.CreateMany(ctx, bookings); !errors.Is(err, domain.ErrOverlap) {
		t.Fatalf("CreateMany() error = %v, want ErrOverlap", err)
	}

	var stored int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE series_id = $1`, string(seriesID)).Scan(&stored); err != nil {
		t.Fatalf("count series rows: %v", err)
	}
	if stored != 0 {
		t.Errorf("stored rows = %d, want the whole run rolled back", stored)
	}
}

func TestCreateManyOnAnEmptyRunTouchesNothing(t *testing.T) {
	pool := openTestDB(t)
	repo := NewBookingRepository(pool)

	inserted, err := repo.CreateMany(context.Background(), nil)
	if err != nil {
		t.Fatalf("CreateMany() error = %v", err)
	}
	if len(inserted) != 0 {
		t.Errorf("CreateMany() = %+v, want no rows", inserted)
	}
}

func TestConcurrentCreatesLeaveOneWinner(t *testing.T) {
	pool := openTestDB(t)
	repo := NewBookingRepository(pool)
	ctx := context.Background()
	room := firstRoom(t, ctx, pool)
	start := time.Now().Add(144 * time.Hour).Truncate(time.Second)
	user := "race-" + nextID(t)[:8]
	t.Cleanup(func() {
		_, _ = pool.Exec(context.WithoutCancel(ctx), `DELETE FROM bookings WHERE user_name = $1`, user)
	})

	var group sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	conflicts := 0

	for range 12 {
		group.Add(1)
		go func() {
			defer group.Done()
			booking := newTestBooking(t, room, start, user)
			if _, err := repo.Create(ctx, booking); err != nil {
				mu.Lock()
				if errors.Is(err, domain.ErrOverlap) {
					conflicts++
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			winners++
			mu.Unlock()
		}()
	}
	group.Wait()

	if winners != 1 {
		t.Errorf("winners = %d, want exactly 1", winners)
	}
	if conflicts != 11 {
		t.Errorf("conflicts = %d, want 11", conflicts)
	}
}

func TestCancelIsAtomicAndReportsTheOutcome(t *testing.T) {
	pool := openTestDB(t)
	repo := NewBookingRepository(pool)
	ctx := context.Background()
	room := firstRoom(t, ctx, pool)
	booking := newTestBooking(t, room, time.Now().Add(168*time.Hour).Truncate(time.Second), "cancel")
	if _, err := repo.Create(ctx, booking); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.WithoutCancel(ctx), `DELETE FROM bookings WHERE id = $1`, booking.ID) })

	now := time.Now().UTC()
	cancelled, err := repo.Cancel(ctx, booking.ID, now)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if cancelled.Status != domain.BookingStatusCancelled || cancelled.CancelledAt == nil {
		t.Errorf("Cancel() = %+v, want a cancelled booking with a stamp", cancelled)
	}

	again, err := repo.Cancel(ctx, booking.ID, now)
	if err != nil {
		t.Fatalf("second Cancel() error = %v", err)
	}
	if !again.CancelledAt.Equal(*cancelled.CancelledAt) {
		t.Errorf("second Cancel() moved the stamp to %s, want %s", again.CancelledAt, cancelled.CancelledAt)
	}

	started := newTestBooking(t, room, time.Now().Add(192*time.Hour).Truncate(time.Second), "cancel-started")
	if _, err := repo.Create(ctx, started); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.WithoutCancel(ctx), `DELETE FROM bookings WHERE id = $1`, started.ID) })

	_, err = repo.Cancel(ctx, started.ID, started.StartTime.Add(time.Hour))
	if err == nil {
		t.Fatal("Cancel() on a started booking = nil error, want a rejection")
	}
	if !errors.Is(err, domain.ErrBookingAlreadyStarted) {
		t.Errorf("Cancel() error = %v, want ErrBookingAlreadyStarted", err)
	}
}

func TestPurgeRemovesOnlyOldCancelledRows(t *testing.T) {
	pool := openTestDB(t)
	repo := NewBookingRepository(pool)
	ctx := context.Background()
	room := firstRoom(t, ctx, pool)
	user := "purge-" + nextID(t)[:8]
	t.Cleanup(func() {
		_, _ = pool.Exec(context.WithoutCancel(ctx), `DELETE FROM bookings WHERE user_name = $1`, user)
	})

	old := newTestBooking(t, room, time.Now().Add(216*time.Hour).Truncate(time.Second), user)
	if _, err := repo.Create(ctx, old); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE bookings SET status = 'cancelled', cancelled_at = $2 WHERE id = $1`, old.ID, time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatalf("age the booking: %v", err)
	}

	fresh := newTestBooking(t, room, time.Now().Add(240*time.Hour).Truncate(time.Second), user)
	if _, err := repo.Create(ctx, fresh); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	active := newTestBooking(t, room, time.Now().Add(264*time.Hour).Truncate(time.Second), user)
	if _, err := repo.Create(ctx, active); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	removed, err := repo.PurgeCancelledBefore(ctx, time.Now().Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatalf("PurgeCancelledBefore() error = %v", err)
	}
	if removed != 1 {
		t.Errorf("purged = %d, want 1", removed)
	}

	var survivors int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE user_name = $1`, user).Scan(&survivors); err != nil {
		t.Fatalf("count survivors: %v", err)
	}
	if survivors != 2 {
		t.Errorf("survivors = %d, want the fresh cancelled row and the active one", survivors)
	}
}

func TestDeletingARoomFailsTheBookingWithRoomDeleted(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()

	var roomID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO rooms (name) VALUES ('temp-' || $1) RETURNING id`, time.Now().Format("150405.000000000")).Scan(&roomID); err != nil {
		t.Skipf("cannot insert a room: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.WithoutCancel(ctx), `DELETE FROM bookings WHERE room_id = $1`, roomID)
	})

	if _, err := pool.Exec(ctx, `DELETE FROM rooms WHERE id = $1`, roomID); err != nil {
		t.Fatalf("delete the room: %v", err)
	}

	repo := NewBookingRepository(pool)
	booking := newTestBooking(t, domain.RoomID(roomID), time.Now().Add(288*time.Hour).Truncate(time.Second), "gone")
	_, err := repo.Create(ctx, booking)
	if !errors.Is(err, domain.ErrRoomDeleted) {
		t.Fatalf("Create() error = %v, want ErrRoomDeleted", err)
	}
}

func TestListingIsPagedAndCappedByTheQuery(t *testing.T) {
	pool := openTestDB(t)
	repo := NewBookingRepository(pool)
	ctx := context.Background()
	room := firstRoom(t, ctx, pool)
	user := "page-" + nextID(t)[:8]
	t.Cleanup(func() {
		_, _ = pool.Exec(context.WithoutCancel(ctx), `DELETE FROM bookings WHERE user_name = $1`, user)
	})

	day := time.Now().Add(48 * time.Hour).Truncate(24 * time.Hour)
	for i := range 5 {
		booking := newTestBooking(t, room, day.Add(time.Duration(i)*90*time.Minute), user)
		if _, err := repo.Create(ctx, booking); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	firstPage, err := repo.ListActiveByRoomAndRange(ctx, room, domain.DayBounds(day), 2, 0)
	if err != nil {
		t.Fatalf("ListActiveByRoomAndRange() error = %v", err)
	}
	if len(firstPage) != 2 {
		t.Fatalf("first page = %d rows, want 2", len(firstPage))
	}

	secondPage, err := repo.ListActiveByRoomAndRange(ctx, room, domain.DayBounds(day), 2, 2)
	if err != nil {
		t.Fatalf("ListActiveByRoomAndRange() error = %v", err)
	}
	if len(secondPage) != 2 {
		t.Fatalf("second page = %d rows, want 2", len(secondPage))
	}
	if firstPage[0].ID == secondPage[0].ID {
		t.Errorf("the second page repeats %s", secondPage[0].ID)
	}

	beyond, err := repo.ListActiveByRoomAndRange(ctx, room, domain.DayBounds(day), 2, 50)
	if err != nil {
		t.Fatalf("ListActiveByRoomAndRange() error = %v", err)
	}
	if len(beyond) != 0 {
		t.Errorf("offset past the end = %d rows, want 0", len(beyond))
	}
}

func TestGetByIDReportsAMissingBooking(t *testing.T) {
	pool := openTestDB(t)
	repo := NewBookingRepository(pool)

	_, err := repo.GetByID(context.Background(), domain.BookingID(nextID(t)))
	if !errors.Is(err, domain.ErrBookingNotFound) {
		t.Errorf("GetByID() error = %v, want ErrBookingNotFound", err)
	}
}
