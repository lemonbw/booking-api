package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/lemonbw/booking-api/internal/domain"
)

func TestTranslateErrorMapsConstraintsToDomainErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		code       string
		constraint string
		want       error
	}{
		{"no overlap", sqlStateExclusionViolation, constraintRoomNoOverlap, domain.ErrOverlap},
		{"room deleted", sqlStateForeignKeyViolation, constraintRoomForeignKey, domain.ErrRoomDeleted},
		{"time order", sqlStateCheckViolation, constraintTimeOrder, domain.ErrInvalidTimeRange},
		{"other check", sqlStateCheckViolation, "some_other_check", domain.ErrIntegrity},
		{"other exclusion", sqlStateExclusionViolation, "some_other_exclusion", domain.ErrIntegrity},
		{"other foreign key", sqlStateForeignKeyViolation, "other_fk", domain.ErrIntegrity},
		{"unique", sqlStateUniqueViolation, "bookings_pkey", domain.ErrIntegrity},
		{"serialization failure", sqlStateSerializationFail, "", domain.ErrUnavailable},
		{"deadlock", sqlStateDeadlockDetected, "", domain.ErrUnavailable},
		{"too many connections", sqlStateTooManyConnections, "", domain.ErrUnavailable},
		{"admin shutdown", sqlStateAdminShutdown, "", domain.ErrUnavailable},
		{"connection exception", "08006", "", domain.ErrUnavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := translateError(&pgconn.PgError{Code: tc.code, ConstraintName: tc.constraint})
			if !errors.Is(err, tc.want) {
				t.Fatalf("translateError(%s/%s) = %v, want %v", tc.code, tc.constraint, err, tc.want)
			}
		})
	}
}

func TestTranslateErrorMapsMalformedIdentifiers(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		message string
		want    error
	}{
		"malformed uuid":      {message: `invalid input syntax for type uuid: "b-missing"`, want: domain.ErrInvalidBookingID},
		"malformed timestamp": {message: `invalid input syntax for type timestamp with time zone: "tomorrow"`, want: domain.ErrInvalidTimeRange},
		"malformed date":      {message: `invalid input syntax for type date: "03.03.2026"`, want: domain.ErrInvalidTimeRange},
		"malformed number":    {message: `invalid input syntax for type integer: "seven"`, want: domain.ErrInvalidTimeRange},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := translateError(&pgconn.PgError{Code: sqlStateDataException, Message: tc.message})
			if !errors.Is(err, tc.want) {
				t.Fatalf("translateError(%q) = %v, want %v", tc.message, err, tc.want)
			}
		})
	}
}

func TestTranslateErrorRecognisesTransportFailures(t *testing.T) {
	t.Parallel()

	transport := []error{
		fmt.Errorf("dial tcp 127.0.0.1:5432: connection refused"),
		fmt.Errorf("write: broken pipe"),
		fmt.Errorf("conn closed"),
		errors.New("unexpected EOF"),
		fmt.Errorf("failed to connect to host=db"),
	}

	for _, err := range transport {
		if got := translateError(err); !errors.Is(got, domain.ErrUnavailable) {
			t.Errorf("translateError(%v) = %v, want ErrUnavailable", err, got)
		}
	}
}

func TestTranslateErrorPassesCancellationThrough(t *testing.T) {
	t.Parallel()

	err := translateError(context.Canceled)
	if errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("translateError(context.Canceled) = %v, want it to stay a cancellation", err)
	}
}

func TestTranslateErrorPassesUnknownSQLStateThrough(t *testing.T) {
	t.Parallel()

	original := &pgconn.PgError{Code: "42P01", Message: "relation does not exist"}
	if got := translateError(original); !errors.Is(got, original) {
		t.Fatalf("translateError() = %v, want the original error", got)
	}
}

func TestSQLStateClassTruncatesToTheClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"23505", "23"},
		{"08P01", "08"},
		{"40001", "40"},
		{"", "unknown"},
		{"0", "unknown"},
	}

	for _, tc := range tests {
		if got := sqlStateClass(tc.in); got != tc.want {
			t.Errorf("sqlStateClass(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMigrationNamesAreSorted(t *testing.T) {
	t.Parallel()

	names, err := migrationNames()
	if err != nil {
		t.Fatalf("migrationNames() returned an unexpected error: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("migrationNames() returned nothing, the migrations directory is not embedded")
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("migration names are not sorted: %q before %q", names[i-1], names[i])
		}
	}
}

func TestParseMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"", ModeApply, false},
		{"apply", ModeApply, false},
		{"APPLY", ModeApply, false},
		{" check ", ModeCheck, false},
		{"drop", "", true},
	}

	for _, tc := range tests {
		got, err := ParseMode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseMode(%q) returned nil error, want an error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMode(%q) returned an unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
