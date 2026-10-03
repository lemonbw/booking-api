package metrics

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCounterRendersEveryLabelCombination(t *testing.T) {
	registry := New()
	registry.ObserveRequest("/api/v1/bookings/{id}", "GET", 200, 3*time.Millisecond)
	registry.ObserveRequest("/api/v1/bookings/{id}", "GET", 200, 5*time.Millisecond)
	registry.ObserveRequest("/api/v1/bookings/{id}", "GET", 404, time.Millisecond)

	rendered := registry.Render()

	want := []string{
		"# HELP booking_api_http_requests_total Total number of HTTP requests handled, by route, method and status.",
		"# TYPE booking_api_http_requests_total counter",
		`booking_api_http_requests_total{method="GET",route="/api/v1/bookings/{id}",status="200"} 2`,
		`booking_api_http_requests_total{method="GET",route="/api/v1/bookings/{id}",status="404"} 1`,
	}
	for _, line := range want {
		if !strings.Contains(rendered, line) {
			t.Errorf("rendered output does not contain %q:\n%s", line, rendered)
		}
	}
}

func TestCountersAccumulatePerOutcome(t *testing.T) {
	registry := New()
	registry.CountBooking("created", "ok")
	registry.CountBooking("created", "conflict")
	registry.CountBooking("cancelled", "ok")
	registry.CountDatabaseError("40001")
	registry.CountRateLimited("/api/v1/bookings")

	rendered := registry.Render()

	for _, want := range []string{
		`booking_api_booking_operations_total{kind="created",outcome="ok"} 1`,
		`booking_api_booking_operations_total{kind="created",outcome="conflict"} 1`,
		`booking_api_booking_operations_total{kind="cancelled",outcome="ok"} 1`,
		`booking_api_database_errors_total{sqlstate="40001"} 1`,
		`booking_api_rate_limited_total{route="/api/v1/bookings"} 1`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered output does not contain %q:\n%s", want, rendered)
		}
	}
}

func TestHistogramCountsEveryBucketUpToTheSample(t *testing.T) {
	registry := New()
	registry.ObserveRequest("/api/v1/rooms", "GET", 200, 500*time.Microsecond)
	registry.ObserveRequest("/api/v1/rooms", "GET", 200, 30*time.Millisecond)

	rendered := registry.Render()

	want := []string{
		"# TYPE booking_api_http_request_duration_seconds histogram",
		`booking_api_http_request_duration_seconds_bucket{le="0.001",method="GET",route="/api/v1/rooms"} 1`,
		`booking_api_http_request_duration_seconds_bucket{le="0.025",method="GET",route="/api/v1/rooms"} 1`,
		`booking_api_http_request_duration_seconds_bucket{le="0.05",method="GET",route="/api/v1/rooms"} 2`,
		`booking_api_http_request_duration_seconds_bucket{le="5",method="GET",route="/api/v1/rooms"} 2`,
		`booking_api_http_request_duration_seconds_bucket{le="+Inf",method="GET",route="/api/v1/rooms"} 2`,
		`booking_api_http_request_duration_seconds_sum{method="GET",route="/api/v1/rooms"} 0.0305`,
		`booking_api_http_request_duration_seconds_count{method="GET",route="/api/v1/rooms"} 2`,
	}
	for _, line := range want {
		if !strings.Contains(rendered, line) {
			t.Errorf("rendered output does not contain %q:\n%s", line, rendered)
		}
	}
}

func TestHistogramSeparatesTheRouteAndMethodLabels(t *testing.T) {
	registry := New()
	registry.ObserveRequest("/api/v1/rooms", "GET", 200, time.Millisecond)
	registry.ObserveRequest("/api/v1/rooms", "POST", 201, 2*time.Millisecond)

	rendered := registry.Render()

	if got := strings.Count(rendered, "_count{"); got != 2 {
		t.Errorf("count samples = %d, want one per route and method", got)
	}
	if !strings.Contains(rendered, `_count{method="POST",route="/api/v1/rooms"} 1`) {
		t.Errorf("rendered output has no POST series:\n%s", rendered)
	}
}

