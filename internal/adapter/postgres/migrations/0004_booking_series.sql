ALTER TABLE bookings
    ADD COLUMN series_id UUID;

CREATE INDEX bookings_series_idx ON bookings (series_id) WHERE series_id IS NOT NULL;

CREATE INDEX bookings_cancelled_at_idx
    ON bookings (cancelled_at)
    WHERE status = 'cancelled' AND cancelled_at IS NOT NULL;
