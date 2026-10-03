package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lemonbw/booking-api/internal/domain"
)

func newTestRouter(t *testing.T, svc *stubUseCase, health HealthChecker, auth Authenticator, metrics Metrics) http.Handler {
	t.Helper()

	if auth == nil {
		auth = AnonymousAuthenticator{}
	}
	if health == nil {
		health = stubHealth{}
	}
	return NewRouter(newTestHandler(svc), health, auth, discardLogger(), metrics)
}

func TestRouterServesTheVersionedRoutes(t *testing.T) {
	svc := &stubUseCase{
		booking:        fixtureBooking(),
		rooms:          []domain.Room{{ID: 7, Name: "Red"}},
		seriesBookings: []domain.Booking{fixtureBooking()},
	}
	router := newTestRouter(t, svc, nil, nil, nil)

	cases := []struct {
		method string
		target string
		body   string
		status int
	}{
		{method: http.MethodPost, target: "/api/v1/bookings", body: createBookingBody, status: http.StatusCreated},
		{method: http.MethodPost, target: "/api/v1/bookings/series", body: `{"room_id":7,"user_name":"ada","start_time":"2026-03-03T09:00:00Z","end_time":"2026-03-03T10:00:00Z","interval":"24h","count":1}`, status: http.StatusCreated},
		{method: http.MethodGet, target: "/api/v1/bookings?room_id=7", status: http.StatusOK},
		{method: http.MethodGet, target: "/api/v1/bookings/b-1", status: http.StatusOK},
		{method: http.MethodDelete, target: "/api/v1/bookings/b-1", status: http.StatusOK},
		{method: http.MethodPost, target: "/api/v1/bookings/b-1/cancel", status: http.StatusOK},
		{method: http.MethodGet, target: "/api/v1/rooms", status: http.StatusOK},
		{method: http.MethodGet, target: "/api/v1/rooms/7", status: http.StatusOK},
	}

	for _, tc := range cases {
		rec := serveRouter(t, router, tc.method, tc.target, tc.body)
		if rec.Code != tc.status {
			t.Errorf("%s %s status = %d, want %d: %s", tc.method, tc.target, rec.Code, tc.status, rec.Body)
		}
	}
}

func TestRouterStripsTheVersionPrefix(t *testing.T) {
	svc := &stubUseCase{rooms: []domain.Room{{ID: 7, Name: "Red"}}}
	router := newTestRouter(t, svc, nil, nil, nil)

	rec := serveRouter(t, router, http.MethodGet, "/api/v1/rooms", "")

	if !strings.Contains(rec.Body.String(), `"name":"Red"`) {
		t.Errorf("body = %q, want the stored room", rec.Body.String())
	}
}

func TestRouterAnswersAnUnknownPathWithJson(t *testing.T) {
	router := newTestRouter(t, &stubUseCase{}, nil, nil, nil)

	cases := map[string]struct {
		method string
		target string
	}{
		"unknown api path":    {method: http.MethodGet, target: "/api/v1/unknown"},
		"unknown nested":      {method: http.MethodGet, target: "/api/v1/bookings/b-1/history"},
		"wrong api version":   {method: http.MethodGet, target: "/api/v2/rooms"},
		"unknown root path":   {method: http.MethodGet, target: "/nope"},
		"trailing slash root": {method: http.MethodGet, target: "/"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := serveRouter(t, router, tc.method, tc.target, "")

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body)
			}
			got := decodeError(t, rec)
			if got.Error != "not_found" {
				t.Errorf("body = %+v, want the not_found envelope", got)
			}
		})
	}
}

func TestRouterAnswersAWrongMethodWithJson(t *testing.T) {
	router := newTestRouter(t, &stubUseCase{}, nil, nil, &recordingMetrics{})

	cases := map[string]struct {
		method string
		target string
		allow  []string
	}{
		"put a booking":    {method: http.MethodPut, target: "/api/v1/bookings/b-1", allow: []string{http.MethodGet, http.MethodDelete}},
		"post to the list": {method: http.MethodPut, target: "/api/v1/bookings", allow: []string{http.MethodGet, http.MethodPost}},
		"put a room":       {method: http.MethodPut, target: "/api/v1/rooms/7", allow: []string{http.MethodGet}},
		"get a cancel":     {method: http.MethodGet, target: "/api/v1/bookings/b-1/cancel", allow: []string{http.MethodPost}},
		"post to a probe":  {method: http.MethodPost, target: "/healthz", allow: []string{http.MethodGet}},
		"post to metrics":  {method: http.MethodPost, target: "/metrics", allow: []string{http.MethodGet}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := serveRouter(t, router, tc.method, tc.target, "")

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusMethodNotAllowed, rec.Body)
			}
			got := decodeError(t, rec)
			if got.Error != "method_not_allowed" {
				t.Errorf("body = %+v, want the method_not_allowed envelope", got)
			}
			allow := rec.Header().Get("Allow")
			for _, method := range tc.allow {
				if !strings.Contains(allow, method) {
					t.Errorf("Allow = %q, want it to mention %s", allow, method)
				}
			}
		})
	}
}

