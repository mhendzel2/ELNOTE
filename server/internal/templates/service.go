package templates

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

type Template struct {
	ID          string    `json:"templateId"`
	OwnerUserID string    `json:"ownerUserId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	BodyTemplate string   `json:"bodyTemplate"`
	Sections    []Section `json:"sections"`
	ProtocolID  *string   `json:"protocolId,omitempty"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Section struct {
	Name        string `json:"name"`
	Placeholder string `json:"placeholder"`
	Required    bool   `json:"required"`
}

type CreateTemplateInput struct {
	OwnerUserID  string
	Title        string
	Description  string
	BodyTemplate string
	Sections     []Section
	ProtocolID   *string
	Tags         []string
}

type UpdateTemplateInput struct {
	TemplateID   string
	OwnerUserID  string
	Description  string
	BodyTemplate string
	Sections     []Section
	Tags         []string
}

type CloneExperimentInput struct {
	SourceExperimentID string
	OwnerUserID        string
	DeviceID           string
	NewTitle           string
}

type CloneExperimentOutput struct {
	ExperimentID    string    `json:"experimentId"`
	OriginalEntryID string    `json:"originalEntryId"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
}

type CreateFromTemplateInput struct {
	TemplateID  string
	OwnerUserID string
	DeviceID    string
	Title       string
	Body        string // optional override — empty means use template body
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

func (s *Service) CreateTemplate(ctx context.Context, in CreateTemplateInput) (*Template, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.BodyTemplate) == "" {
		return nil, fmt.Errorf("%w: bodyTemplate is required", ErrInvalidInput)
	}

	sectionsJSON, _ := json.Marshal(in.Sections)
	if in.Sections == nil {
		sectionsJSON = []byte("[]")
	}
	tagsJSON, _ := json.Marshal(in.Tags)
	if in.Tags == nil {
		tagsJSON = []byte("[]")
	}

	var tmpl Template
	var sectionsRaw, tagsRaw []byte
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO experiment_templates (owner_user_id, title, description, body_template, sections, protocol_id, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, owner_user_id, title, description, body_template, sections, protocol_id, tags, created_at, updated_at`,
		in.OwnerUserID, in.Title, in.Description, in.BodyTemplate, sectionsJSON, in.ProtocolID, tagsJSON,
	).Scan(&tmpl.ID, &tmpl.OwnerUserID, &tmpl.Title, &tmpl.Description, &tmpl.BodyTemplate,
		&sectionsRaw, &tmpl.ProtocolID, &tagsRaw, &tmpl.CreatedAt, &tmpl.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert template: %w", err)
	}
	json.Unmarshal(sectionsRaw, &tmpl.Sections)
	json.Unmarshal(tagsRaw, &tmpl.Tags)

	return &tmpl, nil
}

func (s *Service) ListTemplates(ctx context.Context, ownerUserID string) ([]Template, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, owner_user_id, title, description, body_template, sections, protocol_id, tags, created_at, updated_at
		 FROM experiment_templates WHERE owner_user_id = $1
		 ORDER BY updated_at DESC`,
		ownerUserID,
	)
	if err != nil {
		return nil, fmt.Errorf("query templates: %w", err)
	}
	defer rows.Close()

	var templates []Template
	for rows.Next() {
		var t Template
		var sectionsRaw, tagsRaw []byte
		if err := rows.Scan(&t.ID, &t.OwnerUserID, &t.Title, &t.Description, &t.BodyTemplate,
			&sectionsRaw, &t.ProtocolID, &tagsRaw, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		json.Unmarshal(sectionsRaw, &t.Sections)
		json.Unmarshal(tagsRaw, &t.Tags)
		templates = append(templates, t)
	}
	if templates == nil {
		templates = []Template{}
	}
	return templates, nil
}

func (s *Service) GetTemplate(ctx context.Context, templateID, ownerUserID string) (*Template, error) {
	var t Template
	var sectionsRaw, tagsRaw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, owner_user_id, title, description, body_template, sections, protocol_id, tags, created_at, updated_at
		 FROM experiment_templates WHERE id = $1 AND owner_user_id = $2`,
		templateID, ownerUserID,
	).Scan(&t.ID, &t.OwnerUserID, &t.Title, &t.Description, &t.BodyTemplate,
		&sectionsRaw, &t.ProtocolID, &tagsRaw, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query template: %w", err)
	}
	json.Unmarshal(sectionsRaw, &t.Sections)
	json.Unmarshal(tagsRaw, &t.Tags)
	return &t, nil
}

