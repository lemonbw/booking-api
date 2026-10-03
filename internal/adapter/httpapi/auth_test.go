package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testJWTSecret = "a-jwt-secret-that-is-long-enough-for-tests"

func TestAuthenticatorFromConfigPicksTheMode(t *testing.T) {
	cases := map[string]struct {
		cfg     AuthConfig
		wantErr bool
		check   func(Authenticator) bool
	}{
		"unset": {
			cfg:   AuthConfig{},
			check: func(a Authenticator) bool { _, ok := a.(AnonymousAuthenticator); return ok },
		},
		"none": {
			cfg:   AuthConfig{Mode: " none "},
			check: func(a Authenticator) bool { _, ok := a.(AnonymousAuthenticator); return ok },
		},
		"jwt": {
			cfg: AuthConfig{Mode: "jwt", JWTSecret: testJWTSecret},
			check: func(a Authenticator) bool {
				jwt, ok := a.(*JWTAuthenticator)
				return ok && string(jwt.secret) == testJWTSecret
			},
		},
		"header": {
			cfg: AuthConfig{Mode: "header", HeaderName: "X-User", HeaderSecret: testJWTSecret},
			check: func(a Authenticator) bool {
				hdr, ok := a.(*HeaderAuthenticator)
				return ok && hdr.name == "X-User" && hdr.secret == testJWTSecret
			},
		},
		"jwt without a secret": {
			cfg: AuthConfig{Mode: "jwt", JWTSecret: "short"},
		},
		"header without a name": {
			cfg: AuthConfig{Mode: "header", HeaderSecret: testJWTSecret},
		},
		"header without a secret": {
			cfg: AuthConfig{Mode: "header", HeaderName: "X-User"},
		},
		"unknown mode": {
			cfg: AuthConfig{Mode: "basic"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wantErr := tc.check == nil
			auth, err := AuthenticatorFromConfig(tc.cfg)
			if wantErr {
				if err == nil {
					t.Fatalf("AuthenticatorFromConfig(%+v) = nil error, want a rejection", tc.cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("AuthenticatorFromConfig(%+v) error = %v", tc.cfg, err)
			}
			if !tc.check(auth) {
				t.Errorf("AuthenticatorFromConfig(%+v) = %T, want the configured mode", tc.cfg, auth)
			}
		})
	}
}

func TestAnonymousAuthenticatorAcceptsEveryRequest(t *testing.T) {
	principal, err := AnonymousAuthenticator{}.Authenticate(httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil))
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.Authenticated {
		t.Errorf("principal = %+v, want an unauthenticated identity", principal)
	}
}

func TestJWTAuthenticatorAcceptsAValidToken(t *testing.T) {
	auth := newJWTAuthenticator(t, AuthConfig{JWTSecret: testJWTSecret, Issuer: "issuer", Audience: "clients"})
	token := signToken(t, testJWTSecret, jwtClaims{
		Subject:  "u-9",
		Name:     "Ada",
		Expiry:   time.Now().Add(time.Hour).Unix(),
		Issuer:   "issuer",
		Audience: "clients",
		Admin:    true,
	})

	principal, err := auth.Authenticate(bearerRequest(token))
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.Subject != "u-9" || principal.DisplayName != "Ada" {
		t.Errorf("principal = %+v, want the subject and the name from the token", principal)
	}
	if !principal.Admin || !principal.Authenticated {
		t.Errorf("principal = %+v, want an authenticated admin", principal)
	}
}

func TestJWTAuthenticatorFallsBackToTheSubjectAsName(t *testing.T) {
	auth := newJWTAuthenticator(t, AuthConfig{JWTSecret: testJWTSecret})
	token := signToken(t, testJWTSecret, jwtClaims{Subject: "u-9", Expiry: time.Now().Add(time.Hour).Unix()})

	principal, err := auth.Authenticate(bearerRequest(token))
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.DisplayName != "u-9" {
		t.Errorf("DisplayName = %q, want the subject", principal.DisplayName)
	}
}

func TestJWTAuthenticatorRejectsABadToken(t *testing.T) {
	valid := jwtClaims{Subject: "u-9", Expiry: time.Now().Add(time.Hour).Unix()}
	past := time.Now().Add(-time.Minute).Unix()
	future := time.Now().Add(time.Hour).Unix()

	cases := map[string]struct {
		token  string
		secret string
	}{
		"no header":        {token: ""},
		"not a bearer":     {token: "Basic " + signToken(t, testJWTSecret, valid)},
		"two segments":     {token: "a.b"},
		"garbage segments": {token: "not.base64.token"},
		"expired":          {token: signToken(t, testJWTSecret, jwtClaims{Subject: "u-9", Expiry: past})},
		"no expiry":        {token: signToken(t, testJWTSecret, jwtClaims{Subject: "u-9"})},
		"not yet valid":    {token: signToken(t, testJWTSecret, jwtClaims{Subject: "u-9", Expiry: future, NotBefore: future})},
		"no subject":       {token: signToken(t, testJWTSecret, jwtClaims{Expiry: future})},
		"blank subject":    {token: signToken(t, testJWTSecret, jwtClaims{Subject: "   ", Expiry: future})},
		"other secret":     {token: signToken(t, "another-secret-that-is-long-enough-x", valid)},
		"unsigned":         {token: unsignedToken(jwtClaims{Subject: "u-9", Expiry: future})},
		"none algorithm":   {token: signTokenWithAlg(t, testJWTSecret, "none", jwtClaims{Subject: "u-9", Expiry: future})},
		"tampered payload": {token: tamperPayload(t, signToken(t, testJWTSecret, valid))},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			auth := newJWTAuthenticator(t, AuthConfig{JWTSecret: testJWTSecret})
			if _, err := auth.Authenticate(bearerRequest(tc.token)); !errors.Is(err, ErrUnauthenticated) {
				t.Errorf("Authenticate() error = %v, want ErrUnauthenticated", err)
			}
		})
	}
}

