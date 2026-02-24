package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	internaldb "github.com/mjhen/elnote/server/internal/db"
)

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
)

type Service struct {
	db     *sql.DB
	tokens *TokenManager
}

type LoginInput struct {
	Email      string
	Password   string
	DeviceName string
}

type TokenPair struct {
	TokenType             string    `json:"tokenType"`
	UserID                string    `json:"userId"`
	MustChangePassword    bool      `json:"mustChangePassword"`
	AccessToken           string    `json:"accessToken"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshToken          string    `json:"refreshToken"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
}

func NewService(db *sql.DB, tokens *TokenManager) *Service {
	return &Service{db: db, tokens: tokens}
}

func (s *Service) Login(ctx context.Context, in LoginInput) (TokenPair, error) {
	if strings.TrimSpace(in.Email) == "" || strings.TrimSpace(in.Password) == "" {
		return TokenPair{}, ErrInvalidCredentials
	}
	if strings.TrimSpace(in.DeviceName) == "" {
		return TokenPair{}, errors.New("deviceName is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TokenPair{}, fmt.Errorf("begin login tx: %w", err)
	}
	defer tx.Rollback()

	var userID, passwordHash, role string
	var mustChangePassword bool
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, password_hash, role, COALESCE(must_change_password, FALSE)
		FROM users
		WHERE email = $1
	`, strings.ToLower(strings.TrimSpace(in.Email))).Scan(&userID, &passwordHash, &role, &mustChangePassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TokenPair{}, ErrInvalidCredentials
		}
		return TokenPair{}, fmt.Errorf("lookup user: %w", err)
	}

	ok, err := VerifyPassword(passwordHash, in.Password)
	if err != nil {
		return TokenPair{}, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return TokenPair{}, ErrInvalidCredentials
	}

	refreshToken, refreshHash, refreshExpiresAt, err := s.tokens.IssueRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}

	deviceName := strings.TrimSpace(in.DeviceName)
	var deviceID string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text
		FROM devices
		WHERE user_id = $1 AND device_name = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, userID, deviceName).Scan(&deviceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return TokenPair{}, fmt.Errorf("query device session: %w", err)
	}

	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO devices (
				user_id,
				device_name,
				refresh_token_hash,
				refresh_token_expires_at
			) VALUES ($1, $2, $3, $4)
			RETURNING id::text
		`, userID, deviceName, refreshHash, refreshExpiresAt).Scan(&deviceID)
		if err != nil {
			return TokenPair{}, fmt.Errorf("insert device session: %w", err)
		}
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE devices
			SET refresh_token_hash = $1,
				refresh_token_expires_at = $2,
				revoked_at = NULL,
				updated_at = NOW()
			WHERE id = $3
		`, refreshHash, refreshExpiresAt, deviceID)
		if err != nil {
			return TokenPair{}, fmt.Errorf("update device session: %w", err)
		}
	}

	accessToken, accessExpiresAt, err := s.tokens.IssueAccessToken(userID, role, deviceID)
	if err != nil {
		return TokenPair{}, err
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, userID, "auth.login", "device", deviceID, map[string]any{
		"deviceName": in.DeviceName,
	}); err != nil {
		return TokenPair{}, err
	}

	if err := tx.Commit(); err != nil {
		return TokenPair{}, fmt.Errorf("commit login tx: %w", err)
	}

	return TokenPair{
		TokenType:             "Bearer",
		UserID:                userID,
		MustChangePassword:    mustChangePassword,
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return TokenPair{}, ErrInvalidRefreshToken
	}

	refreshHash := s.tokens.HashRefreshToken(refreshToken)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TokenPair{}, fmt.Errorf("begin refresh tx: %w", err)
	}
	defer tx.Rollback()

	var (
		deviceID   string
		userID     string
		role       string
		expiresAt  time.Time
		revokedAt  sql.NullTime
	)
	err = tx.QueryRowContext(ctx, `
		SELECT d.id::text, d.user_id::text, u.role, d.refresh_token_expires_at, d.revoked_at
		FROM devices d
		JOIN users u ON u.id = d.user_id
		WHERE d.refresh_token_hash = $1
	`, refreshHash).Scan(&deviceID, &userID, &role, &expiresAt, &revokedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TokenPair{}, ErrInvalidRefreshToken
		}
		return TokenPair{}, fmt.Errorf("lookup refresh token: %w", err)
	}

	if revokedAt.Valid || time.Now().UTC().After(expiresAt) {
		return TokenPair{}, ErrInvalidRefreshToken
	}

	newRefreshToken, newRefreshHash, newRefreshExpiresAt, err := s.tokens.IssueRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE devices
		SET refresh_token_hash = $1,
			refresh_token_expires_at = $2,
			updated_at = NOW()
		WHERE id = $3
	`, newRefreshHash, newRefreshExpiresAt, deviceID); err != nil {
		return TokenPair{}, fmt.Errorf("rotate refresh token: %w", err)
	}

	accessToken, accessExpiresAt, err := s.tokens.IssueAccessToken(userID, role, deviceID)
	if err != nil {
		return TokenPair{}, err
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, userID, "auth.refresh", "device", deviceID, map[string]any{}); err != nil {
		return TokenPair{}, err
	}

	if err := tx.Commit(); err != nil {
		return TokenPair{}, fmt.Errorf("commit refresh tx: %w", err)
	}

	return TokenPair{
		TokenType:             "Bearer",
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshToken:          newRefreshToken,
		RefreshTokenExpiresAt: newRefreshExpiresAt,
	}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil
	}

	refreshHash := s.tokens.HashRefreshToken(refreshToken)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin logout tx: %w", err)
	}
	defer tx.Rollback()

	var deviceID, userID string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, user_id::text
		FROM devices
		WHERE refresh_token_hash = $1
		  AND revoked_at IS NULL
	`, refreshHash).Scan(&deviceID, &userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("lookup logout session: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE devices
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, deviceID); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, userID, "auth.logout", "device", deviceID, map[string]any{}); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit logout tx: %w", err)
	}

	return nil
}
