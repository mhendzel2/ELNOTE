package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidInput = errors.New("invalid input")
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type Notification struct {
	ID            string     `json:"notificationId"`
	UserID        string     `json:"userId"`
	EventType     string     `json:"eventType"`
	Title         string     `json:"title"`
	Body          string     `json:"body"`
	ReferenceType string     `json:"referenceType,omitempty"`
	ReferenceID   *string    `json:"referenceId,omitempty"`
	ReadAt        *time.Time `json:"readAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type ListInput struct {
	UserID     string
	UnreadOnly bool
	Limit      int
	Offset     int
}

type ListOutput struct {
	Notifications []Notification `json:"notifications"`
	UnreadCount   int            `json:"unreadCount"`
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

// Create emits a notification for a user.
func (s *Service) Create(ctx context.Context, userID, eventType, title, body, refType string, refID *string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (user_id, event_type, title, body, reference_type, reference_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, eventType, title, body, refType, refID,
	)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// CreateForExperimentOwner notifies the owner of an experiment about an event.
func (s *Service) CreateForExperimentOwner(ctx context.Context, experimentID, eventType, title, body string) error {
	var ownerID string
	err := s.db.QueryRowContext(ctx,
		`SELECT owner_user_id FROM experiments WHERE id = $1`, experimentID,
	).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil // silently skip
		}
		return fmt.Errorf("query experiment owner: %w", err)
	}

	return s.Create(ctx, ownerID, eventType, title, body, "experiment", &experimentID)
}

func (s *Service) List(ctx context.Context, in ListInput) (*ListOutput, error) {
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}

	var query string
	var args []any
	if in.UnreadOnly {
		query = `SELECT id, user_id, event_type, title, body, reference_type, reference_id, read_at, created_at
				 FROM notifications
				 WHERE user_id = $1 AND read_at IS NULL
				 ORDER BY created_at DESC
				 LIMIT $2 OFFSET $3`
	} else {
		query = `SELECT id, user_id, event_type, title, body, reference_type, reference_id, read_at, created_at
				 FROM notifications
				 WHERE user_id = $1
				 ORDER BY created_at DESC
				 LIMIT $2 OFFSET $3`
	}
	args = []any{in.UserID, in.Limit, in.Offset}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query notifications: %w", err)
	}
	defer rows.Close()

	var notifs []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.EventType, &n.Title, &n.Body, &n.ReferenceType, &n.ReferenceID, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		notifs = append(notifs, n)
	}
	if notifs == nil {
		notifs = []Notification{}
	}

	// Unread count
	var unreadCount int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`,
		in.UserID,
	).Scan(&unreadCount)
	if err != nil {
		return nil, fmt.Errorf("count unread: %w", err)
	}

	return &ListOutput{
		Notifications: notifs,
		UnreadCount:   unreadCount,
	}, nil
}

func (s *Service) MarkRead(ctx context.Context, notificationID, userID string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		notificationID, userID,
	)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = NOW()
		 WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("mark all read: %w", err)
	}
	return result.RowsAffected()
}