func TestJWTAuthenticatorChecksTheIssuerAndAudience(t *testing.T) {
	cases := map[string]struct {
		cfg       AuthConfig
		issuer    string
		audience  string
		wantError bool
	}{
		"issuer matches":       {cfg: AuthConfig{Issuer: "issuer"}, issuer: "issuer"},
		"issuer differs":       {cfg: AuthConfig{Issuer: "issuer"}, issuer: "other", wantError: true},
		"issuer not demanded":  {cfg: AuthConfig{}, issuer: "anything"},
		"audience matches":     {cfg: AuthConfig{Audience: "clients"}, audience: "clients"},
		"audience in a list":   {cfg: AuthConfig{Audience: "clients"}, audience: "staff other clients"},
		"audience differs":     {cfg: AuthConfig{Audience: "clients"}, audience: "staff", wantError: true},
		"audience not claimed": {cfg: AuthConfig{Audience: "clients"}, audience: "", wantError: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.JWTSecret = testJWTSecret
			auth := newJWTAuthenticator(t, cfg)
			token := signToken(t, testJWTSecret, jwtClaims{
				Subject:  "u-9",
				Expiry:   time.Now().Add(time.Hour).Unix(),
				Issuer:   tc.issuer,
				Audience: tc.audience,
			})

			_, err := auth.Authenticate(bearerRequest(token))
			if tc.wantError {
				if !errors.Is(err, ErrUnauthenticated) {
					t.Errorf("Authenticate() error = %v, want ErrUnauthenticated", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Authenticate() error = %v, want the token accepted", err)
			}
		})
	}
}

func TestHeaderAuthenticatorNeedsTheNameAndTheSharedSecret(t *testing.T) {
	auth := &HeaderAuthenticator{name: "X-User", secret: testJWTSecret}

	cases := map[string]struct {
		name      string
		secret    string
		want      string
		wantError bool
	}{
		"both present":           {name: "ada", secret: "Bearer " + testJWTSecret, want: "ada"},
		"blank name":             {name: "   ", secret: "Bearer " + testJWTSecret, wantError: true},
		"no secret":              {name: "ada", wantError: true},
		"wrong secret":           {name: "ada", secret: "Bearer nope-nope-nope-nope-nope-1234", wantError: true},
		"secret in the raw form": {name: "ada", secret: testJWTSecret, wantError: true},
	}

	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
			request.Header.Set("X-User", tc.name)
			if tc.secret != "" {
				request.Header.Set("Authorization", tc.secret)
			}

			principal, err := auth.Authenticate(request)
			if tc.wantError {
				if !errors.Is(err, ErrUnauthenticated) {
					t.Fatalf("Authenticate() error = %v, want ErrUnauthenticated", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Authenticate() error = %v", err)
			}
			if principal.Subject != tc.want || !principal.Authenticated {
				t.Errorf("principal = %+v, want %s authenticated", principal, tc.want)
			}
			if principal.DisplayName != tc.want {
				t.Errorf("DisplayName = %q, want %q", principal.DisplayName, tc.want)
			}
		})
	}
}

func TestAudienceMatchesSplitsOnSpacesAndCommas(t *testing.T) {
	cases := map[string]struct {
		aud       string
		want      string
		wantMatch bool
	}{
		"single":     {aud: "clients", want: "clients", wantMatch: true},
		"space list": {aud: "staff clients", want: "clients", wantMatch: true},
		"comma list": {aud: "staff,clients", want: "clients", wantMatch: true},
		"mixed list": {aud: "staff, clients other", want: "clients", wantMatch: true},
		"absent":     {aud: "staff", want: "clients"},
		"empty":      {aud: "", want: "clients"},
		"substring":  {aud: "clients-eu", want: "clients"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := audienceMatches(tc.aud, tc.want); got != tc.wantMatch {
				t.Errorf("audienceMatches(%q, %q) = %t, want %t", tc.aud, tc.want, got, tc.wantMatch)
			}
		})
	}
}

func newJWTAuthenticator(t *testing.T, cfg AuthConfig) *JWTAuthenticator {
	t.Helper()

	auth, err := AuthenticatorFromConfig(AuthConfig{Mode: "jwt", JWTSecret: cfg.JWTSecret, Issuer: cfg.Issuer, Audience: cfg.Audience})
	if err != nil {
		t.Fatalf("AuthenticatorFromConfig() error = %v", err)
	}
	jwt, ok := auth.(*JWTAuthenticator)
	if !ok {
		t.Fatalf("AuthenticatorFromConfig() = %T, want *JWTAuthenticator", auth)
	}
	return jwt
}

func bearerRequest(token string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func signToken(t *testing.T, secret string, claims jwtClaims) string {
	t.Helper()
	return signTokenWithAlg(t, secret, "HS256", claims)
}

func signTokenWithAlg(t *testing.T, secret, alg string, claims jwtClaims) string {
	t.Helper()

	header, err := json.Marshal(map[string]string{"alg": alg, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	body := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func unsignedToken(claims jwtClaims) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		return header + "." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + "."
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

func tamperPayload(t *testing.T, token string) string {
	t.Helper()

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q is not a jwt", token)
	}
	forged, err := json.Marshal(jwtClaims{Subject: "root", Admin: true, Expiry: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return parts[0] + "." + base64.RawURLEncoding.EncodeToString(forged) + "." + parts[2]
}
