package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	default:
		return nil, fmt.Errorf("unknown AUTH_MODE %q (expected none, jwt or header)", cfg.Mode)
	}
}

type AnonymousAuthenticator struct{}

func (AnonymousAuthenticator) Authenticate(*http.Request) (usecase.Principal, error) {
	return usecase.Anonymous, nil
}
