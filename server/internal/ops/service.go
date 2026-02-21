package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	internaldb "github.com/mjhen/elnote/server/internal/db"
)

type Service struct {
	db *sql.DB
}

var (
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
)

type Dashboard struct {
	NowUTC                      time.Time `json:"nowUtc"`
	AuthLogin24h                int64     `json:"authLogin24h"`
	AuthRefresh24h              int64     `json:"authRefresh24h"`
	AuthLogout24h               int64     `json:"authLogout24h"`
	SyncEvents24h               int64     `json:"syncEvents24h"`
	SyncConflicts24h            int64     `json:"syncConflicts24h"`
	AttachmentInitiated24h      int64     `json:"attachmentInitiated24h"`
	AttachmentCompleted24h      int64     `json:"attachmentCompleted24h"`
	ReconcileRuns24h            int64     `json:"reconcileRuns24h"`
	ReconcileFindingsUnresolved int64     `json:"reconcileFindingsUnresolved"`
	AuditEvents24h              int64     `json:"auditEvents24h"`
}

type AuditVerificationResult struct {
	Valid               bool      `json:"valid"`
	CheckedEvents       int64     `json:"checkedEvents"`
	BrokenAtEventID     int64     `json:"brokenAtEventId,omitempty"`
	Message             string    `json:"message"`
	CheckedAt           time.Time `json:"checkedAt"`
	LastVerifiedEventID int64     `json:"lastVerifiedEventId,omitempty"`
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)

	out := Dashboard{NowUTC: now}
	var err error

	if out.AuthLogin24h, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM audit_log WHERE event_type = 'auth.login' AND created_at >= $1`, since); err != nil {
		return Dashboard{}, err
	}
	if out.AuthRefresh24h, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM audit_log WHERE event_type = 'auth.refresh' AND created_at >= $1`, since); err != nil {
		return Dashboard{}, err
	}
	if out.AuthLogout24h, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM audit_log WHERE event_type = 'auth.logout' AND created_at >= $1`, since); err != nil {
		return Dashboard{}, err
	}
	if out.SyncEvents24h, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM sync_events WHERE created_at >= $1`, since); err != nil {
		return Dashboard{}, err
	}
	if out.SyncConflicts24h, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM conflict_artifacts WHERE created_at >= $1`, since); err != nil {
		return Dashboard{}, err
	}
	if out.AttachmentInitiated24h, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM audit_log WHERE event_type = 'attachment.initiate' AND created_at >= $1`, since); err != nil {
		return Dashboard{}, err
	}
	if out.AttachmentCompleted24h, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM audit_log WHERE event_type = 'attachment.complete' AND created_at >= $1`, since); err != nil {
		return Dashboard{}, err
	}
	if out.ReconcileRuns24h, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM attachment_reconcile_runs WHERE started_at >= $1`, since); err != nil {
		return Dashboard{}, err
	}
	if out.ReconcileFindingsUnresolved, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM attachment_reconcile_findings WHERE resolved_at IS NULL`); err != nil {
		return Dashboard{}, err
	}
	if out.AuditEvents24h, err = countQuery(ctx, s.db, `SELECT COUNT(*) FROM audit_log WHERE created_at >= $1`, since); err != nil {
		return Dashboard{}, err
	}

	return out, nil
}

func (s *Service) VerifyAuditHashChain(ctx context.Context) (AuditVerificationResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id,
			created_at,
			COALESCE(actor_user_id::text, ''),
			event_type,
			entity_type,
			COALESCE(entity_id::text, ''),
			payload,
			prev_hash,
			event_hash
		FROM audit_log
		ORDER BY id ASC
	`)
	if err != nil {
		return AuditVerificationResult{}, fmt.Errorf("query audit log for verification: %w", err)
	}
	defer rows.Close()

	result := AuditVerificationResult{
		Valid:     true,
		CheckedAt: time.Now().UTC(),
		Message:   "audit hash chain is valid",
	}

	var prevEventHash []byte
	for rows.Next() {
		var (
			eventID    int64
			createdAt  time.Time
			actorID    string
			eventType  string
			entityType string
			entityID   string
			payload    []byte
			prevHash   []byte
			eventHash  []byte
		)
		if err := rows.Scan(&eventID, &createdAt, &actorID, &eventType, &entityType, &entityID, &payload, &prevHash, &eventHash); err != nil {
			return AuditVerificationResult{}, fmt.Errorf("scan audit row: %w", err)
		}

		result.CheckedEvents++
		result.LastVerifiedEventID = eventID

		if !bytes.Equal(prevHash, prevEventHash) {
			result.Valid = false
			result.BrokenAtEventID = eventID
			result.Message = "audit prev_hash does not match previous event hash"
			return result, nil
		}

		serialized := fmt.Sprintf(
			"%s|%s|%s|%s|%s|%s|%s",
			createdAt.UTC().Format(time.RFC3339Nano),
			actorID,
			eventType,
			entityType,
			entityID,
			string(payload),
			hex.EncodeToString(prevHash),
		)
		computed := sha256.Sum256([]byte(serialized))
		if !bytes.Equal(eventHash, computed[:]) {
			result.Valid = false
			result.BrokenAtEventID = eventID
			result.Message = "audit event_hash checksum mismatch"
			return result, nil
		}

		prevEventHash = eventHash
	}
	if err := rows.Err(); err != nil {
		return AuditVerificationResult{}, fmt.Errorf("iterate audit rows: %w", err)
	}

	return result, nil
}

