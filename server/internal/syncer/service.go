package syncer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Service struct {
	db *sql.DB
}

type execQueryStore interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type AppendEventInput struct {
	OwnerUserID    string
	ActorUserID    string
	DeviceID       string
	EventType      string
	AggregateType  string
	AggregateID    string
	Payload        any
}

type Event struct {
	Cursor        int64           `json:"cursor"`
	OwnerUserID   string          `json:"ownerUserId"`
	ActorUserID   string          `json:"actorUserId,omitempty"`
	DeviceID      string          `json:"deviceId,omitempty"`
	EventType     string          `json:"eventType"`
	AggregateType string          `json:"aggregateType"`
	AggregateID   string          `json:"aggregateId,omitempty"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type PullResult struct {
	Cursor  int64   `json:"cursor"`
	Events  []Event `json:"events"`
	HasMore bool    `json:"hasMore"`
}

type ConflictInput struct {
	OwnerUserID        string
	ActorUserID        string
	DeviceID           string
	ExperimentID       string
	ActionType         string
	ClientBaseEntryID  string
	ServerLatestEntryID string
	Payload            any
}

type ConflictArtifact struct {
	ConflictArtifactID string          `json:"conflictArtifactId"`
	OwnerUserID        string          `json:"ownerUserId"`
	ActorUserID        string          `json:"actorUserId,omitempty"`
	DeviceID           string          `json:"deviceId,omitempty"`
	ExperimentID       string          `json:"experimentId"`
	ActionType         string          `json:"actionType"`
	ClientBaseEntryID  string          `json:"clientBaseEntryId,omitempty"`
	ServerLatestEntryID string         `json:"serverLatestEntryId,omitempty"`
	Payload            json.RawMessage `json:"payload"`
	CreatedAt          time.Time       `json:"createdAt"`
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) AppendEvent(ctx context.Context, store execQueryStore, in AppendEventInput) (int64, error) {
	if store == nil {
		store = s.db
	}
	if in.OwnerUserID == "" || in.EventType == "" || in.AggregateType == "" {
		return 0, errors.New("ownerUserID, eventType, and aggregateType are required")
	}

	payloadJSON, err := json.Marshal(in.Payload)
	if err != nil {
		return 0, fmt.Errorf("marshal sync payload: %w", err)
	}

	var cursor int64
	err = store.QueryRowContext(ctx, `
		INSERT INTO sync_events (
			owner_user_id,
			actor_user_id,
			device_id,
			event_type,
			aggregate_type,
			aggregate_id,
			payload
		) VALUES (
			$1::uuid,
			NULLIF($2, '')::uuid,
			NULLIF($3, '')::uuid,
			$4,
			$5,
			NULLIF($6, '')::uuid,
			$7::jsonb
		)
		RETURNING cursor
	`, in.OwnerUserID, in.ActorUserID, in.DeviceID, in.EventType, in.AggregateType, in.AggregateID, string(payloadJSON)).Scan(&cursor)
	if err != nil {
		return 0, fmt.Errorf("insert sync event: %w", err)
	}

	return cursor, nil
}

func (s *Service) CreateConflict(ctx context.Context, store execQueryStore, in ConflictInput) (ConflictArtifact, error) {
	if store == nil {
		store = s.db
	}
	if in.OwnerUserID == "" || in.ExperimentID == "" || in.ActionType == "" {
		return ConflictArtifact{}, errors.New("ownerUserID, experimentID, and actionType are required")
	}

	payloadJSON, err := json.Marshal(in.Payload)
	if err != nil {
		return ConflictArtifact{}, fmt.Errorf("marshal conflict payload: %w", err)
	}

	out := ConflictArtifact{}
	err = store.QueryRowContext(ctx, `
		INSERT INTO conflict_artifacts (
			owner_user_id,
			actor_user_id,
			device_id,
			experiment_id,
			action_type,
			client_base_entry_id,
			server_latest_entry_id,
			payload
		) VALUES (
			$1::uuid,
			NULLIF($2, '')::uuid,
			NULLIF($3, '')::uuid,
			$4::uuid,
			$5,
			NULLIF($6, '')::uuid,
			NULLIF($7, '')::uuid,
			$8::jsonb
		)
		RETURNING
			id::text,
			owner_user_id::text,
			COALESCE(actor_user_id::text, ''),
			COALESCE(device_id::text, ''),
			experiment_id::text,
			action_type,
			COALESCE(client_base_entry_id::text, ''),
			COALESCE(server_latest_entry_id::text, ''),
			payload,
			created_at
	`, in.OwnerUserID, in.ActorUserID, in.DeviceID, in.ExperimentID, in.ActionType, in.ClientBaseEntryID, in.ServerLatestEntryID, string(payloadJSON)).Scan(
		&out.ConflictArtifactID,
		&out.OwnerUserID,
		&out.ActorUserID,
		&out.DeviceID,
		&out.ExperimentID,
		&out.ActionType,
		&out.ClientBaseEntryID,
		&out.ServerLatestEntryID,
		&out.Payload,
		&out.CreatedAt,
	)
	if err != nil {
		return ConflictArtifact{}, fmt.Errorf("insert conflict artifact: %w", err)
	}

	return out, nil
}

func (s *Service) Pull(ctx context.Context, userID string, cursor int64, limit int) (PullResult, error) {
	if userID == "" {
		return PullResult{}, errors.New("userID is required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			cursor,
			owner_user_id::text,
			COALESCE(actor_user_id::text, ''),
			COALESCE(device_id::text, ''),
			event_type,
			aggregate_type,
			COALESCE(aggregate_id::text, ''),
			payload,
			created_at
		FROM sync_events
		WHERE cursor > $1
		  AND (owner_user_id = $2::uuid OR actor_user_id = $2::uuid)
		ORDER BY cursor ASC
		LIMIT $3
	`, cursor, userID, limit+1)
	if err != nil {
		return PullResult{}, fmt.Errorf("query sync events: %w", err)
	}
	defer rows.Close()

	out := PullResult{Cursor: cursor}
	for rows.Next() {
		var event Event
		if err := rows.Scan(
			&event.Cursor,
			&event.OwnerUserID,
			&event.ActorUserID,
			&event.DeviceID,
			&event.EventType,
			&event.AggregateType,
			&event.AggregateID,
			&event.Payload,
			&event.CreatedAt,
		); err != nil {
			return PullResult{}, fmt.Errorf("scan sync event: %w", err)
		}
		out.Events = append(out.Events, event)
	}
	if err := rows.Err(); err != nil {
		return PullResult{}, fmt.Errorf("iterate sync events: %w", err)
	}

	if len(out.Events) > limit {
		out.Events = out.Events[:limit]
		out.HasMore = true
	}
	if len(out.Events) > 0 {
		out.Cursor = out.Events[len(out.Events)-1].Cursor
	}

	return out, nil
}

