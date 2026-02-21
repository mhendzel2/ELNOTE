package protocols

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	internaldb "github.com/mjhen/elnote/server/internal/db"
	"github.com/mjhen/elnote/server/internal/syncer"
)

var (
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type Protocol struct {
	ID          string    `json:"protocolId"`
	OwnerUserID string    `json:"ownerUserId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ProtocolVersion struct {
	ID            string    `json:"versionId"`
	ProtocolID    string    `json:"protocolId"`
	VersionNumber int       `json:"versionNumber"`
	Body          string    `json:"body"`
	ChangeSummary string    `json:"changeSummary"`
	AuthorUserID  string    `json:"authorUserId"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Deviation struct {
	ID                string    `json:"deviationId"`
	ExperimentID      string    `json:"experimentId"`
	ExperimentEntryID string    `json:"experimentEntryId"`
	DeviationType     string    `json:"deviationType"`
	Rationale         string    `json:"rationale"`
	CreatedAt         time.Time `json:"createdAt"`
}

type ExperimentProtocolLink struct {
	ID                string    `json:"id"`
	ExperimentID      string    `json:"experimentId"`
	ProtocolID        string    `json:"protocolId"`
	ProtocolVersionID string    `json:"protocolVersionId"`
	CreatedAt         time.Time `json:"createdAt"`
}

// ---------------------------------------------------------------------------
// Inputs / Outputs
// ---------------------------------------------------------------------------

type CreateProtocolInput struct {
	OwnerUserID string
	Title       string
	Description string
	InitialBody string
}

type CreateProtocolOutput struct {
	ProtocolID string    `json:"protocolId"`
	VersionID  string    `json:"versionId"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
}

type PublishVersionInput struct {
	ProtocolID    string
	AuthorUserID  string
	Body          string
	ChangeSummary string
}

type PublishVersionOutput struct {
	VersionID     string    `json:"versionId"`
	VersionNumber int       `json:"versionNumber"`
	CreatedAt     time.Time `json:"createdAt"`
}

type LinkProtocolInput struct {
	ExperimentID      string
	ProtocolID        string
	ProtocolVersionID string
	ActorUserID       string
	DeviceID          string
}

type RecordDeviationInput struct {
	ExperimentID      string
	ExperimentEntryID string
	DeviationType     string
	Rationale         string
	ActorUserID       string
	DeviceID          string
}

type RecordDeviationOutput struct {
	DeviationID string    `json:"deviationId"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

type Service struct {
	db   *sql.DB
	sync *syncer.Service
}

func NewService(db *sql.DB, syncService *syncer.Service) *Service {
	return &Service{db: db, sync: syncService}
}

func (s *Service) CreateProtocol(ctx context.Context, in CreateProtocolInput) (*CreateProtocolOutput, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.InitialBody) == "" {
		return nil, fmt.Errorf("%w: initial body is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var protocolID string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx,
		`INSERT INTO protocols (owner_user_id, title, description, status)
		 VALUES ($1, $2, $3, 'draft')
		 RETURNING id, created_at`,
		in.OwnerUserID, in.Title, in.Description,
	).Scan(&protocolID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert protocol: %w", err)
	}

	var versionID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO protocol_versions (protocol_id, version_number, body, change_summary, author_user_id)
		 VALUES ($1, 1, $2, 'Initial version', $3)
		 RETURNING id`,
		protocolID, in.InitialBody, in.OwnerUserID,
	).Scan(&versionID)
	if err != nil {
		return nil, fmt.Errorf("insert version: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"protocolId": protocolID,
		"title":      in.Title,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.OwnerUserID, "protocol.created", "protocol", protocolID, payload)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &CreateProtocolOutput{
		ProtocolID: protocolID,
		VersionID:  versionID,
		Status:     "draft",
		CreatedAt:  createdAt,
	}, nil
}

func (s *Service) PublishVersion(ctx context.Context, in PublishVersionInput) (*PublishVersionOutput, error) {
	if strings.TrimSpace(in.Body) == "" {
		return nil, fmt.Errorf("%w: body is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Verify protocol exists and caller owns it
	var ownerID, status string
	err = tx.QueryRowContext(ctx,
		`SELECT owner_user_id, status FROM protocols WHERE id = $1`,
		in.ProtocolID,
	).Scan(&ownerID, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query protocol: %w", err)
	}
	if ownerID != in.AuthorUserID {
		return nil, ErrForbidden
	}
	if status == "archived" {
		return nil, fmt.Errorf("%w: protocol is archived", ErrInvalidInput)
	}

	// Get next version number
	var maxVersion int
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version_number), 0) FROM protocol_versions WHERE protocol_id = $1`,
		in.ProtocolID,
	).Scan(&maxVersion)
	if err != nil {
		return nil, fmt.Errorf("query max version: %w", err)
	}

	nextVersion := maxVersion + 1
	var versionID string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx,
		`INSERT INTO protocol_versions (protocol_id, version_number, body, change_summary, author_user_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		in.ProtocolID, nextVersion, in.Body, in.ChangeSummary, in.AuthorUserID,
	).Scan(&versionID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert version: %w", err)
	}

	// Auto-publish if still draft
	if status == "draft" {
		_, err = tx.ExecContext(ctx,
			`UPDATE protocols SET status = 'published', updated_at = NOW() WHERE id = $1`,
			in.ProtocolID,
		)
		if err != nil {
			return nil, fmt.Errorf("publish protocol: %w", err)
		}
	}

	payload, _ := json.Marshal(map[string]any{
		"protocolId":    in.ProtocolID,
		"versionNumber": nextVersion,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.AuthorUserID, "protocol.version_published", "protocol", in.ProtocolID, payload)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &PublishVersionOutput{
		VersionID:     versionID,
		VersionNumber: nextVersion,
		CreatedAt:     createdAt,
	}, nil
}

func (s *Service) GetProtocol(ctx context.Context, protocolID, userID, role string) (*Protocol, error) {
	var p Protocol
	err := s.db.QueryRowContext(ctx,
		`SELECT id, owner_user_id, title, description, status, created_at, updated_at
		 FROM protocols WHERE id = $1`,
		protocolID,
	).Scan(&p.ID, &p.OwnerUserID, &p.Title, &p.Description, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query protocol: %w", err)
	}
	// Owners see their own; admins see published/archived
	if p.OwnerUserID != userID {
		if role != "admin" || p.Status == "draft" {
			return nil, ErrForbidden
		}
	}
	return &p, nil
}

func (s *Service) ListProtocols(ctx context.Context, userID, role string) ([]Protocol, error) {
	var query string
	var args []any
	if role == "admin" {
		query = `SELECT id, owner_user_id, title, description, status, created_at, updated_at
				 FROM protocols WHERE status IN ('published','archived')
				 ORDER BY updated_at DESC`
	} else {
		query = `SELECT id, owner_user_id, title, description, status, created_at, updated_at
				 FROM protocols WHERE owner_user_id = $1
				 ORDER BY updated_at DESC`
		args = append(args, userID)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query protocols: %w", err)
	}
	defer rows.Close()

	var protocols []Protocol
	for rows.Next() {
		var p Protocol
		if err := rows.Scan(&p.ID, &p.OwnerUserID, &p.Title, &p.Description, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan protocol: %w", err)
		}
		protocols = append(protocols, p)
	}
	if protocols == nil {
		protocols = []Protocol{}
	}
	return protocols, nil
}

func (s *Service) ListVersions(ctx context.Context, protocolID, userID, role string) ([]ProtocolVersion, error) {
	// Check access
	if _, err := s.GetProtocol(ctx, protocolID, userID, role); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, protocol_id, version_number, body, change_summary, author_user_id, created_at
		 FROM protocol_versions WHERE protocol_id = $1
		 ORDER BY version_number DESC`,
		protocolID,
	)
	if err != nil {
		return nil, fmt.Errorf("query versions: %w", err)
	}
	defer rows.Close()

	var versions []ProtocolVersion
	for rows.Next() {
		var v ProtocolVersion
		if err := rows.Scan(&v.ID, &v.ProtocolID, &v.VersionNumber, &v.Body, &v.ChangeSummary, &v.AuthorUserID, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		versions = append(versions, v)
	}
	if versions == nil {
		versions = []ProtocolVersion{}
	}
	return versions, nil
}

func (s *Service) UpdateStatus(ctx context.Context, protocolID, ownerUserID, newStatus string) error {
	if newStatus != "published" && newStatus != "archived" {
		return fmt.Errorf("%w: status must be published or archived", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var currentOwner string
	err = tx.QueryRowContext(ctx,
		`SELECT owner_user_id FROM protocols WHERE id = $1`, protocolID,
	).Scan(&currentOwner)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("query owner: %w", err)
	}
	if currentOwner != ownerUserID {
		return ErrForbidden
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE protocols SET status = $1, updated_at = NOW() WHERE id = $2`,
		newStatus, protocolID,
	)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{"protocolId": protocolID, "status": newStatus})
	internaldb.AppendAuditEvent(ctx, tx, ownerUserID, "protocol.status_changed", "protocol", protocolID, payload)

	return tx.Commit()
}

func (s *Service) LinkToExperiment(ctx context.Context, in LinkProtocolInput) (*ExperimentProtocolLink, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Verify experiment belongs to actor
	var expOwner string
	err = tx.QueryRowContext(ctx,
		`SELECT owner_user_id FROM experiments WHERE id = $1`, in.ExperimentID,
	).Scan(&expOwner)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query experiment: %w", err)
	}
	if expOwner != in.ActorUserID {
		return nil, ErrForbidden
	}

	var link ExperimentProtocolLink
	err = tx.QueryRowContext(ctx,
		`INSERT INTO experiment_protocols (experiment_id, protocol_id, protocol_version_id)
		 VALUES ($1, $2, $3)
		 RETURNING id, experiment_id, protocol_id, protocol_version_id, created_at`,
		in.ExperimentID, in.ProtocolID, in.ProtocolVersionID,
	).Scan(&link.ID, &link.ExperimentID, &link.ProtocolID, &link.ProtocolVersionID, &link.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert link: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"experimentId":      in.ExperimentID,
		"protocolId":        in.ProtocolID,
		"protocolVersionId": in.ProtocolVersionID,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.ActorUserID, "experiment.protocol_linked", "experiment", in.ExperimentID, payload)

	s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   expOwner,
		ActorUserID:   in.ActorUserID,
		DeviceID:      in.DeviceID,
		EventType:     "experiment.protocol_linked",
		AggregateType: "experiment",
		AggregateID:   in.ExperimentID,
		Payload:       payload,
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &link, nil
}

func (s *Service) RecordDeviation(ctx context.Context, in RecordDeviationInput) (*RecordDeviationOutput, error) {
	if in.DeviationType != "planned" && in.DeviationType != "unplanned" && in.DeviationType != "observation" {
		return nil, fmt.Errorf("%w: deviationType must be planned, unplanned, or observation", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Rationale) == "" {
		return nil, fmt.Errorf("%w: rationale is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var expOwner string
	err = tx.QueryRowContext(ctx,
		`SELECT owner_user_id FROM experiments WHERE id = $1`, in.ExperimentID,
	).Scan(&expOwner)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query experiment: %w", err)
	}
	if expOwner != in.ActorUserID {
		return nil, ErrForbidden
	}

	var deviationID string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx,
		`INSERT INTO protocol_deviations (experiment_id, experiment_entry_id, deviation_type, rationale)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		in.ExperimentID, in.ExperimentEntryID, in.DeviationType, in.Rationale,
	).Scan(&deviationID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert deviation: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"deviationId":   deviationID,
		"deviationType": in.DeviationType,
		"experimentId":  in.ExperimentID,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.ActorUserID, "protocol.deviation_recorded", "experiment", in.ExperimentID, payload)

	s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   expOwner,
		ActorUserID:   in.ActorUserID,
		DeviceID:      in.DeviceID,
		EventType:     "protocol.deviation_recorded",
		AggregateType: "experiment",
		AggregateID:   in.ExperimentID,
		Payload:       payload,
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &RecordDeviationOutput{DeviationID: deviationID, CreatedAt: createdAt}, nil
}

func (s *Service) ListDeviations(ctx context.Context, experimentID, userID, role string) ([]Deviation, error) {
	// Check access — owner or admin on completed experiments
	var expOwner, expStatus string
	err := s.db.QueryRowContext(ctx,
		`SELECT owner_user_id, status FROM experiments WHERE id = $1`, experimentID,
	).Scan(&expOwner, &expStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query experiment: %w", err)
	}
	if expOwner != userID {
		if role != "admin" || expStatus != "completed" {
			return nil, ErrForbidden
		}
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, experiment_id, experiment_entry_id, deviation_type, rationale, created_at
		 FROM protocol_deviations WHERE experiment_id = $1
		 ORDER BY created_at`,
		experimentID,
	)
	if err != nil {
		return nil, fmt.Errorf("query deviations: %w", err)
	}
	defer rows.Close()

	var deviations []Deviation
	for rows.Next() {
		var d Deviation
		if err := rows.Scan(&d.ID, &d.ExperimentID, &d.ExperimentEntryID, &d.DeviationType, &d.Rationale, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan deviation: %w", err)
		}
		deviations = append(deviations, d)
	}
	if deviations == nil {
		deviations = []Deviation{}
	}
	return deviations, nil
}
