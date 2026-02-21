package experiments

import (
	"context"
	"database/sql"
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

type ConflictError struct {
	ConflictArtifactID string
	ExperimentID       string
	ClientBaseEntryID  string
	ServerLatestEntryID string
}

func (e *ConflictError) Error() string {
	return "stale addendum conflict"
}

type Service struct {
	db   *sql.DB
	sync *syncer.Service
}

func NewService(db *sql.DB, syncService *syncer.Service) *Service {
	return &Service{db: db, sync: syncService}
}

type CreateExperimentInput struct {
	OwnerUserID  string
	DeviceID     string
	Title        string
	OriginalBody string
}

type CreateExperimentOutput struct {
	ExperimentID    string    `json:"experimentId"`
	OriginalEntryID string    `json:"originalEntryId"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
}

type AddAddendumInput struct {
	ExperimentID string
	OwnerUserID  string
	DeviceID     string
	BaseEntryID  string
	Body         string
}

type AddAddendumOutput struct {
	EntryID           string    `json:"entryId"`
	ExperimentID      string    `json:"experimentId"`
	SupersedesEntryID string    `json:"supersedesEntryId"`
	CreatedAt         time.Time `json:"createdAt"`
}

type MarkCompletedOutput struct {
	ExperimentID string    `json:"experimentId"`
	Status       string    `json:"status"`
	CompletedAt  time.Time `json:"completedAt"`
}

type EffectiveView struct {
	ExperimentID     string     `json:"experimentId"`
	OwnerUserID      string     `json:"ownerUserId"`
	Status           string     `json:"status"`
	Title            string     `json:"title"`
	OriginalEntryID  string     `json:"originalEntryId"`
	EffectiveEntryID string     `json:"effectiveEntryId"`
	EffectiveBody    string     `json:"effectiveBody"`
	CreatedAt        time.Time  `json:"createdAt"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
}

type HistoryEntry struct {
	EntryID           string    `json:"entryId"`
	EntryType         string    `json:"entryType"`
	SupersedesEntryID *string   `json:"supersedesEntryId,omitempty"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"createdAt"`
}

type HistoryView struct {
	ExperimentID string         `json:"experimentId"`
	Entries      []HistoryEntry `json:"entries"`
}

func (s *Service) CreateExperiment(ctx context.Context, in CreateExperimentInput) (CreateExperimentOutput, error) {
	if strings.TrimSpace(in.OwnerUserID) == "" || strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.OriginalBody) == "" {
		return CreateExperimentOutput{}, ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CreateExperimentOutput{}, fmt.Errorf("begin create experiment tx: %w", err)
	}
	defer tx.Rollback()

	var (
		experimentID string
		createdAt    time.Time
	)
	err = tx.QueryRowContext(ctx, `
		INSERT INTO experiments (owner_user_id, title, status)
		VALUES ($1, $2, 'draft')
		RETURNING id::text, created_at
	`, in.OwnerUserID, strings.TrimSpace(in.Title)).Scan(&experimentID, &createdAt)
	if err != nil {
		return CreateExperimentOutput{}, fmt.Errorf("insert experiment: %w", err)
	}

	var originalEntryID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO experiment_entries (
			experiment_id,
			author_user_id,
			entry_type,
			body,
			supersedes_entry_id
		) VALUES (
			$1,
			$2,
			'original',
			$3,
			NULL
		)
		RETURNING id::text
	`, experimentID, in.OwnerUserID, in.OriginalBody).Scan(&originalEntryID)
	if err != nil {
		return CreateExperimentOutput{}, fmt.Errorf("insert original entry: %w", err)
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, in.OwnerUserID, "experiment.create", "experiment", experimentID, map[string]any{
		"title":           in.Title,
		"originalEntryId": originalEntryID,
	}); err != nil {
		return CreateExperimentOutput{}, err
	}

	if _, err := s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   in.OwnerUserID,
		ActorUserID:   in.OwnerUserID,
		DeviceID:      in.DeviceID,
		EventType:     "experiment.created",
		AggregateType: "experiment",
		AggregateID:   experimentID,
		Payload: map[string]any{
			"experimentId":   experimentID,
			"originalEntryId": originalEntryID,
			"title":          in.Title,
		},
	}); err != nil {
		return CreateExperimentOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return CreateExperimentOutput{}, fmt.Errorf("commit create experiment tx: %w", err)
	}

	return CreateExperimentOutput{
		ExperimentID:    experimentID,
		OriginalEntryID: originalEntryID,
		Status:          "draft",
		CreatedAt:       createdAt,
	}, nil
}

func (s *Service) AddAddendum(ctx context.Context, in AddAddendumInput) (AddAddendumOutput, error) {
	if strings.TrimSpace(in.ExperimentID) == "" || strings.TrimSpace(in.OwnerUserID) == "" || strings.TrimSpace(in.Body) == "" {
		return AddAddendumOutput{}, ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AddAddendumOutput{}, fmt.Errorf("begin add addendum tx: %w", err)
	}
	defer tx.Rollback()

	if err := ensureOwner(ctx, tx, in.ExperimentID, in.OwnerUserID); err != nil {
		return AddAddendumOutput{}, err
	}

	var supersedesEntryID string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text
		FROM experiment_entries
		WHERE experiment_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, in.ExperimentID).Scan(&supersedesEntryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AddAddendumOutput{}, ErrNotFound
		}
		return AddAddendumOutput{}, fmt.Errorf("load previous entry: %w", err)
	}

	if strings.TrimSpace(in.BaseEntryID) != "" && in.BaseEntryID != supersedesEntryID {
		conflict, err := s.sync.CreateConflict(ctx, tx, syncer.ConflictInput{
			OwnerUserID:         in.OwnerUserID,
			ActorUserID:         in.OwnerUserID,
			DeviceID:            in.DeviceID,
			ExperimentID:        in.ExperimentID,
			ActionType:          "addendum.create.stale_base",
			ClientBaseEntryID:   in.BaseEntryID,
			ServerLatestEntryID: supersedesEntryID,
			Payload: map[string]any{
				"body": in.Body,
			},
		})
		if err != nil {
			return AddAddendumOutput{}, err
		}

		if err := internaldb.AppendAuditEvent(ctx, tx, in.OwnerUserID, "experiment.addendum.conflict", "conflict_artifact", conflict.ConflictArtifactID, map[string]any{
			"experimentId":       in.ExperimentID,
			"clientBaseEntryId":  in.BaseEntryID,
			"serverLatestEntryId": supersedesEntryID,
		}); err != nil {
			return AddAddendumOutput{}, err
		}

		if _, err := s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
			OwnerUserID:   in.OwnerUserID,
			ActorUserID:   in.OwnerUserID,
			DeviceID:      in.DeviceID,
			EventType:     "conflict.stale_addendum",
			AggregateType: "conflict_artifact",
			AggregateID:   conflict.ConflictArtifactID,
			Payload: map[string]any{
				"experimentId":        in.ExperimentID,
				"clientBaseEntryId":   in.BaseEntryID,
				"serverLatestEntryId": supersedesEntryID,
			},
		}); err != nil {
			return AddAddendumOutput{}, err
		}

		if err := tx.Commit(); err != nil {
			return AddAddendumOutput{}, fmt.Errorf("commit addendum conflict tx: %w", err)
		}

		return AddAddendumOutput{}, &ConflictError{
			ConflictArtifactID: conflict.ConflictArtifactID,
			ExperimentID:       in.ExperimentID,
			ClientBaseEntryID:  in.BaseEntryID,
			ServerLatestEntryID: supersedesEntryID,
		}
	}

	var (
		entryID   string
		createdAt time.Time
	)
	err = tx.QueryRowContext(ctx, `
		INSERT INTO experiment_entries (
			experiment_id,
			author_user_id,
			entry_type,
			body,
			supersedes_entry_id
		) VALUES (
			$1,
			$2,
			'addendum',
			$3,
			$4
		)
		RETURNING id::text, created_at
	`, in.ExperimentID, in.OwnerUserID, in.Body, supersedesEntryID).Scan(&entryID, &createdAt)
	if err != nil {
		return AddAddendumOutput{}, fmt.Errorf("insert addendum: %w", err)
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, in.OwnerUserID, "experiment.addendum.create", "experiment_entry", entryID, map[string]any{
		"experimentId":      in.ExperimentID,
		"supersedesEntryId": supersedesEntryID,
	}); err != nil {
		return AddAddendumOutput{}, err
	}

	if _, err := s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   in.OwnerUserID,
		ActorUserID:   in.OwnerUserID,
		DeviceID:      in.DeviceID,
		EventType:     "experiment.addendum.created",
		AggregateType: "experiment_entry",
		AggregateID:   entryID,
		Payload: map[string]any{
			"experimentId":      in.ExperimentID,
			"supersedesEntryId": supersedesEntryID,
		},
	}); err != nil {
		return AddAddendumOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return AddAddendumOutput{}, fmt.Errorf("commit add addendum tx: %w", err)
	}

	return AddAddendumOutput{
		EntryID:           entryID,
		ExperimentID:      in.ExperimentID,
		SupersedesEntryID: supersedesEntryID,
		CreatedAt:         createdAt,
	}, nil
}

func (s *Service) MarkCompleted(ctx context.Context, experimentID, ownerUserID, deviceID string) (MarkCompletedOutput, error) {
	if strings.TrimSpace(experimentID) == "" || strings.TrimSpace(ownerUserID) == "" {
		return MarkCompletedOutput{}, ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MarkCompletedOutput{}, fmt.Errorf("begin complete tx: %w", err)
	}
	defer tx.Rollback()

	if err := ensureOwner(ctx, tx, experimentID, ownerUserID); err != nil {
		return MarkCompletedOutput{}, err
	}

	var completedAt time.Time
	err = tx.QueryRowContext(ctx, `
		UPDATE experiments
		SET status = 'completed', completed_at = COALESCE(completed_at, NOW()), updated_at = NOW()
		WHERE id = $1
		RETURNING completed_at
	`, experimentID).Scan(&completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MarkCompletedOutput{}, ErrNotFound
		}
		return MarkCompletedOutput{}, fmt.Errorf("mark completed: %w", err)
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, ownerUserID, "experiment.complete", "experiment", experimentID, map[string]any{}); err != nil {
		return MarkCompletedOutput{}, err
	}

	if _, err := s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   ownerUserID,
		ActorUserID:   ownerUserID,
		DeviceID:      deviceID,
		EventType:     "experiment.completed",
		AggregateType: "experiment",
		AggregateID:   experimentID,
		Payload: map[string]any{
			"experimentId": experimentID,
		},
	}); err != nil {
		return MarkCompletedOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return MarkCompletedOutput{}, fmt.Errorf("commit complete tx: %w", err)
	}

	return MarkCompletedOutput{
		ExperimentID: experimentID,
		Status:       "completed",
		CompletedAt:  completedAt,
	}, nil
}

func (s *Service) GetEffectiveView(ctx context.Context, experimentID, viewerUserID, viewerRole string) (EffectiveView, error) {
	if err := s.authorizeRead(ctx, experimentID, viewerUserID, viewerRole); err != nil {
		return EffectiveView{}, err
	}

	var (
		out         EffectiveView
		completedAt sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT
			e.id::text,
			e.owner_user_id::text,
			e.status,
			e.title,
			original.id::text,
			COALESCE(latest.id::text, original.id::text) AS effective_entry_id,
			COALESCE(latest.body, original.body)          AS effective_body,
			e.created_at,
			e.completed_at
		FROM experiments e
		JOIN experiment_entries original
			ON original.experiment_id = e.id
		   AND original.entry_type = 'original'
		LEFT JOIN LATERAL (
			SELECT id, body
			FROM experiment_entries
			WHERE experiment_id = e.id
			  AND entry_type = 'addendum'
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		) latest ON true
		WHERE e.id = $1
	`, experimentID).Scan(
		&out.ExperimentID,
		&out.OwnerUserID,
		&out.Status,
		&out.Title,
		&out.OriginalEntryID,
		&out.EffectiveEntryID,
		&out.EffectiveBody,
		&out.CreatedAt,
		&completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return EffectiveView{}, ErrNotFound
		}
		return EffectiveView{}, fmt.Errorf("get effective experiment: %w", err)
	}

	if completedAt.Valid {
		out.CompletedAt = &completedAt.Time
	}

	return out, nil
}

