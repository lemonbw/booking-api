package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lemonbw/booking-api/internal/usecase"
)

var ErrUnauthenticated = errors.New("authentication required")

type Authenticator interface {
	Authenticate(r *http.Request) (usecase.Principal, error)
}

type AuthConfig struct {
	Mode string

	JWTSecret string

	Issuer   string
	Audience string

	HeaderName   string
	HeaderSecret string
}

func AuthenticatorFromConfig(cfg AuthConfig) (Authenticator, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "", "none":
		return AnonymousAuthenticator{}, nil
	case "jwt":
		if len(cfg.JWTSecret) < 32 {
			return nil, errors.New("AUTH_MODE=jwt requires AUTH_JWT_SECRET of at least 32 characters")
		}
		return &JWTAuthenticator{
			secret:   []byte(cfg.JWTSecret),
			issuer:   cfg.Issuer,
			audience: cfg.Audience,
		}, nil
	case "header":
		if strings.TrimSpace(cfg.HeaderName) == "" {
			return nil, errors.New("AUTH_MODE=header requires AUTH_HEADER_NAME")
		}
		if len(cfg.HeaderSecret) < 32 {
			return nil, errors.New("AUTH_MODE=header requires AUTH_HEADER_SECRET of at least 32 characters")
		}
		return &HeaderAuthenticator{name: cfg.HeaderName, secret: cfg.HeaderSecret}, nil
	default:
		return nil, fmt.Errorf("unknown AUTH_MODE %q (expected none, jwt or header)", cfg.Mode)
	}
}

type AnonymousAuthenticator struct{}

func (AnonymousAuthenticator) Authenticate(*http.Request) (usecase.Principal, error) {
	return usecase.Anonymous, nil
}

type JWTAuthenticator struct {
	secret   []byte
	issuer   string
	audience string
}

type jwtClaims struct {
	Subject   string `json:"sub"`
	Expiry    int64  `json:"exp"`
	NotBefore int64  `json:"nbf"`
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	Name      string `json:"name"`
	Admin     bool   `json:"admin"`
}

func (a *JWTAuthenticator) Authenticate(r *http.Request) (usecase.Principal, error) {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		return usecase.Principal{}, ErrUnauthenticated
	}
	return a.verify(strings.TrimSpace(raw[len("bearer "):]))
}
func (a *JWTAuthenticator) verify(token string) (usecase.Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return usecase.Principal{}, ErrUnauthenticated
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return usecase.Principal{}, ErrUnauthenticated
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return usecase.Principal{}, ErrUnauthenticated
	}
	if !strings.EqualFold(header.Alg, "HS256") {
		return usecase.Principal{}, ErrUnauthenticated
	}
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(mac.Sum(nil), signature) {
		return usecase.Principal{}, ErrUnauthenticated
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return usecase.Principal{}, ErrUnauthenticated
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return usecase.Principal{}, ErrUnauthenticated
	}
	now := time.Now().UTC().Unix()
	if claims.Expiry == 0 || now >= claims.Expiry {
		return usecase.Principal{}, ErrUnauthenticated
	}
	if claims.NotBefore != 0 && now < claims.NotBefore {
		return usecase.Principal{}, ErrUnauthenticated
	}
	if a.issuer != "" && claims.Issuer != a.issuer {
		return usecase.Principal{}, ErrUnauthenticated
	}
	if a.audience != "" && !audienceMatches(claims.Audience, a.audience) {
		return usecase.Principal{}, ErrUnauthenticated
	}
	subject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		return usecase.Principal{}, ErrUnauthenticated
	}
	principal := usecase.Principal{
		Subject:       subject,
		DisplayName:   strings.TrimSpace(claims.Name),
		Admin:         claims.Admin,
		Authenticated: true,
	}
	if principal.DisplayName == "" {
		principal.DisplayName = subject
	}
	return principal, nil
}
func audienceMatches(aud, want string) bool {
	for _, candidate := range strings.FieldsFunc(aud, func(r rune) bool { return r == ' ' || r == ',' }) {
		if strings.TrimSpace(candidate) == want {
			return true
		}
	}
	return false
}

type HeaderAuthenticator struct {
	name   string
	secret string
}

func (a *HeaderAuthenticator) Authenticate(r *http.Request) (usecase.Principal, error) {
	subject := strings.TrimSpace(r.Header.Get(a.name))
	if subject == "" {
		return usecase.Principal{}, ErrUnauthenticated
	}
	provided := strings.TrimSpace(r.Header.Get("Authorization"))
	want := "Bearer " + a.secret
	if subtle.ConstantTimeCompare([]byte(provided), []byte(want)) != 1 {
		return usecase.Principal{}, ErrUnauthenticated
	}
	return usecase.Principal{
		Subject:       subject,
		DisplayName:   subject,
		Authenticated: true,
	}, nil
}