func (s *Service) UpdateTemplate(ctx context.Context, in UpdateTemplateInput) (*Template, error) {
	sectionsJSON, _ := json.Marshal(in.Sections)
	if in.Sections == nil {
		sectionsJSON = []byte("[]")
	}
	tagsJSON, _ := json.Marshal(in.Tags)
	if in.Tags == nil {
		tagsJSON = []byte("[]")
	}

	var t Template
	var sectionsRaw, tagsRaw []byte
	err := s.db.QueryRowContext(ctx,
		`UPDATE experiment_templates
		 SET description = $1, body_template = $2, sections = $3, tags = $4, updated_at = NOW()
		 WHERE id = $5 AND owner_user_id = $6
		 RETURNING id, owner_user_id, title, description, body_template, sections, protocol_id, tags, created_at, updated_at`,
		in.Description, in.BodyTemplate, sectionsJSON, tagsJSON, in.TemplateID, in.OwnerUserID,
	).Scan(&t.ID, &t.OwnerUserID, &t.Title, &t.Description, &t.BodyTemplate,
		&sectionsRaw, &t.ProtocolID, &tagsRaw, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update template: %w", err)
	}
	json.Unmarshal(sectionsRaw, &t.Sections)
	json.Unmarshal(tagsRaw, &t.Tags)
	return &t, nil
}

func (s *Service) DeleteTemplate(ctx context.Context, templateID, ownerUserID string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM experiment_templates WHERE id = $1 AND owner_user_id = $2`,
		templateID, ownerUserID,
	)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) CloneExperiment(ctx context.Context, in CloneExperimentInput) (*CloneExperimentOutput, error) {
	if strings.TrimSpace(in.NewTitle) == "" {
		return nil, fmt.Errorf("%w: newTitle is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Get source experiment effective body
	var sourceOwner, effectiveBody string
	err = tx.QueryRowContext(ctx,
		`SELECT e.owner_user_id,
			(SELECT ee.body FROM experiment_entries ee WHERE ee.experiment_id = e.id ORDER BY ee.created_at DESC LIMIT 1)
		 FROM experiments e WHERE e.id = $1`,
		in.SourceExperimentID,
	).Scan(&sourceOwner, &effectiveBody)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query source: %w", err)
	}
	if sourceOwner != in.OwnerUserID {
		return nil, ErrForbidden
	}

	// Create new experiment
	var expID string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx,
		`INSERT INTO experiments (owner_user_id, title, status) VALUES ($1, $2, 'draft') RETURNING id, created_at`,
		in.OwnerUserID, in.NewTitle,
	).Scan(&expID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert experiment: %w", err)
	}

	// Create original entry with cloned body
	var entryID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO experiment_entries (experiment_id, author_user_id, entry_type, body) VALUES ($1, $2, 'original', $3) RETURNING id`,
		expID, in.OwnerUserID, effectiveBody,
	).Scan(&entryID)
	if err != nil {
		return nil, fmt.Errorf("insert entry: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"experimentId":       expID,
		"sourceExperimentId": in.SourceExperimentID,
		"originalEntryId":    entryID,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.OwnerUserID, "experiment.cloned", "experiment", expID, payload)

	s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   in.OwnerUserID,
		ActorUserID:   in.OwnerUserID,
		DeviceID:      in.DeviceID,
		EventType:     "experiment.created",
		AggregateType: "experiment",
		AggregateID:   expID,
		Payload:       payload,
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &CloneExperimentOutput{
		ExperimentID:    expID,
		OriginalEntryID: entryID,
		Status:          "draft",
		CreatedAt:       createdAt,
	}, nil
}

func (s *Service) CreateFromTemplate(ctx context.Context, in CreateFromTemplateInput) (*CloneExperimentOutput, error) {
	// Load template
	var tmplBody string
	var tmplOwner string
	err := s.db.QueryRowContext(ctx,
		`SELECT body_template, owner_user_id FROM experiment_templates WHERE id = $1`,
		in.TemplateID,
	).Scan(&tmplBody, &tmplOwner)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query template: %w", err)
	}
	if tmplOwner != in.OwnerUserID {
		return nil, ErrForbidden
	}

	body := tmplBody
	if strings.TrimSpace(in.Body) != "" {
		body = in.Body
	}

	title := in.Title
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var expID string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx,
		`INSERT INTO experiments (owner_user_id, title, status) VALUES ($1, $2, 'draft') RETURNING id, created_at`,
		in.OwnerUserID, title,
	).Scan(&expID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert experiment: %w", err)
	}

	var entryID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO experiment_entries (experiment_id, author_user_id, entry_type, body) VALUES ($1, $2, 'original', $3) RETURNING id`,
		expID, in.OwnerUserID, body,
	).Scan(&entryID)
	if err != nil {
		return nil, fmt.Errorf("insert entry: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"experimentId":    expID,
		"templateId":      in.TemplateID,
		"originalEntryId": entryID,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.OwnerUserID, "experiment.created_from_template", "experiment", expID, payload)

	s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   in.OwnerUserID,
		ActorUserID:   in.OwnerUserID,
		DeviceID:      in.DeviceID,
		EventType:     "experiment.created",
		AggregateType: "experiment",
		AggregateID:   expID,
		Payload:       payload,
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &CloneExperimentOutput{
		ExperimentID:    expID,
		OriginalEntryID: entryID,
		Status:          "draft",
		CreatedAt:       createdAt,
	}, nil
}