func (s *Service) ListConflicts(ctx context.Context, userID string, limit int) ([]ConflictArtifact, error) {
	if userID == "" {
		return nil, errors.New("userID is required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id::text,
			owner_user_id::text,
			COALESCE(actor_user_id::text, ''),
			COALESCE(device_id::text, ''),
			experiment_id::text,
			action_type,
			COALESCE(client_base_entry_id::text, ''),
			COALESCE(server_latest_entry_id::text, ''),
			payload,
			created_at
		FROM conflict_artifacts
		WHERE owner_user_id = $1::uuid
		   OR actor_user_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query conflict artifacts: %w", err)
	}
	defer rows.Close()

	artifacts := make([]ConflictArtifact, 0)
	for rows.Next() {
		var artifact ConflictArtifact
		if err := rows.Scan(
			&artifact.ConflictArtifactID,
			&artifact.OwnerUserID,
			&artifact.ActorUserID,
			&artifact.DeviceID,
			&artifact.ExperimentID,
			&artifact.ActionType,
			&artifact.ClientBaseEntryID,
			&artifact.ServerLatestEntryID,
			&artifact.Payload,
			&artifact.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan conflict artifact: %w", err)
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conflict artifacts: %w", err)
	}

	return artifacts, nil
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

func (s *Service) ServeWS(w http.ResponseWriter, r *http.Request, userID string, cursor int64) error {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("upgrade websocket: %w", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type":   "connected",
		"cursor": cursor,
	}); err != nil {
		return fmt.Errorf("write websocket connected payload: %w", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.SetReadLimit(1024)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	currentCursor := cursor
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-done:
			return nil
		case <-ticker.C:
			pullCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			result, err := s.Pull(pullCtx, userID, currentCursor, 200)
			cancel()
			if err != nil {
				if writeErr := conn.WriteJSON(map[string]any{"type": "error", "error": err.Error()}); writeErr != nil {
					return fmt.Errorf("write websocket error payload: %w", writeErr)
				}
				continue
			}

			message := map[string]any{
				"type":   "events",
				"cursor": result.Cursor,
				"events": result.Events,
			}
			if len(result.Events) == 0 {
				message["type"] = "heartbeat"
			}
			if err := conn.WriteJSON(message); err != nil {
				return fmt.Errorf("write websocket payload: %w", err)
			}

			currentCursor = result.Cursor
		}
	}
}
