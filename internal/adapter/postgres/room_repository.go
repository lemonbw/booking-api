package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/lemonbw/booking-api/internal/domain"
)

const roomColumns = `id, name`

type roomRow struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
}

func (r roomRow) toDomain() domain.Room {
	return domain.Room{ID: domain.RoomID(r.ID), Name: r.Name}
}
func queryOneRoom(ctx context.Context, db Querier, sql string, args ...any) (domain.Room, error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return domain.Room{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.Room{}, err
		}
		return domain.Room{}, pgx.ErrNoRows
	}
	row, err := pgx.RowToStructByNameLax[roomRow](rows)
	if err != nil {
		return domain.Room{}, err
	}
	rows.Close()
	return row.toDomain(), nil
}

type RoomRepository struct {
	db Querier
}

func NewRoomRepository(db Querier) *RoomRepository {
	return &RoomRepository{db: db}
}
func (r *RoomRepository) GetByID(ctx context.Context, id domain.RoomID) (domain.Room, error) {
	const q = `SELECT ` + roomColumns + ` FROM rooms WHERE id = $1`

	room, err := queryOneRoom(ctx, r.db, q, int64(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Room{}, domain.ErrRoomNotFound
		}
		return domain.Room{}, fmt.Errorf("get room %d: %w", id, translateError(err))
	}
	return room, nil
}
func (r *RoomRepository) List(ctx context.Context) ([]domain.Room, error) {
	const q = `SELECT ` + roomColumns + ` FROM rooms ORDER BY id ASC`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query rooms: %w", translateError(err))
	}
	defer rows.Close()

	rooms := make([]domain.Room, 0, 8)
	for rows.Next() {
		row, err := pgx.RowToStructByNameLax[roomRow](rows)
		if err != nil {
			return nil, fmt.Errorf("scan room: %w", err)
		}
		rooms = append(rooms, row.toDomain())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rooms: %w", err)
	}
	return rooms, nil
}
