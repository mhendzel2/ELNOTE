package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type AuditStore interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func AppendAuditEvent(
	ctx context.Context,
	store AuditStore,
	actorUserID string,
	eventType string,
	entityType string,
	entityID string,
	payload any,
) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal audit payload: %w", err)
	}
	payloadJSON, err = canonicalizeAuditPayload(payloadJSON)
	if err != nil {
		return fmt.Errorf("canonicalize audit payload: %w", err)
	}

	var prevHash []byte
	err = store.QueryRowContext(ctx, `SELECT event_hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&prevHash)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load previous audit hash: %w", err)
	}
	if err == sql.ErrNoRows {
		prevHash = nil
	}

	// Postgres stores timestamptz at microsecond precision by default.
	// Truncate pre-hash to keep hash input deterministic across write/read.
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	serialized := fmt.Sprintf(
		"%s|%s|%s|%s|%s|%s|%s",
		createdAt.Format(time.RFC3339Nano),
		actorUserID,
		eventType,
		entityType,
		entityID,
		string(payloadJSON),
		hex.EncodeToString(prevHash),
	)
	eventHash := sha256.Sum256([]byte(serialized))

	_, err = store.ExecContext(ctx, `
		INSERT INTO audit_log (
			actor_user_id,
			event_type,
			entity_type,
			entity_id,
			payload,
			created_at,
			prev_hash,
			event_hash
		) VALUES (
			NULLIF($1, '')::uuid,
			$2,
			$3,
			NULLIF($4, '')::uuid,
			$5::jsonb,
			$6,
			$7,
			$8
		)
	`, actorUserID, eventType, entityType, entityID, string(payloadJSON), createdAt, prevHash, eventHash[:])
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}

	return nil
}

func canonicalizeAuditPayload(raw []byte) ([]byte, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return json.Marshal(payload)
}
