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
				 FROM protocols
				 ORDER BY updated_at DESC`
	} else {
		query = `SELECT id, owner_user_id, title, description, status, created_at, updated_at
				 FROM protocols WHERE owner_user_id = $1 OR status = 'published'
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

// ---------------------------------------------------------------------------
// Seed default protocols
// ---------------------------------------------------------------------------

type seedProtocol struct {
	Title       string
	Description string
	Body        string
}

// SeedDefaultProtocols inserts a library of standard laboratory protocols
// on first run. It is idempotent — skipped if the seed protocols already exist.
func (s *Service) SeedDefaultProtocols(ctx context.Context, systemUserID string) error {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM protocols WHERE title = 'Western Blot' AND owner_user_id = $1`,
		systemUserID,
	).Scan(&count); err != nil {
		return fmt.Errorf("check existing seed protocols: %w", err)
	}
	if count > 0 {
		return nil // already seeded
	}

	seeds := defaultProtocols()
	for _, sp := range seeds {
		_, err := s.CreateProtocol(ctx, CreateProtocolInput{
			OwnerUserID: systemUserID,
			Title:       sp.Title,
			Description: sp.Description,
			InitialBody: sp.Body,
		})
		if err != nil {
			return fmt.Errorf("seed protocol %q: %w", sp.Title, err)
		}

		// Auto-publish so they appear as ready-to-use
		// Find the protocol just created
		var pid string
		if err := s.db.QueryRowContext(ctx,
			`SELECT id FROM protocols WHERE title = $1 AND owner_user_id = $2 ORDER BY created_at DESC LIMIT 1`,
			sp.Title, systemUserID,
		).Scan(&pid); err != nil {
			return fmt.Errorf("find seeded protocol %q: %w", sp.Title, err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE protocols SET status = 'published' WHERE id = $1`, pid); err != nil {
			return fmt.Errorf("publish seeded protocol %q: %w", sp.Title, err)
		}
	}
	return nil
}

