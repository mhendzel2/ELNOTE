package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/mjhen/elnote/server/internal/auth"
)

var (
	ErrMissingAuthorization = errors.New("missing authorization header")
	ErrInvalidAuthorization = errors.New("invalid authorization header")
)

type AccessTokenParser interface {
	ParseAccessToken(token string) (auth.AccessClaims, error)
}

type AuthUser struct {
	ID       string
	Role     string
	DeviceID string
}

func AuthenticateRequest(r *http.Request, parser AccessTokenParser) (AuthUser, error) {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" {
		return AuthUser{}, ErrMissingAuthorization
	}

	parts := strings.SplitN(raw, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return AuthUser{}, ErrInvalidAuthorization
	}

	claims, err := parser.ParseAccessToken(strings.TrimSpace(parts[1]))
	if err != nil {
		return AuthUser{}, err
	}

	return AuthUser{ID: claims.Sub, Role: claims.Role, DeviceID: claims.DeviceID}, nil
}