func (s *Service) GetHistory(ctx context.Context, experimentID, viewerUserID, viewerRole string) (HistoryView, error) {
	if err := s.authorizeRead(ctx, experimentID, viewerUserID, viewerRole); err != nil {
		return HistoryView{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, entry_type, supersedes_entry_id::text, body, created_at
		FROM experiment_entries
		WHERE experiment_id = $1
		ORDER BY created_at ASC, id ASC
	`, experimentID)
	if err != nil {
		return HistoryView{}, fmt.Errorf("query experiment history: %w", err)
	}
	defer rows.Close()

	history := HistoryView{ExperimentID: experimentID}
	for rows.Next() {
		var (
			entry      HistoryEntry
			supersedes sql.NullString
		)
		if err := rows.Scan(&entry.EntryID, &entry.EntryType, &supersedes, &entry.Body, &entry.CreatedAt); err != nil {
			return HistoryView{}, fmt.Errorf("scan experiment history: %w", err)
		}
		if supersedes.Valid {
			entry.SupersedesEntryID = &supersedes.String
		}
		history.Entries = append(history.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return HistoryView{}, fmt.Errorf("iterate experiment history: %w", err)
	}

	if len(history.Entries) == 0 {
		return HistoryView{}, ErrNotFound
	}

	return history, nil
}

func (s *Service) authorizeRead(ctx context.Context, experimentID, viewerUserID, viewerRole string) error {
	var ownerID, status string
	err := s.db.QueryRowContext(ctx, `
		SELECT owner_user_id::text, status
		FROM experiments
		WHERE id = $1
	`, experimentID).Scan(&ownerID, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lookup experiment access: %w", err)
	}

	if viewerUserID == ownerID {
		return nil
	}
	if viewerRole == "admin" && status == "completed" {
		return nil
	}
	return ErrForbidden
}

func ensureOwner(ctx context.Context, tx *sql.Tx, experimentID, ownerUserID string) error {
	var ownerID string
	err := tx.QueryRowContext(ctx, `SELECT owner_user_id::text FROM experiments WHERE id = $1`, experimentID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lookup experiment owner: %w", err)
	}
	if ownerID != ownerUserID {
		return ErrForbidden
	}
	return nil
}