func (s *Service) ForensicExport(ctx context.Context, experimentID string) (map[string]any, error) {
	experimentID = strings.TrimSpace(experimentID)
	if experimentID == "" {
		return nil, ErrInvalidInput
	}

	var (
		expID       string
		ownerUserID string
		title       string
		status      string
		createdAt   time.Time
		completedAt sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, owner_user_id::text, title, status, created_at, completed_at
		FROM experiments
		WHERE id = $1::uuid
	`, experimentID).Scan(
		&expID,
		&ownerUserID,
		&title,
		&status,
		&createdAt,
		&completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load experiment for forensic export: %w", err)
	}
	if status != "completed" {
		return nil, ErrForbidden
	}

	experiment := map[string]any{
		"experimentId": expID,
		"ownerUserId":  ownerUserID,
		"title":        title,
		"status":       status,
		"createdAt":    createdAt,
	}
	if completedAt.Valid {
		experiment["completedAt"] = completedAt.Time
	}

	entries, err := rowsToMaps(ctx, s.db, `
		SELECT id::text AS entry_id, entry_type, supersedes_entry_id::text, body, author_user_id::text, created_at
		FROM experiment_entries
		WHERE experiment_id = $1::uuid
		ORDER BY created_at ASC, id ASC
	`, experimentID)
	if err != nil {
		return nil, err
	}

	comments, err := rowsToMaps(ctx, s.db, `
		SELECT id::text AS comment_id, author_user_id::text, body, created_at
		FROM record_comments
		WHERE experiment_id = $1::uuid
		ORDER BY created_at ASC
	`, experimentID)
	if err != nil {
		return nil, err
	}

	proposals, err := rowsToMaps(ctx, s.db, `
		SELECT id::text AS proposal_id, proposer_user_id::text, title, body, created_at
		FROM experiment_proposals
		WHERE source_experiment_id = $1::uuid
		ORDER BY created_at ASC
	`, experimentID)
	if err != nil {
		return nil, err
	}

	attachments, err := rowsToMaps(ctx, s.db, `
		SELECT id::text AS attachment_id, uploader_user_id::text, object_key, checksum, size_bytes, mime_type, status, created_at, completed_at
		FROM attachments
		WHERE experiment_id = $1::uuid
		ORDER BY created_at ASC
	`, experimentID)
	if err != nil {
		return nil, err
	}

	auditRows, err := s.db.QueryContext(ctx, `
		SELECT
			id,
			event_id::text,
			COALESCE(actor_user_id::text, ''),
			event_type,
			entity_type,
			COALESCE(entity_id::text, ''),
			payload,
			created_at,
			COALESCE(encode(prev_hash, 'hex'), ''),
			COALESCE(encode(event_hash, 'hex'), '')
		FROM (
			SELECT
				id,
				event_id,
				actor_user_id,
				event_type,
				entity_type,
				entity_id,
				payload,
				created_at,
				prev_hash,
				event_hash
			FROM audit_log
			WHERE entity_id = $1::uuid
			   OR payload->>'experimentId' = $1
			   OR payload->>'sourceExperimentId' = $1
			ORDER BY id ASC
		) q
	`, experimentID)
	if err != nil {
		return nil, fmt.Errorf("query audit rows for forensic export: %w", err)
	}
	defer auditRows.Close()

	auditEvents := make([]map[string]any, 0)
	for auditRows.Next() {
		var (
			id        int64
			eventID   string
			actorID   string
			eventType string
			entityType string
			entityID  string
			payload   []byte
			createdAt time.Time
			prevHash  string
			eventHash string
		)
		if err := auditRows.Scan(&id, &eventID, &actorID, &eventType, &entityType, &entityID, &payload, &createdAt, &prevHash, &eventHash); err != nil {
			return nil, fmt.Errorf("scan forensic audit row: %w", err)
		}
		var payloadObj map[string]any
		_ = json.Unmarshal(payload, &payloadObj)
		auditEvents = append(auditEvents, map[string]any{
			"id":         id,
			"eventId":    eventID,
			"actorUserId": actorID,
			"eventType":  eventType,
			"entityType": entityType,
			"entityId":   entityID,
			"payload":    payloadObj,
			"createdAt":  createdAt,
			"prevHash":   prevHash,
			"eventHash":  eventHash,
		})
	}
	if err := auditRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forensic audit rows: %w", err)
	}

	return map[string]any{
		"exportedAt":  time.Now().UTC(),
		"experiment":  experiment,
		"entries":     entries,
		"comments":    comments,
		"proposals":   proposals,
		"attachments": attachments,
		"auditEvents": auditEvents,
	}, nil
}

func (s *Service) LogForensicExport(ctx context.Context, actorUserID, experimentID string) error {
	actorUserID = strings.TrimSpace(actorUserID)
	experimentID = strings.TrimSpace(experimentID)
	if actorUserID == "" || experimentID == "" {
		return ErrInvalidInput
	}

	if err := internaldb.AppendAuditEvent(ctx, s.db, actorUserID, "ops.forensic.export", "experiment", experimentID, map[string]any{
		"experimentId": experimentID,
	}); err != nil {
		return fmt.Errorf("append forensic export audit event: %w", err)
	}

	return nil
}

func countQuery(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, args ...any) (int64, error) {
	var count int64
	if err := q.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count query failed: %w", err)
	}
	return count, nil
}

func rowsToMaps(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query rows for forensic export: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("load column names for forensic export: %w", err)
	}

	out := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan forensic export row: %w", err)
		}

		row := make(map[string]any, len(columns))
		for i, col := range columns {
			v := values[i]
			switch typed := v.(type) {
			case []byte:
				row[col] = string(typed)
			default:
				row[col] = typed
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forensic export rows: %w", err)
	}

	return out, nil
}
