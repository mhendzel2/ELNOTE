package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mjhen/elnote/server/internal/auth"
	internaldb "github.com/mjhen/elnote/server/internal/db"
)

var (
	ErrForbidden      = errors.New("forbidden")
	ErrNotFound       = errors.New("not found")
	ErrInvalidInput   = errors.New("invalid input")
	ErrConflict       = errors.New("conflict")
	ErrDuplicateEmail = errors.New("duplicate email")
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type User struct {
	ID             string    `json:"userId"`
	Email          string    `json:"email"`
	Role           string    `json:"role"`
	IsDefaultAdmin bool      `json:"isDefaultAdmin"`
	MustChangePassword bool   `json:"mustChangePassword"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type AccountRequest struct {
	ID          string     `json:"requestId"`
	RequestType string     `json:"requestType"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Note        string     `json:"note,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	FulfilledAt *time.Time `json:"fulfilledAt,omitempty"`
}

type CreateUserInput struct {
	AdminUserID string
	Email       string
	Password    string
	Role        string
}

type UpdateUserInput struct {
	AdminUserID string
	TargetID    string
	Role        string // optional - empty means no change
}

type ChangePasswordInput struct {
	UserID      string
	OldPassword string
	NewPassword string
}

type CreateAccountRequestInput struct {
	RequestType string
	Username    string
	Email       string
	Note        string
}

type ListAccountRequestsInput struct {
	Status string
	Limit  int
}

type ApproveAccountRequestInput struct {
	RequestID          string
	AdminUserID        string
	Role               string
	TemporaryPassword  string
}

type DismissAccountRequestInput struct {
	RequestID   string
	AdminUserID string
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CreateUser(ctx context.Context, in CreateUserInput) (*User, error) {
	if strings.TrimSpace(in.Email) == "" {
		return nil, fmt.Errorf("%w: email is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Password) == "" {
		return nil, fmt.Errorf("%w: password is required", ErrInvalidInput)
	}
	validRoles := map[string]bool{"owner": true, "admin": true, "author": true, "viewer": true}
	if !validRoles[in.Role] {
		return nil, fmt.Errorf("%w: role must be owner, admin, author, or viewer", ErrInvalidInput)
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var user User
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, role, must_change_password)
		 VALUES ($1, $2, $3, TRUE)
		 RETURNING id, email, role, COALESCE(is_default_admin, FALSE), COALESCE(must_change_password, FALSE), created_at, updated_at`,
		strings.ToLower(strings.TrimSpace(in.Email)), hash, in.Role,
	).Scan(&user.ID, &user.Email, &user.Role, &user.IsDefaultAdmin, &user.MustChangePassword, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return nil, fmt.Errorf("%w: email already exists", ErrConflict)
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"userId": user.ID,
		"email":  user.Email,
		"role":   user.Role,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.AdminUserID, "user.created", "user", user.ID, payload)

	_, _ = tx.ExecContext(ctx,
		`UPDATE account_requests
		 SET status = 'fulfilled', fulfilled_by_user_id = $1, fulfilled_at = NOW(), updated_at = NOW()
		 WHERE LOWER(email) = LOWER($2) AND status = 'pending'`,
		in.AdminUserID, user.Email,
	)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &user, nil
}

func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, role, COALESCE(is_default_admin, FALSE), COALESCE(must_change_password, FALSE), created_at, updated_at FROM users ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.IsDefaultAdmin, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if users == nil {
		users = []User{}
	}
	return users, nil
}

func (s *Service) GetUser(ctx context.Context, userID string) (*User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, role, COALESCE(is_default_admin, FALSE), COALESCE(must_change_password, FALSE), created_at, updated_at FROM users WHERE id = $1`,
		userID,
	).Scan(&u.ID, &u.Email, &u.Role, &u.IsDefaultAdmin, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	return &u, nil
}

func (s *Service) UpdateUser(ctx context.Context, in UpdateUserInput) (*User, error) {
	if in.Role != "" {
		validRoles := map[string]bool{"owner": true, "admin": true, "author": true, "viewer": true}
		if !validRoles[in.Role] {
			return nil, fmt.Errorf("%w: role must be owner, admin, author, or viewer", ErrInvalidInput)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var u User
	if in.Role != "" {
		err = tx.QueryRowContext(ctx,
			`UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2
			 RETURNING id, email, role, COALESCE(is_default_admin, FALSE), COALESCE(must_change_password, FALSE), created_at, updated_at`,
			in.Role, in.TargetID,
		).Scan(&u.ID, &u.Email, &u.Role, &u.IsDefaultAdmin, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt)
	} else {
		err = tx.QueryRowContext(ctx,
			`SELECT id, email, role, COALESCE(is_default_admin, FALSE), COALESCE(must_change_password, FALSE), created_at, updated_at FROM users WHERE id = $1`,
			in.TargetID,
		).Scan(&u.ID, &u.Email, &u.Role, &u.IsDefaultAdmin, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update user: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"targetUserId": in.TargetID,
		"newRole":      in.Role,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.AdminUserID, "user.updated", "user", in.TargetID, payload)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &u, nil
}

func (s *Service) ChangePassword(ctx context.Context, in ChangePasswordInput) error {
	if strings.TrimSpace(in.NewPassword) == "" {
		return fmt.Errorf("%w: new password is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var currentHash string
	err = tx.QueryRowContext(ctx,
		`SELECT password_hash FROM users WHERE id = $1`, in.UserID,
	).Scan(&currentHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("query user: %w", err)
	}

	if ok, verr := auth.VerifyPassword(currentHash, in.OldPassword); verr != nil || !ok {
		return fmt.Errorf("%w: current password is incorrect", ErrForbidden)
	}

	newHash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, must_change_password = FALSE, updated_at = NOW() WHERE id = $2`,
		newHash, in.UserID,
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{"userId": in.UserID})
	internaldb.AppendAuditEvent(ctx, tx, in.UserID, "user.password_changed", "user", in.UserID, payload)

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Default LabAdmin account
// ---------------------------------------------------------------------------

