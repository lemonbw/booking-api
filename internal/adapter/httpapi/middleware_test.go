package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lemonbw/booking-api/internal/usecase"
)

func TestRequestIDIsEchoedWhenItIsUsable(t *testing.T) {
	for _, id := range []string{"req-1", "0af7651916cd43dd8448eb211c80319c", "req.1/2=3"} {
		request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		request.Header.Set(requestIDHeader, id)
		rec := httptest.NewRecorder()

		requestIDMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, request)

		if got := rec.Header().Get(requestIDHeader); got != id {
			t.Errorf("X-Request-Id = %q, want %q", got, id)
		}
	}
}

func TestRequestIDIsReplacedWhenItCannotBeTrusted(t *testing.T) {
	cases := map[string]string{
		"absent":    "",
		"blank":     "   ",
		"space":     "req 1",
		"control":   "req\x00id",
		"non ascii": "запрос",
		"too long":  strings.Repeat("a", 129),
		"newline":   "req\n1",
	}

	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			if id != "" {
				request.Header.Set(requestIDHeader, id)
			}
			rec := httptest.NewRecorder()

			requestIDMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, request)

			got := rec.Header().Get(requestIDHeader)
			if got == "" {
				t.Fatal("X-Request-Id is empty, want a generated id")
			}
			if got == id && strings.TrimSpace(id) != "" {
				t.Errorf("X-Request-Id = %q, want it replaced", got)
			}
			if len(got) != 24 || !isPrintableASCII(got) {
				t.Errorf("X-Request-Id = %q, want 24 printable hex characters", got)
			}
		})
	}
}

func TestGeneratedRequestIDsDiffer(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for range 100 {
		id := newRequestID()
		if _, taken := seen[id]; taken {
			t.Fatalf("newRequestID() returned %q twice", id)
		}
		seen[id] = struct{}{}
	}
}

func TestTraceparentIsKeptOnlyWhenItIsWellFormed(t *testing.T) {
	valid := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	cases := map[string]struct {
		traceparent string
		want        string
	}{
		"valid":       {traceparent: valid, want: valid},
		"padded":      {traceparent: "  " + valid + "  ", want: valid},
		"too short":   {traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-0"},
		"too long":    {traceparent: valid + "0"},
		"upper case":  {traceparent: strings.ToUpper(valid)},
		"wrong flags": {traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-0z"},
		"short span":  {traceparent: "00-4bf92f3577b34da6a3ce929d0e0e473-00f067aa0ba902b7-01"},
		"no dashes":   {traceparent: strings.ReplaceAll(valid, "-", "")},
		"empty":       {},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var seen string
			handler := traceMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = TraceparentFromContext(r.Context())
			}), discardLogger())
			request := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
			if tc.traceparent != "" {
				request.Header.Set(traceparentHeader, tc.traceparent)
			}

			handler.ServeHTTP(httptest.NewRecorder(), request)

			if seen != tc.want {
				t.Errorf("TraceparentFromContext() = %q, want %q", seen, tc.want)
			}
		})
	}
}

func TestAuthMiddlewareRejectsAMissingCredential(t *testing.T) {
	auth := &HeaderAuthenticator{name: "X-User", secret: testJWTSecret}
	called := false
	handler := authMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), auth, discardLogger())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rooms", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusUnauthorized, rec.Body)
	}
	if got := decodeError(t, rec); got.Error != "unauthenticated" {
		t.Errorf("body = %+v, want unauthenticated", got)
	}
	if called {
		t.Error("the wrapped handler ran, want the request stopped")
	}
}

func TestAuthMiddlewarePassesThePrincipalDown(t *testing.T) {
	auth := newJWTAuthenticator(t, AuthConfig{JWTSecret: testJWTSecret})
	token := signToken(t, testJWTSecret, jwtClaims{Subject: "u-9", Expiry: time.Now().Add(time.Hour).Unix()})

	var principal string
	handler := authMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		principal = PrincipalFromContext(r.Context()).Subject
	}), auth, discardLogger())

	handler.ServeHTTP(httptest.NewRecorder(), bearerRequest(token))

	if principal != "u-9" {
		t.Errorf("principal = %q, want u-9", principal)
	}
}

func TestPrincipalFromContextIsAnonymousByDefault(t *testing.T) {
	if got := PrincipalFromContext(t.Context()); got != usecase.Anonymous {
		t.Errorf("PrincipalFromContext() = %+v, want the anonymous principal", got)
	}
}

func TestMetricsMiddlewareRecordsApiTrafficOnly(t *testing.T) {
	registry := &recordingMetrics{}
	handler := metricsMiddleware(registry)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	cases := []struct {
		target string
		route  string
		status int
	}{
		{target: "/api/v1/bookings/b-1", route: "/api/v1/bookings/{id}", status: http.StatusTeapot},
		{target: "/api/v1/rooms/7", route: "/api/v1/rooms/{roomID}", status: http.StatusTeapot},
		{target: "/api/v1/rooms", route: "/api/v1/rooms", status: http.StatusTeapot},
		{target: "/healthz", status: http.StatusTeapot},
		{target: "/metrics", status: http.StatusTeapot},
	}

	for _, tc := range cases {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.target, nil))
	}

	if len(registry.requests) != 3 {
		t.Fatalf("requests = %+v, want the three api paths only", registry.requests)
	}
	for i, want := range []struct {
		route  string
		status int
	}{{"/api/v1/bookings/{id}", http.StatusTeapot}, {"/api/v1/rooms/{roomID}", http.StatusTeapot}, {"/api/v1/rooms", http.StatusTeapot}} {
		if registry.requests[i].route != want.route {
			t.Errorf("request %d route = %q, want %q", i, registry.requests[i].route, want.route)
		}
		if registry.requests[i].status != want.status {
			t.Errorf("request %d status = %d, want %d", i, registry.requests[i].status, want.status)
		}
		if registry.requests[i].method != http.MethodGet {
			t.Errorf("request %d method = %q, want GET", i, registry.requests[i].method)
		}
	}
}

