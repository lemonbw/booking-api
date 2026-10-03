package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/lemonbw/booking-api/internal/domain"
)

const bookingColumns = `
	id::text        AS id,
	series_id::text AS series_id,
	room_id,
	user_name,
	start_time,
	end_time,
	status,
	created_at,
	updated_at,
	cancelled_at`

type bookingRow struct {
	ID          string     `db:"id"`
	SeriesID    *string    `db:"series_id"`
	RoomID      int64      `db:"room_id"`
	UserName    string     `db:"user_name"`
	StartTime   time.Time  `db:"start_time"`
	EndTime     time.Time  `db:"end_time"`
	Status      string     `db:"status"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
	CancelledAt *time.Time `db:"cancelled_at"`

	Outcome string `db:"outcome"`
}

func (r bookingRow) toDomain() domain.Booking {
	booking := domain.Booking{
		ID:        domain.BookingID(r.ID),
		RoomID:    domain.RoomID(r.RoomID),
		UserName:  r.UserName,
		StartTime: r.StartTime.UTC(),
		EndTime:   r.EndTime.UTC(),
		Status:    domain.BookingStatus(r.Status),
		CreatedAt: r.CreatedAt.UTC(),
		UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.SeriesID != nil {
		booking.SeriesID = domain.SeriesID(*r.SeriesID)
	}
	if r.CancelledAt != nil {
		utc := r.CancelledAt.UTC()
		booking.CancelledAt = &utc
	}
	return booking
}
func queryOne(ctx context.Context, db Querier, sql string, args ...any) (bookingRow, error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return bookingRow{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return bookingRow{}, err
		}
		return bookingRow{}, pgx.ErrNoRows
	}
	row, err := pgx.RowToStructByNameLax[bookingRow](rows)
	if err != nil {
		return bookingRow{}, err
	}
	rows.Close()
	return row, nil
}
func collectBookings(rows pgx.Rows) ([]domain.Booking, error) {
	defer rows.Close()

	bookings := make([]domain.Booking, 0, 16)
	for rows.Next() {
		row, err := pgx.RowToStructByNameLax[bookingRow](rows)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, row.toDomain())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bookings, nil
}

type DB interface {
	Querier
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}
type BookingRepository struct {
	db DB
}

func NewBookingRepository(db DB) *BookingRepository {
	return &BookingRepository{db: db}
}
func (r *BookingRepository) Create(ctx context.Context, booking domain.Booking) (domain.Booking, error) {
	const q = `
		INSERT INTO bookings (id, room_id, user_name, start_time, end_time, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		RETURNING ` + bookingColumns

	row, err := queryOne(ctx, r.db, q,
		booking.ID,
		int64(booking.RoomID),
		booking.UserName,
		booking.StartTime,
		booking.EndTime,
		string(booking.Status),
		booking.CreatedAt,
	)
	if err != nil {
		return domain.Booking{}, fmt.Errorf("create booking %s: %w", booking.ID, translateError(err))
	}
	return row.toDomain(), nil
}

func nullableSeriesID(id domain.SeriesID) any {
	if id == "" {
		return nil
	}
	return string(id)
}
func (r *BookingRepository) CreateMany(ctx context.Context, bookings []domain.Booking) ([]domain.Booking, error) {
	if len(bookings) == 0 {
		return []domain.Booking{}, nil
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", translateError(err))
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	inserted := make([]domain.Booking, 0, len(bookings))
	for _, booking := range bookings {
		const q = `
			INSERT INTO bookings (id, series_id, room_id, user_name, start_time, end_time, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
			RETURNING ` + bookingColumns

		row, err := queryOne(ctx, tx, q,
			booking.ID,
			nullableSeriesID(booking.SeriesID),
			int64(booking.RoomID),
			booking.UserName,
			booking.StartTime,
			booking.EndTime,
			string(booking.Status),
			booking.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert booking %s: %w", booking.ID, translateError(err))
		}
		inserted = append(inserted, row.toDomain())
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit bookings: %w", translateError(err))
	}
	return inserted, nil
}
func (r *BookingRepository) GetByID(ctx context.Context, id domain.BookingID) (domain.Booking, error) {
	const q = `SELECT ` + bookingColumns + ` FROM bookings WHERE id = $1`

	row, err := queryOne(ctx, r.db, q, string(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Booking{}, domain.ErrBookingNotFound
		}
		return domain.Booking{}, fmt.Errorf("get booking %s: %w", id, translateError(err))
	}
	return row.toDomain(), nil
}
func (r *BookingRepository) ListActiveByRoomAndRange(ctx context.Context, roomID domain.RoomID, rng domain.TimeRange, limit, offset int) ([]domain.Booking, error) {
	const q = `
		SELECT ` + bookingColumns + `
		FROM bookings
		WHERE room_id = $1
		  AND status = 'active'
		  AND start_time < $3
		  AND end_time   > $2
		ORDER BY start_time ASC, id ASC
		LIMIT $4 OFFSET $5`

	rows, err := r.db.Query(ctx, q, int64(roomID), rng.Start, rng.End, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query bookings: %w", translateError(err))
	}
	bookings, err := collectBookings(rows)
	if err != nil {
		return nil, fmt.Errorf("collect bookings: %w", translateError(err))
	}
	return bookings, nil
}

const overlappingScanLimit = 1000

func (r *BookingRepository) FindActiveOverlapping(ctx context.Context, roomID domain.RoomID, rng domain.TimeRange) ([]domain.Booking, error) {
	return r.ListActiveByRoomAndRange(ctx, roomID, rng, overlappingScanLimit, 0)
}

func (r *BookingRepository) Cancel(ctx context.Context, id domain.BookingID, at time.Time) (domain.Booking, error) {
	const q = `
		WITH target AS (
			SELECT id, status, start_time FROM bookings WHERE id = $1
		),
		updated AS (
			UPDATE bookings b
			SET status = 'cancelled', cancelled_at = $2, updated_at = $2
			FROM target t
			WHERE b.id = t.id AND t.status = 'active' AND t.start_time > $2
			RETURNING 'cancelled'::text AS outcome, b.id::text AS id, b.room_id, b.user_name,
			          b.start_time, b.end_time, b.status, b.created_at, b.updated_at, b.cancelled_at
		)
		SELECT * FROM updated
		UNION ALL
		SELECT 'already_cancelled'::text, b.id::text, b.room_id, b.user_name,
		       b.start_time, b.end_time, b.status, b.created_at, b.updated_at, b.cancelled_at
		FROM bookings b JOIN target t ON b.id = t.id
		WHERE t.status = 'cancelled'
		UNION ALL
		SELECT 'already_started'::text, b.id::text, b.room_id, b.user_name,
		       b.start_time, b.end_time, b.status, b.created_at, b.updated_at, b.cancelled_at
		FROM bookings b JOIN target t ON b.id = t.id
		WHERE t.status = 'active' AND t.start_time <= $2
		LIMIT 1`

	row, err := queryOne(ctx, r.db, q, string(id), at)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Booking{}, domain.ErrBookingNotFound
		}
		return domain.Booking{}, fmt.Errorf("cancel booking %s: %w", id, translateError(err))
	}
	booking := row.toDomain()
	switch row.Outcome {
	case cancelOutcomeAlreadyStarted:
		return booking, domain.NewFieldError("start_time", domain.ErrBookingAlreadyStarted)
	default:
		return booking, nil
	}
}

const (
	cancelOutcomeCancelled      = "cancelled"
	cancelOutcomeAlreadyStarted = "already_started"
)

func (r *BookingRepository) PurgeCancelledBefore(ctx context.Context, before time.Time, batch int) (int64, error) {
	const q = `
		WITH victims AS (
			SELECT id
			FROM bookings
			WHERE status = 'cancelled' AND cancelled_at < $1
			ORDER BY cancelled_at
			LIMIT $2
		)
		DELETE FROM bookings b USING victims
		WHERE b.id = victims.id`

	tag, err := r.db.Exec(ctx, q, before, batch)
	if err != nil {
		return 0, fmt.Errorf("purge cancelled bookings: %w", translateError(err))
	}
	return tag.RowsAffected(), nil
}
