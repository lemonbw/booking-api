CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE rooms (
    id         INTEGER PRIMARY KEY,
    name       TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE bookings (
    id           UUID        PRIMARY KEY,
    room_id      INTEGER     NOT NULL REFERENCES rooms (id) ON DELETE RESTRICT,
    user_name    TEXT        NOT NULL,
    start_time   TIMESTAMPTZ NOT NULL,
    end_time     TIMESTAMPTZ NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'active',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    cancelled_at TIMESTAMPTZ,

    CONSTRAINT bookings_status_check CHECK (status IN ('active', 'cancelled')),
    CONSTRAINT bookings_time_order     CHECK (start_time < end_time),
    CONSTRAINT bookings_user_name_len  CHECK (char_length(user_name) BETWEEN 1 AND 255),

    CONSTRAINT bookings_room_no_overlap
        EXCLUDE USING gist (
            room_id WITH =,
            tstzrange(start_time, end_time, '[)') WITH &&
        ) WHERE (status = 'active')
);

CREATE INDEX bookings_room_active_range_idx
    ON bookings (room_id, start_time)
    WHERE status = 'active';