func defaultProtocols() []seedProtocol {
	return []seedProtocol{
		{
			Title:       "Western Blot",
			Description: "Standard western blot protocol for protein detection using gel electrophoresis and antibody probing.",
			Body: `# Western Blot Protocol

## Materials
- SDS-PAGE gel (appropriate percentage for target protein)
- Running buffer (25 mM Tris, 192 mM glycine, 0.1% SDS)
- Transfer buffer (25 mM Tris, 192 mM glycine, 20% methanol)
- PVDF or nitrocellulose membrane
- Blocking buffer (5% non-fat milk or BSA in TBST)
- TBST wash buffer (TBS + 0.1% Tween-20)
- Primary and secondary antibodies
- ECL detection reagent

## Procedure

### 1. Sample Preparation
1. Lyse cells in RIPA buffer supplemented with protease and phosphatase inhibitors.
2. Determine protein concentration using BCA or Bradford assay.
3. Normalize samples to equal protein concentration.
4. Add 4× Laemmli sample buffer and heat at 95°C for 5 minutes.

### 2. SDS-PAGE
1. Load equal amounts of protein (20-50 μg) per lane.
2. Include molecular weight markers.
3. Run gel at 80V through stacking gel, then 120V through resolving gel.
4. Run until dye front reaches the bottom of the gel.

### 3. Transfer
1. Activate PVDF membrane in methanol for 1 minute (skip for nitrocellulose).
2. Assemble transfer sandwich: sponge → filter paper → gel → membrane → filter paper → sponge.
3. Transfer at 100V for 60-90 minutes at 4°C (or overnight at 30V).
4. Verify transfer with Ponceau S staining.

### 4. Blocking
1. Block membrane in 5% milk/TBST for 1 hour at room temperature.

### 5. Primary Antibody
1. Dilute primary antibody in 5% BSA/TBST per manufacturer recommendation.
2. Incubate membrane overnight at 4°C with gentle rocking.

### 6. Washing
1. Wash 3× with TBST, 10 minutes each.

### 7. Secondary Antibody
1. Dilute HRP-conjugated secondary antibody in 5% milk/TBST.
2. Incubate 1 hour at room temperature.
3. Wash 3× with TBST, 10 minutes each.

### 8. Detection
1. Apply ECL substrate and image using chemiluminescence imager.
2. Adjust exposure for optimal signal-to-noise ratio.

## Notes
- For phospho-specific antibodies, block and dilute antibodies in BSA (not milk).
- Strip and re-probe membranes for loading controls (e.g., β-actin, GAPDH).`,
		},
		{
			Title:       "qPCR (Quantitative Real-Time PCR)",
			Description: "Quantitative real-time polymerase chain reaction for gene expression analysis.",
			Body: `# qPCR Protocol

## Materials
- cDNA template (from reverse transcription)
- SYBR Green Master Mix or TaqMan probes
- Forward and reverse primers (10 μM stock)
- Nuclease-free water
- 96-well or 384-well optical plates
- Optical adhesive film

## Procedure

### 1. RNA Extraction & Reverse Transcription
1. Extract total RNA using TRIzol or column-based kit.
2. Assess RNA quality (A260/A280 ≥ 1.8) and quantity.
3. Reverse-transcribe 0.5-2 μg RNA using random hexamers or oligo-dT primers.
4. Dilute cDNA 1:5 to 1:10 with nuclease-free water.

### 2. Reaction Setup (per well, 20 μL total)
| Component | Volume |
|-----------|--------|
| 2× SYBR Green Master Mix | 10 μL |
| Forward primer (10 μM) | 0.5 μL |
| Reverse primer (10 μM) | 0.5 μL |
| cDNA template | 2 μL |
| Nuclease-free water | 7 μL |

### 3. Plate Setup
1. Include no-template controls (NTC) for each primer pair.
2. Include reference gene (e.g., GAPDH, ACTB, 18S rRNA).
3. Run each sample in technical triplicate.

### 4. Cycling Conditions
| Step | Temperature | Time | Cycles |
|------|------------|------|--------|
| Initial denaturation | 95°C | 10 min | 1 |
| Denaturation | 95°C | 15 sec | 40 |
| Annealing/Extension | 60°C | 1 min | 40 |
| Melt curve | 60-95°C | continuous | 1 |

### 5. Analysis
1. Set threshold in the exponential phase.
2. Calculate ΔCt = Ct(target) − Ct(reference).
3. Calculate ΔΔCt = ΔCt(treated) − ΔCt(control).
4. Fold change = 2^(−ΔΔCt).
5. Verify single melt curve peak for each amplicon.

## Notes
- Primer efficiency should be 90-110% (validated by standard curve).
- Ensure melt curves show a single peak (no primer dimers).`,
		},
		{
			Title:       "PCR (Standard Endpoint PCR)",
			Description: "Standard polymerase chain reaction for DNA amplification and cloning.",
			Body: `# Standard PCR Protocol

## Materials
- DNA template (1-100 ng genomic DNA, or 1-10 ng plasmid)
- High-fidelity DNA polymerase (e.g., Phusion, Q5)
- 5× or 2× reaction buffer
- dNTP mix (10 mM each)
- Forward and reverse primers (10 μM)
- Nuclease-free water
- DMSO (optional, for GC-rich templates)

## Procedure

### 1. Reaction Setup (50 μL total)
| Component | Volume |
|-----------|--------|
| 5× HF Buffer | 10 μL |
| dNTPs (10 mM each) | 1 μL |
| Forward primer (10 μM) | 2.5 μL |
| Reverse primer (10 μM) | 2.5 μL |
| Template DNA | 1 μL |
| Polymerase | 0.5 μL |
| DMSO (optional) | 1.5 μL |
| Nuclease-free water | to 50 μL |

### 2. Cycling Conditions
| Step | Temperature | Time | Cycles |
|------|------------|------|--------|
| Initial denaturation | 98°C | 30 sec | 1 |
| Denaturation | 98°C | 10 sec | 30-35 |
| Annealing | Tm − 5°C | 30 sec | 30-35 |
| Extension | 72°C | 30 sec/kb | 30-35 |
| Final extension | 72°C | 5 min | 1 |
| Hold | 4°C | ∞ | — |

### 3. Verification
1. Run 5 μL on 1% agarose gel with DNA ladder.
2. Verify single band at expected size.
3. Gel-purify if needed for downstream applications.

## Notes
- Use high-fidelity polymerase for cloning applications.
- Optimize annealing temperature using gradient PCR if necessary.
- Add DMSO (3%) for GC-rich templates.`,
		},
		{
			Title:       "Cell Culture — Passaging Adherent Cells",
			Description: "Standard protocol for routine subculture of adherent mammalian cell lines.",
			Body: `# Adherent Cell Passaging Protocol

## Materials
- Complete growth medium (pre-warmed to 37°C)
- 1× PBS (Ca²⁺/Mg²⁺-free, pre-warmed)
- 0.25% Trypsin-EDTA (pre-warmed)
- 15 mL conical tubes
- New culture flasks/dishes
- Hemocytometer or automated cell counter (optional)

## Procedure

### 1. Preparation
1. Pre-warm medium, PBS, and trypsin to 37°C.
2. Examine cells under microscope for confluence (passage at 80-90%).
3. Check for contamination (turbidity, floating clumps, color change).

### 2. Detachment
1. Aspirate spent medium.
2. Rinse monolayer with 5-10 mL PBS to remove residual serum.
3. Add trypsin (1 mL per 25 cm², e.g., 3 mL for T75).
4. Incubate at 37°C for 2-5 minutes, gently tapping flask.
5. Verify detachment under microscope.

### 3. Neutralization & Counting
1. Add 2× volume of complete medium to neutralize trypsin.
2. Pipette gently to create single-cell suspension.
3. Transfer to 15 mL tube.
4. Count cells if needed (trypan blue exclusion for viability).

### 4. Reseeding
1. Seed cells at appropriate density:
   - HEK293T: 1-2 × 10⁶ cells per T75
   - HeLa: 0.5-1 × 10⁶ cells per T75
   - General: 1:3 to 1:10 split ratio
2. Add fresh complete medium to final volume.
3. Rock flask gently to distribute cells evenly.
4. Place in 37°C, 5% CO₂ incubator.

### 5. Documentation
1. Record passage number, split ratio, and date.
2. Note cell viability and morphology.

## Notes
- Do not over-trypsinize — monitor cells under microscope.
- Cells at high passage (>30) may show altered phenotype.
- Mycoplasma testing should be performed routinely.`,
		},
		{
			Title:       "Transfection — Lipofection",
			Description: "Lipid-based transfection of plasmid DNA into mammalian cells using Lipofectamine or similar reagent.",
			Body: `# Lipofection Transfection Protocol

## Materials
- Cells at 70-80% confluence in appropriate vessel
- Plasmid DNA (transfection-grade, endotoxin-free)
- Lipofectamine 2000/3000 or equivalent
- Opti-MEM reduced-serum medium
- Complete growth medium (antibiotic-free)

## Procedure (for 6-well plate, scale as needed)

### 1. Day Before Transfection
1. Seed 5 × 10⁵ cells per well in 2 mL antibiotic-free medium.
2. Cells should be 70-80% confluent at time of transfection.

### 2. Complex Preparation
**Tube A — DNA:**
1. Dilute 2.5 μg plasmid DNA in 125 μL Opti-MEM.

**Tube B — Lipid:**
1. Dilute 7.5 μL Lipofectamine 2000 in 125 μL Opti-MEM.
2. Incubate 5 minutes at room temperature.

**Combine:**
1. Add Tube A to Tube B (do NOT reverse).
2. Mix gently by pipetting.
3. Incubate 20 minutes at room temperature.

### 3. Transfection
1. Add 250 μL complex mixture dropwise to cells.
2. Swirl plate gently to mix.
3. Incubate at 37°C, 5% CO₂.
4. Replace medium after 6 hours (optional but recommended).

### 4. Analysis
1. Assess transfection efficiency at 24-48 hours.
2. For GFP reporters, check fluorescence at 24 hours.
3. For protein expression, harvest at 48-72 hours.
4. For stable selection, add selection antibiotic at 48 hours.

## Scaling Guide
| Vessel | DNA | Lipofectamine | Opti-MEM (each tube) |
|--------|-----|---------------|---------------------|
| 96-well | 0.2 μg | 0.5 μL | 25 μL |
| 24-well | 0.5 μg | 1.5 μL | 50 μL |
| 6-well | 2.5 μg | 7.5 μL | 125 μL |
| 10 cm dish | 10 μg | 30 μL | 500 μL |

## Notes
- DNA:Lipofectamine ratio may need optimization per cell line.
- Antibiotics during transfection increase toxicity.
- Use positive control (GFP plasmid) to calibrate efficiency.`,
		},
		{
			Title:       "CRISPR-Cas9 Gene Knockout",
			Description: "CRISPR/Cas9-mediated gene disruption using ribonucleoprotein (RNP) or plasmid delivery.",
			Body: `# CRISPR-Cas9 Gene Knockout Protocol

## Materials
- sgRNA (synthetic or expressed from plasmid)
- Cas9 protein (for RNP) or Cas9-expression plasmid
- Transfection reagent or electroporator
- T7 Endonuclease I (for mismatch detection)
- PCR primers flanking cut site
- Genomic DNA extraction kit

## Procedure

### 1. sgRNA Design
1. Use design tools (Benchling, CRISPOR, or CHOPCHOP).
2. Select guides with high on-target score and low off-target score.
3. Target early constitutive exons for maximum knockout efficiency.
4. Design 2-3 guides per gene for redundancy.

### 2. RNP Assembly (if using protein)
1. Mix 100 pmol Cas9 protein with 120 pmol sgRNA.
2. Incubate at room temperature for 10-15 minutes to form complex.

### 3. Delivery
**Electroporation (recommended for hard-to-transfect cells):**
1. Resuspend 2 × 10⁵ cells in electroporation buffer.
2. Add RNP complex.
3. Electroporate per optimized settings for cell type.
4. Transfer to pre-warmed medium.

**Lipofection (for easy-to-transfect cells):**
1. Follow standard lipofection protocol with Cas9 plasmid + sgRNA plasmid.
2. Use 1:1 molar ratio.

### 4. Validation — T7 Endonuclease I Assay
1. Extract genomic DNA at 48-72 hours post-transfection.
2. PCR-amplify region spanning the cut site (~500-800 bp amplicon).
3. Denature and re-anneal PCR product (95°C 5 min, cool slowly).
4. Digest with T7EI for 30 minutes at 37°C.
5. Run on 2% agarose gel — cleavage bands indicate editing.

### 5. Clonal Isolation
1. Single-cell sort by FACS or limiting dilution.
2. Expand clones for 2-3 weeks.
3. Screen clones by Sanger sequencing of the target site.
4. Confirm knockout by Western blot.

## Notes
- Include a non-targeting sgRNA control.
- Check top 3-5 predicted off-target sites in validated clones.
- For essential genes, consider CRISPRi (knockdown) instead.`,
		},
		{
			Title:       "Flow Cytometry — Surface Staining",
			Description: "Standard surface marker staining protocol for flow cytometric analysis.",
			Body: `# Flow Cytometry Surface Staining Protocol

## Materials
- Single-cell suspension (1-5 × 10⁶ cells per condition)
- FACS buffer (PBS + 2% FBS + 0.1% sodium azide)
- Fluorochrome-conjugated antibodies
- Fc block (anti-CD16/CD32 for mouse, Human TruStain FcX for human)
- DAPI or propidium iodide (viability dye)
- 5 mL round-bottom tubes or 96-well V-bottom plate
- Compensation beads (for multi-color panels)

## Procedure

### 1. Sample Preparation
1. Prepare single-cell suspension (filter through 70 μm strainer if needed).
2. Count cells and aliquot 1 × 10⁶ cells per condition.
3. Wash once with FACS buffer (300 × g, 5 min).

### 2. Fc Blocking
1. Resuspend pellet in 50 μL FACS buffer.
2. Add Fc block per manufacturer dilution.
3. Incubate 10 minutes on ice.

### 3. Antibody Staining
1. Add fluorochrome-conjugated antibodies at titrated concentrations.
2. Mix gently and incubate 20-30 minutes on ice, protected from light.
3. Wash 2× with 1 mL FACS buffer (300 × g, 5 min).

### 4. Viability Staining
1. Resuspend in 200-500 μL FACS buffer.
2. Add DAPI (0.1 μg/mL) or PI (0.5 μg/mL) immediately before acquisition.

### 5. Controls
- Unstained cells (autofluorescence baseline)
- Single-stain controls (for compensation)
- FMO (Fluorescence-Minus-One) controls for gating
- Isotype controls (optional)

### 6. Acquisition
1. Set up instrument with compensation using single-stain beads.
2. Acquire ≥10,000 events in the live gate.
3. Gate: FSC/SSC → singlets → live cells → population of interest.

## Notes
- Keep cells cold and protected from light throughout.
- Use fixable viability dyes if fixation is needed before acquisition.
- Titrate all antibodies before use to determine optimal concentration.`,
		},
		{
			Title:       "ELISA (Enzyme-Linked Immunosorbent Assay)",
			Description: "Sandwich ELISA protocol for quantitative detection of soluble proteins/cytokines.",
			Body: `# Sandwich ELISA Protocol

## Materials
- ELISA kit or matched antibody pair (capture + detection)
- 96-well high-binding ELISA plate
- Coating buffer (0.1 M sodium carbonate, pH 9.5)
- Wash buffer (PBS + 0.05% Tween-20)
- Blocking buffer (1% BSA in PBS)
- Detection antibody (biotinylated)
- HRP-Streptavidin conjugate
- TMB substrate solution
- Stop solution (2N H₂SO₄)
- Recombinant protein standard

## Procedure

### 1. Coat Plate (Day 1)
1. Dilute capture antibody to 1-4 μg/mL in coating buffer.
2. Add 100 μL per well.
3. Seal plate and incubate overnight at 4°C.

### 2. Block (Day 2)
1. Wash plate 3× with wash buffer.
2. Add 200 μL blocking buffer per well.
3. Incubate 1-2 hours at room temperature.

### 3. Standards & Samples
1. Prepare 7-point serial dilution of standard (plus blank).
2. Dilute samples as needed in blocking buffer.
3. Wash plate 3× with wash buffer.
4. Add 100 μL standard or sample per well (in duplicate).
5. Incubate 2 hours at room temperature (or overnight at 4°C).

### 4. Detection Antibody
1. Wash plate 4× with wash buffer.
2. Dilute detection antibody per kit instructions.
3. Add 100 μL per well.
4. Incubate 1 hour at room temperature.

### 5. HRP-Streptavidin
1. Wash plate 4× with wash buffer.
2. Dilute HRP-Streptavidin per kit instructions.
3. Add 100 μL per well.
4. Incubate 30 minutes at room temperature, protected from light.

### 6. Development
1. Wash plate 5× with wash buffer.
2. Add 100 μL TMB substrate per well.
3. Incubate 5-30 minutes (monitor blue color development).
4. Add 50 μL stop solution (yellow color).
5. Read absorbance at 450 nm (reference 570 nm).

### 7. Analysis
1. Subtract blank from all readings.
2. Plot 4-parameter logistic (4PL) standard curve.
3. Interpolate sample concentrations from curve.

## Notes
- Do not let wells dry out between steps.
- Consistent wash technique is critical for reproducibility.
- Samples outside the linear range should be re-tested at different dilutions.`,
		},
		{
			Title:       "Immunofluorescence (IF) Staining",
			Description: "Immunofluorescence staining of fixed cells or tissue sections for confocal/fluorescence microscopy.",
			Body: `# Immunofluorescence Staining Protocol

## Materials
- Cells on coverslips or chamber slides
- 4% paraformaldehyde (PFA) in PBS
- Permeabilization buffer (0.1-0.5% Triton X-100 in PBS)
- Blocking buffer (5% normal goat serum + 0.1% Triton X-100 in PBS)
- Primary antibodies
- Fluorochrome-conjugated secondary antibodies
- DAPI (1 μg/mL in PBS)
- Mounting medium (anti-fade, e.g., ProLong Gold)
- Microscope slides

## Procedure

### 1. Fixation
1. Aspirate medium and wash cells with PBS.
2. Fix with 4% PFA for 15 minutes at room temperature.
3. Wash 3× with PBS, 5 minutes each.

### 2. Permeabilization
1. Incubate with 0.1% Triton X-100 in PBS for 10 minutes.
2. Wash 3× with PBS, 5 minutes each.

### 3. Blocking
1. Block with 5% normal goat serum in PBS for 1 hour at room temperature.

### 4. Primary Antibody
1. Dilute primary antibody in blocking buffer per manufacturer recommendation.
2. Add to sample and incubate overnight at 4°C in a humidified chamber.
3. Wash 3× with PBS, 5 minutes each.

### 5. Secondary Antibody
1. Dilute fluorescent secondary antibody (1:500 to 1:1000) in blocking buffer.
2. Incubate 1 hour at room temperature, protected from light.
3. Wash 3× with PBS, 5 minutes each.

### 6. Nuclear Staining
1. Incubate with DAPI (1 μg/mL) for 5 minutes.
2. Wash 2× with PBS.

### 7. Mounting
1. Mount coverslip on slide with anti-fade mounting medium.
2. Seal edges with nail polish (optional, for non-hardening media).
3. Allow to cure overnight at room temperature in the dark.

### 8. Imaging
1. Image on fluorescence or confocal microscope.
2. Use appropriate filter sets for each fluorochrome.
3. Include secondary-only controls to determine background.

## Notes
- Use methanol fixation (−20°C, 10 min) for some nuclear/cytoskeletal antigens.
- Match secondary antibody host to blocking serum host.
- Keep samples protected from light after applying fluorescent antibodies.`,
		},
		{
			Title:       "Bacterial Transformation",
			Description: "Heat-shock transformation of chemically competent E. coli with plasmid DNA.",
			Body: `# Bacterial Transformation Protocol

## Materials
- Chemically competent E. coli (DH5α, TOP10, or Stbl3)
- Plasmid DNA or ligation reaction
- SOC medium (pre-warmed to 37°C)
- LB agar plates with appropriate antibiotic
- 42°C water bath
- Ice

## Procedure

### 1. Thaw Competent Cells
1. Remove competent cells from −80°C.
2. Thaw on ice for 20-30 minutes. Do NOT use hands to warm.

### 2. DNA Addition
1. Add 1-5 μL plasmid DNA (1-100 ng) or 5-10 μL ligation mix to cells.
2. Flick tube gently 4-5 times to mix. Do NOT pipette up and down.
3. Incubate on ice for 30 minutes.

### 3. Heat Shock
1. Transfer tube to 42°C water bath for exactly 45 seconds.
2. Immediately transfer back to ice for 2 minutes.

### 4. Recovery
1. Add 250 μL room-temperature SOC medium.
2. Incubate at 37°C for 1 hour with shaking (225 rpm).

### 5. Plating
1. Spread 50-200 μL on pre-warmed LB-antibiotic plates.
2. For low-efficiency transformations, pellet cells and resuspend in 100 μL before plating.
3. Incubate plates inverted overnight at 37°C.

### 6. Colony Screening
1. Pick 4-8 colonies into LB + antibiotic.
2. Grow overnight at 37°C.
3. Miniprep plasmid DNA.
4. Verify by restriction digest and/or sequencing.

## Notes
- Always include a positive control (uncut plasmid) and negative control (no DNA).
- For ligation reactions, use maximum 10% of total volume to avoid salt inhibition.
- Stbl3 cells recommended for lentiviral plasmids (recombination-resistant).`,
		},
		{
			Title:       "RNA Extraction (TRIzol Method)",
			Description: "Total RNA isolation from cultured cells using TRIzol reagent and chloroform extraction.",
			Body: `# RNA Extraction — TRIzol Protocol

## Materials
- TRIzol reagent
- Chloroform
- Isopropanol
- 75% ethanol (made with nuclease-free water)
- Nuclease-free water
- RNase-free tubes and tips

## Procedure

### 1. Cell Lysis
1. Aspirate medium from cells.
2. Add 1 mL TRIzol per 10 cm² surface area (or per 5-10 × 10⁶ cells).
3. Pipette up and down to lyse. Pass through 21G needle if viscous.
4. Incubate 5 minutes at room temperature.

### 2. Phase Separation
1. Add 200 μL chloroform per 1 mL TRIzol.
2. Shake vigorously by hand for 15 seconds.
3. Incubate 2-3 minutes at room temperature.
4. Centrifuge at 12,000 × g for 15 minutes at 4°C.

### 3. RNA Precipitation
1. Transfer the upper aqueous phase (~500 μL) to a new tube.
   - **CRITICAL:** Do not disturb the interphase.
2. Add 500 μL isopropanol.
3. Incubate 10 minutes at room temperature.
4. Centrifuge at 12,000 × g for 10 minutes at 4°C.

### 4. RNA Wash
1. Remove supernatant carefully. The pellet may be invisible.
2. Add 1 mL 75% ethanol.
3. Vortex briefly and centrifuge at 7,500 × g for 5 minutes at 4°C.
4. Repeat wash step once.

### 5. Resuspension
1. Air-dry pellet for 5-10 minutes (do NOT over-dry).
2. Resuspend in 20-50 μL nuclease-free water.
3. Incubate at 55-60°C for 10 minutes to dissolve.

### 6. Quality Assessment
1. Measure A260/A280 (target ≥ 1.8) and A260/A230 (target ≥ 1.5).
2. Run on Bioanalyzer/TapeStation for RIN score (target ≥ 7).
3. Store at −80°C.

## Notes
- Work in an RNase-free environment. Clean bench and pipettes.
- Use barrier tips to prevent aerosol contamination.
- Add 1 μL glycogen (20 μg/μL) as carrier for low-input samples.`,
		},
		{
			Title:       "Plasmid DNA Purification (Miniprep)",
			Description: "Small-scale alkaline lysis plasmid DNA purification from overnight bacterial cultures.",
			Body: `# Plasmid Miniprep Protocol

## Materials
- Overnight bacterial culture (2-5 mL LB + antibiotic)
- Miniprep kit (or manual buffers below)
  - Buffer P1: 50 mM Tris-HCl pH 8.0, 10 mM EDTA, 100 μg/mL RNase A
  - Buffer P2: 200 mM NaOH, 1% SDS
  - Buffer N3: 4.2 M Gu-HCl, 0.9 M potassium acetate, pH 4.8
- Wash buffer (with ethanol added)
- Elution buffer (10 mM Tris-HCl pH 8.5) or nuclease-free water

## Procedure

### 1. Harvest Bacteria
1. Transfer 1.5-5 mL overnight culture to microcentrifuge tube.
2. Centrifuge at 6,800 × g for 3 minutes.
3. Discard supernatant completely.

### 2. Resuspend
1. Add 250 μL Buffer P1.
2. Vortex or pipette until pellet is fully resuspended (no clumps).

### 3. Lyse
1. Add 250 μL Buffer P2.
2. Invert tube 4-6 times gently. Do NOT vortex.
3. Incubate at room temperature for no more than 5 minutes.

### 4. Neutralize
1. Add 350 μL Buffer N3.
2. Invert tube 4-6 times immediately.
3. Centrifuge at 17,900 × g for 10 minutes.

### 5. Bind, Wash, Elute
1. Transfer supernatant to spin column.
2. Centrifuge 1 minute, discard flow-through.
3. Add 750 μL wash buffer, centrifuge 1 minute.
4. Discard flow-through, centrifuge 1 minute to dry.
5. Transfer column to clean tube.
6. Add 50 μL elution buffer, incubate 1 minute, centrifuge 1 minute.

### 6. Quantification
1. Measure concentration by Nanodrop (A260).
2. Expected yield: 5-20 μg from 5 mL culture.
3. Verify by restriction digest if needed.

## Notes
- Always use fresh overnight cultures (12-16 hours).
- Do not exceed 5 minutes lysis time to avoid genomic DNA contamination.
- Low-copy plasmids may require midiprep for adequate yield.`,
		},
		{
			Title:       "Gel Electrophoresis (Agarose)",
			Description: "Agarose gel electrophoresis for separation and visualization of DNA fragments.",
			Body: `# Agarose Gel Electrophoresis Protocol

## Materials
- Agarose (molecular biology grade)
- 1× TAE or TBE buffer
- DNA loading dye (6×)
- DNA ladder/marker
- Ethidium bromide or SYBR Safe
- Gel casting tray, combs, and electrophoresis tank
- UV transilluminator or blue light imager

## Procedure

### 1. Gel Preparation
1. Choose agarose percentage based on fragment size:
   | Fragment Size | Agarose % |
   |--------------|-----------|
   | >2 kb | 0.7% |
   | 0.5-2 kb | 1.0% |
   | 0.2-1 kb | 1.5% |
   | <0.5 kb | 2.0% |

2. Weigh agarose and add to 1× TAE/TBE in an Erlenmeyer flask.
3. Microwave until fully dissolved (swirl every 30 seconds).
4. Cool to ~55°C.
5. Add SYBR Safe (1:10,000) or EtBr (0.5 μg/mL).
6. Pour into casting tray with comb(s).
7. Allow to solidify for 20-30 minutes.

### 2. Sample Preparation
1. Add loading dye to DNA samples.
2. Mix by pipetting.

### 3. Electrophoresis
1. Place gel in tank and submerge in 1× buffer.
2. Load samples and DNA ladder.
3. Run at 5-8 V/cm (e.g., 100V for a standard mini-gel).
4. Run for 30-60 minutes (until dye front is 2/3 through gel).

### 4. Visualization
1. Image under UV or blue light.
2. Verify band sizes against ladder.
3. Photograph for documentation.

## Notes
- TAE provides better resolution for large fragments; TBE for small.
- EtBr is mutagenic — use SYBR Safe for a safer alternative.
- For gel extraction, minimize UV exposure time to prevent DNA damage.`,
		},
		{
			Title:       "Protein Expression (E. coli, IPTG Induction)",
			Description: "Recombinant protein expression in E. coli using IPTG-inducible T7 promoter systems.",
			Body: `# Protein Expression Protocol (E. coli)

## Materials
- Expression strain (BL21(DE3), Rosetta, etc.)
- Expression plasmid with T7 promoter (pET, pRSET, etc.)
- LB medium with appropriate antibiotic
- IPTG (1 M stock, filter-sterilized)
- Spectrophotometer

## Procedure

### 1. Starter Culture
1. Inoculate a single colony into 10 mL LB + antibiotic.
2. Grow overnight at 37°C, 220 rpm.

### 2. Growth Phase
1. Dilute overnight culture 1:100 into fresh LB + antibiotic.
2. Grow at 37°C, 220 rpm until OD₆₀₀ = 0.4-0.6 (log phase).
3. This typically takes 2-3 hours.

### 3. Induction
1. Remove a 1 mL "pre-induction" sample (pellet and store at −20°C).
2. Add IPTG to final concentration:
   - Standard: 0.5-1.0 mM
   - Low-level: 0.1 mM (for solubility)
3. Induce at desired temperature:
   | Temperature | Duration | Best For |
   |------------|----------|----------|
   | 37°C | 3-4 hours | High yield, may form inclusion bodies |
   | 25°C | 6-8 hours | Better solubility |
   | 18°C | Overnight | Best solubility |

### 4. Harvest
1. Remove a 1 mL "post-induction" sample.
2. Centrifuge culture at 4,000 × g for 20 minutes at 4°C.
3. Discard supernatant.
4. Flash-freeze pellet in liquid nitrogen.
5. Store at −80°C.

### 5. Quick Solubility Check
1. Resuspend pre- and post-induction pellets in SDS sample buffer.
2. Boil 5 minutes.
3. Run on SDS-PAGE to confirm expression.
4. For solubility: Lyse a small aliquot (sonication + spin) and compare supernatant vs. pellet by SDS-PAGE.

## Notes
- Optimize IPTG concentration and temperature for each construct.
- Co-expression of chaperones can improve solubility (e.g., GroEL/ES).
- Auto-induction media can be used for walk-away overnight expression.`,
		},
		{
			Title:       "Cell Freezing and Thawing",
			Description: "Cryopreservation protocol for mammalian cell banking and recovery from liquid nitrogen storage.",
			Body: `# Cell Freezing and Thawing Protocol

## Materials

### For Freezing
- Healthy cells at log-phase growth (>90% viability)
- Complete growth medium
- Freezing medium (90% FBS + 10% DMSO, or commercial alternatives)
- Cryovials (1-2 mL)
- Mr. Frosty or controlled-rate freezer
- Liquid nitrogen storage

### For Thawing
- 37°C water bath
- Pre-warmed complete growth medium
- 15 mL conical tubes
- Culture vessels

## Freezing Procedure

### 1. Preparation
1. Prepare freezing medium fresh (keep cold).
2. Label cryovials with cell line, passage number, date, initials, and vial count.

### 2. Harvest Cells
1. Detach cells as per passaging protocol.
2. Count cells and assess viability (>90% required).
3. Centrifuge at 300 × g for 5 minutes.

### 3. Resuspend and Aliquot
1. Resuspend pellet in cold freezing medium at 1-5 × 10⁶ cells/mL.
2. Aliquot 1 mL per cryovial.
3. Work quickly — DMSO is toxic to cells at room temperature.

### 4. Controlled Freezing
1. Place vials in Mr. Frosty (isopropanol jacket) at −80°C.
2. Rate: approximately −1°C/minute.
3. Transfer to liquid nitrogen within 24-72 hours.

## Thawing Procedure

### 1. Rapid Thaw
1. Remove cryovial from liquid nitrogen.
2. Thaw rapidly in 37°C water bath (~1-2 minutes).
3. Swirl gently until small ice crystal remains.

### 2. DMSO Removal
1. Transfer cells to 15 mL tube with 9 mL pre-warmed medium.
2. Centrifuge at 300 × g for 5 minutes.
3. Aspirate supernatant and resuspend in fresh medium.

### 3. Culture
1. Plate in appropriate vessel.
2. Check attachment/viability at 24 hours.
3. Change medium the next day to remove dead cells and debris.
4. Passage when confluent — do NOT use for experiments for at least 1-2 passages.

## Notes
- Always wear face shield and cryogloves when handling liquid nitrogen.
- Create a cell banking hierarchy: Master → Working → Experimental.
- Record all freeze/thaw events in cell line log.`,
		},
	}
}