const (
	DefaultAdminEmail    = "labadmin"
	DefaultAdminPassword = "CCI#3341"
	DefaultAdminRole     = "admin"
)

// SeedDefaultAdmin creates the default LabAdmin account if it does not already
// exist.  It is meant to be called once at application startup.
func (s *Service) SeedDefaultAdmin(ctx context.Context) error {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`,
		DefaultAdminEmail,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check default admin: %w", err)
	}
	if exists {
		return nil // already seeded
	}

	hash, err := auth.HashPassword(DefaultAdminPassword)
	if err != nil {
		return fmt.Errorf("hash default password: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, role, is_default_admin)
		 VALUES ($1, $2, $3, TRUE)
		 ON CONFLICT (email) DO NOTHING`,
		DefaultAdminEmail, hash, DefaultAdminRole,
	)
	if err != nil {
		return fmt.Errorf("seed default admin: %w", err)
	}
	return nil
}

// ResetDefaultAdmin resets the LabAdmin password back to the default value.
// This is an unauthenticated safety-net endpoint so the admin can recover
// access if the password is lost.
func (s *Service) ResetDefaultAdmin(ctx context.Context) error {
	hash, err := auth.HashPassword(DefaultAdminPassword)
	if err != nil {
		return fmt.Errorf("hash default password: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, must_change_password = FALSE, updated_at = NOW()
		 WHERE email = $2 AND is_default_admin = TRUE`,
		hash, DefaultAdminEmail,
	)
	if err != nil {
		return fmt.Errorf("reset default admin: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) CreateAccountRequest(ctx context.Context, in CreateAccountRequestInput) (*AccountRequest, error) {
	if in.RequestType != "account_create" && in.RequestType != "password_recovery" {
		return nil, fmt.Errorf("%w: requestType must be account_create or password_recovery", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Username) == "" {
		return nil, fmt.Errorf("%w: username is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Email) == "" {
		return nil, fmt.Errorf("%w: email is required", ErrInvalidInput)
	}

	var out AccountRequest
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO account_requests (request_type, username, email, note)
		 VALUES ($1, $2, LOWER($3), $4)
		 RETURNING id, request_type, username, email, COALESCE(note, ''), status, created_at, updated_at, fulfilled_at`,
		in.RequestType,
		strings.TrimSpace(in.Username),
		strings.TrimSpace(in.Email),
		strings.TrimSpace(in.Note),
	).Scan(&out.ID, &out.RequestType, &out.Username, &out.Email, &out.Note, &out.Status, &out.CreatedAt, &out.UpdatedAt, &out.FulfilledAt)
	if err != nil {
		return nil, fmt.Errorf("insert account request: %w", err)
	}

	return &out, nil
}

func (s *Service) ListAdminUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, role, COALESCE(is_default_admin, FALSE), COALESCE(must_change_password, FALSE), created_at, updated_at
		 FROM users
		 WHERE role IN ('admin','owner')
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query admin users: %w", err)
	}
	defer rows.Close()

	admins := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.IsDefaultAdmin, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan admin user: %w", err)
		}
		admins = append(admins, u)
	}
	return admins, nil
}

func (s *Service) ListAccountRequests(ctx context.Context, in ListAccountRequestsInput) ([]AccountRequest, error) {
	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "pending"
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, request_type, username, email, COALESCE(note, ''), status, created_at, updated_at, fulfilled_at
		 FROM account_requests
		 WHERE status = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		status, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query account requests: %w", err)
	}
	defer rows.Close()

	requests := []AccountRequest{}
	for rows.Next() {
		var item AccountRequest
		if err := rows.Scan(&item.ID, &item.RequestType, &item.Username, &item.Email, &item.Note, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.FulfilledAt); err != nil {
			return nil, fmt.Errorf("scan account request: %w", err)
		}
		requests = append(requests, item)
	}

	return requests, nil
}

