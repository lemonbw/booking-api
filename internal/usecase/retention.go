package usecase

import (
	"context"
	"errors"
	"time"
)

type PurgeRetention struct {
	Retention time.Duration
	Interval  time.Duration
	Batch     int
}

func (s *BookingService) StartRetentionSweep(ctx context.Context, p PurgeRetention) {
	if p.Retention <= 0 || p.Interval <= 0 || p.Batch <= 0 {
		s.log.InfoContext(ctx, "retention sweep is disabled", "retention", p.Retention.String())
		return
	}
	go func() {
		ticker := time.NewTicker(p.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.log.InfoContext(ctx, "retention sweep stopped")
				return
			case <-ticker.C:
				s.runPurge(ctx, p)
			}
		}
	}()
}

func (s *BookingService) runPurge(ctx context.Context, p PurgeRetention) {
	purgeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.Interval)
	defer cancel()

	cutoff := s.clock.Now().UTC().Add(-p.Retention)
	removed, err := s.bookings.PurgeCancelledBefore(purgeCtx, cutoff, p.Batch)
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		s.log.WarnContext(ctx, "retention sweep timed out")
	case err != nil:
		s.log.ErrorContext(ctx, "retention sweep failed", "error", err)
	case removed > 0:
		s.log.InfoContext(ctx, "retention sweep removed cancelled bookings",
			"removed", removed,
			"cutoff", cutoff.Format(time.RFC3339),
		)
	}
}