func TestRouterServesTheProbes(t *testing.T) {
	cases := map[string]struct {
		health HealthChecker
		target string
		status int
		body   string
	}{
		"liveness":           {health: stubHealth{}, target: "/healthz", status: http.StatusOK, body: `"status":"ok"`},
		"readiness":          {health: stubHealth{}, target: "/readyz", status: http.StatusOK, body: `"status":"ready"`},
		"readiness degraded": {health: stubHealth{err: errors.New("connection refused")}, target: "/readyz", status: http.StatusServiceUnavailable, body: `"status":"unavailable"`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := newTestRouter(t, &stubUseCase{}, tc.health, nil, nil)

			rec := serveRouter(t, router, http.MethodGet, tc.target, "")

			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tc.body) {
				t.Errorf("body = %q, want %s", rec.Body.String(), tc.body)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q, want json", got)
			}
		})
	}
}

func TestRouterServesMetricsWithoutACredential(t *testing.T) {
	registry := &recordingMetrics{rendered: "booking_api_requests_total 1\n"}
	auth := newJWTAuthenticator(t, AuthConfig{JWTSecret: testJWTSecret})
	router := newTestRouter(t, &stubUseCase{}, nil, auth, registry)

	rec := serveRouter(t, router, http.MethodGet, "/metrics", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Errorf("Content-Type = %q, want the prometheus text format", got)
	}
	if rec.Body.String() != registry.rendered {
		t.Errorf("body = %q, want the rendered registry", rec.Body.String())
	}
}

func TestRouterHidesMetricsWithoutARegistry(t *testing.T) {
	router := newTestRouter(t, &stubUseCase{}, nil, nil, nil)

	rec := serveRouter(t, router, http.MethodGet, "/metrics", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body)
	}
}

func TestRouterGuardsTheApiWithTheConfiguredAuthenticator(t *testing.T) {
	auth := newJWTAuthenticator(t, AuthConfig{JWTSecret: testJWTSecret})
	router := newTestRouter(t, &stubUseCase{booking: fixtureBooking()}, nil, auth, nil)

	rec := serveRouter(t, router, http.MethodPost, "/api/v1/bookings", createBookingBody)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status without a token = %d, want %d: %s", rec.Code, http.StatusUnauthorized, rec.Body)
	}
	if got := decodeError(t, rec); got.Error != "unauthenticated" {
		t.Errorf("body = %+v, want unauthenticated", got)
	}

	token := signToken(t, testJWTSecret, jwtClaims{Subject: "u-9", Expiry: time.Now().Add(time.Hour).Unix()})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(createBookingBody))
	request.Header.Set("Authorization", "Bearer "+token)
	accepted := httptest.NewRecorder()
	router.ServeHTTP(accepted, request)

	if accepted.Code != http.StatusCreated {
		t.Errorf("status with a token = %d, want %d: %s", accepted.Code, http.StatusCreated, accepted.Body)
	}
}

func TestRouterStampsEveryResponseWithARequestID(t *testing.T) {
	router := newTestRouter(t, &stubUseCase{}, nil, nil, nil)

	rec := serveRouter(t, router, http.MethodGet, "/healthz", "")

	if got := rec.Header().Get(requestIDHeader); got == "" {
		t.Error("X-Request-Id is empty, want a generated id")
	}
}

func TestRouterPassesTheForwardedClientIPIntoTheMetrics(t *testing.T) {
	registry := &recordingMetrics{rendered: ""}
	router := newTestRouter(t, &stubUseCase{}, nil, nil, registry)

	serveRouter(t, router, http.MethodGet, "/api/v1/rooms", "")

	if len(registry.requests) != 1 {
		t.Fatalf("requests = %+v, want one record", registry.requests)
	}
	if got := registry.requests[0]; got.route != "/api/v1/rooms" || got.method != http.MethodGet || got.status != http.StatusOK {
		t.Errorf("request = %+v, want the rooms route with a 200", got)
	}
}

func serveRouter(t *testing.T, router http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	router.ServeHTTP(rec, httptest.NewRequest(method, target, reader))
	return rec
}