func (s *Service) ApproveAccountRequest(ctx context.Context, in ApproveAccountRequestInput) (*AccountRequest, error) {
	if strings.TrimSpace(in.RequestID) == "" {
		return nil, fmt.Errorf("%w: requestId is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.TemporaryPassword) == "" {
		return nil, fmt.Errorf("%w: temporaryPassword is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var req AccountRequest
	err = tx.QueryRowContext(ctx,
		`SELECT id, request_type, username, email, COALESCE(note, ''), status, created_at, updated_at, fulfilled_at
		 FROM account_requests
		 WHERE id = $1
		 FOR UPDATE`,
		in.RequestID,
	).Scan(&req.ID, &req.RequestType, &req.Username, &req.Email, &req.Note, &req.Status, &req.CreatedAt, &req.UpdatedAt, &req.FulfilledAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query account request: %w", err)
	}

	if req.Status != "pending" {
		return nil, fmt.Errorf("%w: request is not pending", ErrConflict)
	}

	tempHash, err := auth.HashPassword(in.TemporaryPassword)
	if err != nil {
		return nil, fmt.Errorf("hash temporary password: %w", err)
	}

	if req.RequestType == "account_create" {
		role := strings.TrimSpace(in.Role)
		if role == "" {
			role = "author"
		}
		validRoles := map[string]bool{"owner": true, "admin": true, "author": true, "viewer": true}
		if !validRoles[role] {
			return nil, fmt.Errorf("%w: role must be owner, admin, author, or viewer", ErrInvalidInput)
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO users (email, password_hash, role, must_change_password)
			 VALUES (LOWER($1), $2, $3, TRUE)`,
			req.Email, tempHash, role,
		)
		if err != nil {
			if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
				return nil, fmt.Errorf("%w: email already exists", ErrConflict)
			}
			return nil, fmt.Errorf("create user from request: %w", err)
		}
	} else {
		res, err := tx.ExecContext(ctx,
			`UPDATE users
			 SET password_hash = $1, must_change_password = TRUE, updated_at = NOW()
			 WHERE LOWER(email) = LOWER($2)`,
			tempHash, req.Email,
		)
		if err != nil {
			return nil, fmt.Errorf("reset password from request: %w", err)
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			return nil, fmt.Errorf("%w: no existing user found for requested email", ErrNotFound)
		}
	}

	err = tx.QueryRowContext(ctx,
		`UPDATE account_requests
		 SET status = 'fulfilled', fulfilled_by_user_id = $1, fulfilled_at = NOW(), updated_at = NOW()
		 WHERE id = $2
		 RETURNING id, request_type, username, email, COALESCE(note, ''), status, created_at, updated_at, fulfilled_at`,
		in.AdminUserID, req.ID,
	).Scan(&req.ID, &req.RequestType, &req.Username, &req.Email, &req.Note, &req.Status, &req.CreatedAt, &req.UpdatedAt, &req.FulfilledAt)
	if err != nil {
		return nil, fmt.Errorf("mark request fulfilled: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"requestId":   req.ID,
		"requestType": req.RequestType,
		"email":       req.Email,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.AdminUserID, "account_request.approved", "account_request", req.ID, payload)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &req, nil
}

func (s *Service) DismissAccountRequest(ctx context.Context, in DismissAccountRequestInput) (*AccountRequest, error) {
	if strings.TrimSpace(in.RequestID) == "" {
		return nil, fmt.Errorf("%w: requestId is required", ErrInvalidInput)
	}

	var req AccountRequest
	err := s.db.QueryRowContext(ctx,
		`UPDATE account_requests
		 SET status = 'dismissed', fulfilled_by_user_id = $1, fulfilled_at = NOW(), updated_at = NOW()
		 WHERE id = $2 AND status = 'pending'
		 RETURNING id, request_type, username, email, COALESCE(note, ''), status, created_at, updated_at, fulfilled_at`,
		in.AdminUserID, in.RequestID,
	).Scan(&req.ID, &req.RequestType, &req.Username, &req.Email, &req.Note, &req.Status, &req.CreatedAt, &req.UpdatedAt, &req.FulfilledAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("dismiss account request: %w", err)
	}

	return &req, nil
}

// DeleteUser deletes a user by ID. Only admins can delete users.
func (s *Service) DeleteUser(ctx context.Context, adminUserID, targetUserID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Prevent deleting the default admin
	var isDefault bool
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(is_default_admin, FALSE) FROM users WHERE id = $1`,
		targetUserID,
	).Scan(&isDefault)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("check user: %w", err)
	}
	if isDefault {
		return fmt.Errorf("%w: cannot delete the default admin", ErrForbidden)
	}

	_, err = tx.ExecContext(ctx,
		`DELETE FROM users WHERE id = $1`, targetUserID,
	)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"deletedUserId": targetUserID,
	})
	internaldb.AppendAuditEvent(ctx, tx, adminUserID, "user.deleted", "user", targetUserID, payload)

	return tx.Commit()
}
