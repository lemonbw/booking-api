package domain_test

import (
	"testing"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

func TestNewSeriesLaysOutOccurrences(t *testing.T) {
	t.Parallel()

	first := domain.NewTimeRange(validStart(), validStart().Add(time.Hour))
	series, err := domain.NewSeries("series-1", 3, "Ada", first, 24*time.Hour, 5, 52, referenceNow)
	if err != nil {
		t.Fatalf("NewSeries() returned an unexpected error: %v", err)
	}
	if series.Count != 5 || series.Step != 24*time.Hour {
		t.Errorf("Count = %d, Step = %s, want 5 and 24h", series.Count, series.Step)
	}

	occurrences := series.Occurrences()
	if len(occurrences) != 5 {
		t.Fatalf("len(Occurrences()) = %d, want 5", len(occurrences))
	}
	for i, occ := range occurrences {
		wantStart := first.Start.Add(time.Duration(i) * 24 * time.Hour)
		if !occ.Start.Equal(wantStart) {
			t.Errorf("occurrence %d starts at %s, want %s", i, occ.Start, wantStart)
		}
		if !occ.End.Equal(occ.Start.Add(time.Hour)) {
			t.Errorf("occurrence %d ends at %s, want %s", i, occ.End, occ.Start.Add(time.Hour))
		}
	}

	last := series.Last()
	if !last.End.Equal(series.Bounds().End) {
		t.Errorf("Last().End = %s, want Bounds().End = %s", last.End, series.Bounds().End)
	}
}

func TestNewSeriesRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	first := domain.NewTimeRange(validStart(), validStart().Add(time.Hour))

	tests := []struct {
		name      string
		id        domain.SeriesID
		roomID    domain.RoomID
		userName  string
		first     domain.TimeRange
		step      time.Duration
		count     int
		maxCount  int
		wantField string
	}{
		{"empty id", "", 1, "Ada", first, 24 * time.Hour, 3, 52, "series_id"},
		{"zero room", "s-1", 0, "Ada", first, 24 * time.Hour, 3, 52, "room_id"},
		{"blank user", "s-1", 1, " ", first, 24 * time.Hour, 3, 52, "user_name"},
		{"zero step", "s-1", 1, "Ada", first, 0, 3, 52, "interval"},
		{"negative step", "s-1", 1, "Ada", first, -time.Hour, 3, 52, "interval"},
		{"step shorter than the occurrence", "s-1", 1, "Ada", first, 30 * time.Minute, 3, 52, "interval"},
		{"zero count", "s-1", 1, "Ada", first, 24 * time.Hour, 0, 52, "count"},
		{"too many", "s-1", 1, "Ada", first, 24 * time.Hour, 53, 52, "count"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := domain.NewSeries(tc.id, tc.roomID, tc.userName, tc.first, tc.step, tc.count, tc.maxCount, referenceNow)
			if err == nil {
				t.Fatal("NewSeries() returned nil error, want a field error")
			}
			var fieldErr *domain.FieldError
			if !errorAs(err, &fieldErr) {
				t.Fatalf("error type = %T, want *domain.FieldError", err)
			}
			if fieldErr.Field != tc.wantField {
				t.Errorf("Field = %q, want %q", fieldErr.Field, tc.wantField)
			}
		})
	}
}

func TestNewSeriesRejectsASeriesThatStartsInThePast(t *testing.T) {
	t.Parallel()

	first := domain.NewTimeRange(referenceNow.Add(-2*time.Hour), referenceNow.Add(-time.Hour))
	_, err := domain.NewSeries("s-1", 1, "Ada", first, 24*time.Hour, 3, 52, referenceNow)
	if err == nil {
		t.Fatal("NewSeries() returned nil error, want an error for a series starting in the past")
	}
	var fieldErr *domain.FieldError
	if !errorAs(err, &fieldErr) || fieldErr.Field != "start_time" {
		t.Errorf("error = %v, want a field error on start_time", err)
	}
}