func TestGaugeKeepsTheLastValue(t *testing.T) {
	registry := New()
	registry.SetBuildInfo("v0.6.0")
	registry.SetBuildInfo("v0.6.1")

	rendered := registry.Render()

	if !strings.Contains(rendered, `booking_api_build_info{version="v0.6.1"} 1`) {
		t.Errorf("rendered output does not carry the latest version:\n%s", rendered)
	}
	if strings.Contains(rendered, `booking_api_build_info{version="v0.6.0"}`) {
		t.Errorf("rendered output still carries the replaced version:\n%s", rendered)
	}
}

func TestLabelValuesAreEscapedExactlyOnce(t *testing.T) {
	cases := map[string]struct {
		value string
		want  string
	}{
		"plain":       {value: "/api/v1/rooms", want: `route="/api/v1/rooms"`},
		"quote":       {value: `a"b`, want: `route="a\"b"`},
		"backslash":   {value: `a\b`, want: `route="a\\b"`},
		"newline":     {value: "a\nb", want: `route="a\nb"`},
		"brace":       {value: "/api/v1/bookings/{id}", want: `route="/api/v1/bookings/{id}"`},
		"quote alone": {value: `"`, want: `route="\""`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			registry := New()
			registry.CountRateLimited(tc.value)

			rendered := registry.Render()

			if !strings.Contains(rendered, tc.want) {
				t.Errorf("rendered output does not contain %q:\n%s", tc.want, rendered)
			}
			if strings.Contains(rendered, `\\"`) {
				t.Errorf("rendered output escapes a quote twice:\n%s", rendered)
			}
		})
	}
}

func TestLabelValuesWithoutQuotesStayReadable(t *testing.T) {
	registry := New()
	registry.CountDatabaseError("23505")

	rendered := registry.Render()

	if !strings.Contains(rendered, `booking_api_database_errors_total{sqlstate="23505"} 1`) {
		t.Errorf("rendered output is not the plain sqlstate form:\n%s", rendered)
	}
}

func TestEveryFamilyRendersItsHeaderEvenWhenEmpty(t *testing.T) {
	rendered := New().Render()

	for _, want := range []string{
		"# HELP booking_api_http_requests_total ",
		"# TYPE booking_api_http_requests_total counter",
		"# TYPE booking_api_http_request_duration_seconds histogram",
		"# TYPE booking_api_booking_operations_total counter",
		"# TYPE booking_api_database_errors_total counter",
		"# TYPE booking_api_rate_limited_total counter",
		"# TYPE booking_api_build_info gauge",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered output does not contain %q:\n%s", want, rendered)
		}
	}
}

func TestRenderIsStableAcrossCalls(t *testing.T) {
	registry := New()
	for i := range 5 {
		registry.ObserveRequest("/api/v1/rooms", "GET", 200+2*i, time.Duration(i)*time.Millisecond)
		registry.CountBooking("created", "ok")
	}

	first := registry.Render()
	for range 5 {
		if got := registry.Render(); got != first {
			t.Fatalf("Render() is not stable:\nfirst:\n%s\ngot:\n%s", first, got)
		}
	}
}

func TestRegistryIsSafeForConcurrentUse(t *testing.T) {
	registry := New()

	var group sync.WaitGroup
	for worker := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			for i := range 50 {
				registry.ObserveRequest("/api/v1/rooms", "GET", 200, time.Duration(i)*time.Millisecond)
				registry.CountBooking("created", "ok")
				registry.CountDatabaseError("40001")
				if worker%2 == 0 {
					registry.SetBuildInfo("v0.6.1")
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			registry.Render()
		}
	}()

	group.Wait()
	<-done

	if got := strings.Count(registry.Render(), `booking_api_booking_operations_total{kind="created",outcome="ok"} 400`); got != 1 {
		t.Errorf("the counter did not end up at 400:\n%s", registry.Render())
	}
}