func TestMetricsMiddlewareIsSkippedWithoutARegistry(t *testing.T) {
	handler := metricsMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestStatusRecorderCountsTheFirstStatusAndTheBytes(t *testing.T) {
	cases := map[string]struct {
		write      func(http.ResponseWriter)
		wantStatus int
		wantBytes  int
	}{
		"nothing written": {
			write:      func(http.ResponseWriter) {},
			wantStatus: http.StatusOK,
		},
		"body without a status": {
			write:      func(w http.ResponseWriter) { _, _ = w.Write([]byte("hello")) },
			wantStatus: http.StatusOK,
			wantBytes:  5,
		},
		"status then body": {
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte("hi"))
			},
			wantStatus: http.StatusCreated,
			wantBytes:  2,
		},
		"status twice keeps the first": {
			write: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusCreated)
				w.WriteHeader(http.StatusTeapot)
			},
			wantStatus: http.StatusCreated,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
			tc.write(rec)

			if got := statusOf(rec); got != tc.wantStatus {
				t.Errorf("statusOf() = %d, want %d", got, tc.wantStatus)
			}
			if rec.bytes != tc.wantBytes {
				t.Errorf("bytes = %d, want %d", rec.bytes, tc.wantBytes)
			}
		})
	}
}

func TestRoutePatternKeepsTheCardinalityLow(t *testing.T) {
	cases := map[string]string{
		"/api/v1/bookings":             "/api/v1/bookings",
		"/api/v1/bookings/series":      "/api/v1/bookings/series",
		"/api/v1/bookings/9f1c":        "/api/v1/bookings/{id}",
		"/api/v1/bookings/9f1c/cancel": "/api/v1/bookings/{id}",
		"/api/v1/rooms":                "/api/v1/rooms",
		"/api/v1/rooms/7":              "/api/v1/rooms/{roomID}",
		"/api/v1/rooms/7/bookings":     "/api/v1/rooms/{roomID}",
		"/api/v2/bookings":             routeOther,
		"/healthz":                     "/healthz",
		"/readyz":                      "/readyz",
		"/metrics":                     "/metrics",
		"/":                            routeOther,
	}

	for path, want := range cases {
		if got := routePatternFor(path); got != want {
			t.Errorf("routePatternFor(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestClientIPPrefersTheForwardedChain(t *testing.T) {
	cases := map[string]struct {
		forwarded string
		remote    string
		want      string
	}{
		"first hop":      {forwarded: "203.0.113.7, 10.0.0.1", remote: "10.0.0.9:1234", want: "203.0.113.7"},
		"single hop":     {forwarded: "203.0.113.7", remote: "10.0.0.9:1234", want: "203.0.113.7"},
		"padded":         {forwarded: "  203.0.113.7 , 10.0.0.1", remote: "10.0.0.9:1234", want: "203.0.113.7"},
		"direct request": {remote: "10.0.0.9:1234", want: "10.0.0.9"},
		"bare host":      {remote: "10.0.0.9", want: "10.0.0.9"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
			request.RemoteAddr = tc.remote
			if tc.forwarded != "" {
				request.Header.Set("X-Forwarded-For", tc.forwarded)
			}

			if got := clientIP(request); got != tc.want {
				t.Errorf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAccessLogMiddlewareWritesOneLine(t *testing.T) {
	var sink strings.Builder
	handler := requestIDMiddleware(accessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}), newTextLogger(&sink)))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/b-1/cancel", nil)
	request.Header.Set(requestIDHeader, "req-1")
	request.RemoteAddr = "10.0.0.9:1234"
	handler.ServeHTTP(httptest.NewRecorder(), request)

	line := sink.String()
	for _, want := range []string{"method=POST", "status=202", "bytes=0", "request_id=req-1", "remote_addr=10.0.0.9", "route=/api/v1/bookings/{id}"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line %q does not mention %q", line, want)
		}
	}
	if strings.Count(line, "\n") != 1 {
		t.Errorf("log line %q, want exactly one record", line)
	}
}

func TestAccessLogMiddlewareIncludesTheTraceID(t *testing.T) {
	var sink strings.Builder
	handler := traceMiddleware(accessLogMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), newTextLogger(&sink)), discardLogger())

	request := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	request.Header.Set(traceparentHeader, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if want := "trace_id=4bf92f3577b34da6a3ce929d0e0e4736"; !strings.Contains(sink.String(), want) {
		t.Errorf("log line %q does not mention %q", sink.String(), want)
	}
}

func TestIsPrintableASCIIRejectsBlanks(t *testing.T) {
	cases := map[string]bool{
		"req-1":  true,
		"":       false,
		" ":      false,
		"a\tb":   false,
		"req\n1": false,
		"ok-._~": true,
		"запрос": false,
	}

	for value, want := range cases {
		if got := isPrintableASCII(value); got != want {
			t.Errorf("isPrintableASCII(%q) = %t, want %t", value, got, want)
		}
	}
}

func TestAnUnexpectedAuthenticatorErrorStillAnswersUnauthorized(t *testing.T) {
	broken := authenticatorFunc(func(*http.Request) (usecase.Principal, error) {
		return usecase.Anonymous, errors.New("key store unavailable")
	})

	handler := authMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the wrapped handler ran, want the request stopped")
	}), broken, discardLogger())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rooms", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

type authenticatorFunc func(*http.Request) (usecase.Principal, error)

func (f authenticatorFunc) Authenticate(r *http.Request) (usecase.Principal, error) { return f(r) }
