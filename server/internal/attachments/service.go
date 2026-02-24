package attachments

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

const (
	findingTypeInitiatedStale             = "initiated_stale"
	findingTypeCompletedMissingChecksum   = "completed_missing_checksum"
	findingTypeCompletedMissingObject     = "completed_missing_object"
	findingTypeOrphanObject               = "orphan_object"
	findingTypeCompletedIntegrityMismatch = "completed_object_integrity_mismatch"
	findingTypeObjectProbeFailed          = "object_probe_failed"
	findingTypeObjectListingFailed        = "object_listing_failed"
)

type Service struct {
	db             *sql.DB
	sync           *syncer.Service
	signer         URLSigner
	inspector      ObjectStoreInspector
	uploadURLTTL   time.Duration
	downloadURLTTL time.Duration
}

type InitiateInput struct {
	ExperimentID string
	OwnerUserID  string
	DeviceID     string
	ObjectKey    string
	SizeBytes    int64
	MimeType     string
}

type InitiateOutput struct {
	AttachmentID string    `json:"attachmentId"`
	ExperimentID string    `json:"experimentId"`
	ObjectKey    string    `json:"objectKey"`
	UploadURL    string    `json:"uploadUrl"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CreatedAt    time.Time `json:"createdAt"`
}

type CompleteInput struct {
	AttachmentID string
	OwnerUserID  string
	DeviceID     string
	Checksum     string
	SizeBytes    int64
}

type CompleteOutput struct {
	AttachmentID string    `json:"attachmentId"`
	Status       string    `json:"status"`
	CompletedAt  time.Time `json:"completedAt"`
}

type DownloadInput struct {
	AttachmentID string
	ViewerUserID string
	ViewerRole   string
}

type DownloadOutput struct {
	AttachmentID string    `json:"attachmentId"`
	ObjectKey    string    `json:"objectKey"`
	DownloadURL  string    `json:"downloadUrl"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type ReconcileInput struct {
	ActorUserID string
	StaleAfter  time.Duration
	Limit       int
}

type ReconcileOutput struct {
	RunID                   string    `json:"runId"`
	StartedAt               time.Time `json:"startedAt"`
	FinishedAt              time.Time `json:"finishedAt"`
	StaleInitiatedCount     int       `json:"staleInitiatedCount"`
	MissingChecksumCount    int       `json:"missingChecksumCount"`
	MissingObjectCount      int       `json:"missingObjectCount"`
	OrphanObjectCount       int       `json:"orphanObjectCount"`
	IntegrityMismatchCount  int       `json:"integrityMismatchCount"`
	ObjectProbeErrorCount   int       `json:"objectProbeErrorCount"`
	ObjectListingErrorCount int       `json:"objectListingErrorCount"`
	TotalFindingsCreated    int       `json:"totalFindingsCreated"`
}

func NewService(db *sql.DB, syncService *syncer.Service, signer URLSigner, inspector ObjectStoreInspector, uploadTTL, downloadTTL time.Duration) *Service {
	if uploadTTL <= 0 {
		uploadTTL = 15 * time.Minute
	}
	if downloadTTL <= 0 {
		downloadTTL = 15 * time.Minute
	}
	if inspector == nil {
		inspector = NewSignedURLObjectInspector(signer, "", 10*time.Second)
	}
	return &Service{
		db:             db,
		sync:           syncService,
		signer:         signer,
		inspector:      inspector,
		uploadURLTTL:   uploadTTL,
		downloadURLTTL: downloadTTL,
	}
}

func (s *Service) Initiate(ctx context.Context, in InitiateInput) (InitiateOutput, error) {
	if strings.TrimSpace(in.ExperimentID) == "" || strings.TrimSpace(in.OwnerUserID) == "" || strings.TrimSpace(in.ObjectKey) == "" || strings.TrimSpace(in.MimeType) == "" || in.SizeBytes <= 0 {
		return InitiateOutput{}, ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InitiateOutput{}, fmt.Errorf("begin initiate attachment tx: %w", err)
	}
	defer tx.Rollback()

	ownerID, err := experimentOwner(ctx, tx, in.ExperimentID)
	if err != nil {
		return InitiateOutput{}, err
	}
	if ownerID != in.OwnerUserID {
		return InitiateOutput{}, ErrForbidden
	}

	out := InitiateOutput{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO attachments (
			experiment_id,
			uploader_user_id,
			object_key,
			size_bytes,
			mime_type,
			status
		) VALUES (
			$1::uuid,
			$2::uuid,
			$3,
			$4,
			$5,
			'initiated'
		)
		RETURNING id::text, experiment_id::text, object_key, created_at
	`, in.ExperimentID, in.OwnerUserID, strings.TrimSpace(in.ObjectKey), in.SizeBytes, strings.TrimSpace(in.MimeType)).Scan(
		&out.AttachmentID,
		&out.ExperimentID,
		&out.ObjectKey,
		&out.CreatedAt,
	)
	if err != nil {
		return InitiateOutput{}, fmt.Errorf("insert attachment metadata: %w", err)
	}

	out.ExpiresAt = time.Now().UTC().Add(s.uploadURLTTL)
	uploadURL, err := s.signer.SignUpload(out.ObjectKey, out.ExpiresAt)
	if err != nil {
		return InitiateOutput{}, fmt.Errorf("sign upload URL: %w", err)
	}
	out.UploadURL = uploadURL

	if err := internaldb.AppendAuditEvent(ctx, tx, in.OwnerUserID, "attachment.initiate", "attachment", out.AttachmentID, map[string]any{
		"experimentId": in.ExperimentID,
		"objectKey":    out.ObjectKey,
		"sizeBytes":    in.SizeBytes,
		"mimeType":     in.MimeType,
	}); err != nil {
		return InitiateOutput{}, err
	}

	if _, err := s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   in.OwnerUserID,
		ActorUserID:   in.OwnerUserID,
		DeviceID:      in.DeviceID,
		EventType:     "attachment.initiated",
		AggregateType: "attachment",
		AggregateID:   out.AttachmentID,
		Payload: map[string]any{
			"experimentId": in.ExperimentID,
			"objectKey":    out.ObjectKey,
		},
	}); err != nil {
		return InitiateOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return InitiateOutput{}, fmt.Errorf("commit initiate attachment tx: %w", err)
	}

	return out, nil
}

func (s *Service) Complete(ctx context.Context, in CompleteInput) (CompleteOutput, error) {
	if strings.TrimSpace(in.AttachmentID) == "" || strings.TrimSpace(in.OwnerUserID) == "" || strings.TrimSpace(in.Checksum) == "" || in.SizeBytes <= 0 {
		return CompleteOutput{}, ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompleteOutput{}, fmt.Errorf("begin complete attachment tx: %w", err)
	}
	defer tx.Rollback()

	var (
		experimentID string
		ownerID      string
		sizeBytes    int64
		status       string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT
			a.experiment_id::text,
			e.owner_user_id::text,
			a.size_bytes,
			a.status
		FROM attachments a
		JOIN experiments e ON e.id = a.experiment_id
		WHERE a.id = $1::uuid
		FOR UPDATE
	`, in.AttachmentID).Scan(&experimentID, &ownerID, &sizeBytes, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CompleteOutput{}, ErrNotFound
		}
		return CompleteOutput{}, fmt.Errorf("load attachment metadata: %w", err)
	}
	if ownerID != in.OwnerUserID {
		return CompleteOutput{}, ErrForbidden
	}
	if sizeBytes != in.SizeBytes {
		return CompleteOutput{}, ErrInvalidInput
	}
	if status == "completed" {
		return CompleteOutput{}, ErrInvalidInput
	}

	out := CompleteOutput{}
	err = tx.QueryRowContext(ctx, `
		UPDATE attachments
		SET checksum = $2,
			status = 'completed',
			completed_at = NOW()
		WHERE id = $1::uuid
		RETURNING id::text, status, completed_at
	`, in.AttachmentID, strings.TrimSpace(in.Checksum)).Scan(&out.AttachmentID, &out.Status, &out.CompletedAt)
	if err != nil {
		return CompleteOutput{}, fmt.Errorf("complete attachment metadata: %w", err)
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, in.OwnerUserID, "attachment.complete", "attachment", out.AttachmentID, map[string]any{
		"experimentId": experimentID,
		"checksum":     in.Checksum,
		"sizeBytes":    in.SizeBytes,
	}); err != nil {
		return CompleteOutput{}, err
	}

	if _, err := s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   in.OwnerUserID,
		ActorUserID:   in.OwnerUserID,
		DeviceID:      in.DeviceID,
		EventType:     "attachment.completed",
		AggregateType: "attachment",
		AggregateID:   out.AttachmentID,
		Payload: map[string]any{
			"experimentId": experimentID,
		},
	}); err != nil {
		return CompleteOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return CompleteOutput{}, fmt.Errorf("commit complete attachment tx: %w", err)
	}

	return out, nil
}

func (s *Service) Download(ctx context.Context, in DownloadInput) (DownloadOutput, error) {
	if strings.TrimSpace(in.AttachmentID) == "" || strings.TrimSpace(in.ViewerUserID) == "" {
		return DownloadOutput{}, ErrInvalidInput
	}

	var (
		out              DownloadOutput
		experimentOwner  string
		experimentStatus string
		attachmentStatus string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT
			a.id::text,
			a.object_key,
			e.owner_user_id::text,
			e.status,
			a.status
		FROM attachments a
		JOIN experiments e ON e.id = a.experiment_id
		WHERE a.id = $1::uuid
	`, in.AttachmentID).Scan(&out.AttachmentID, &out.ObjectKey, &experimentOwner, &experimentStatus, &attachmentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DownloadOutput{}, ErrNotFound
		}
		return DownloadOutput{}, fmt.Errorf("load attachment for download: %w", err)
	}

	if !(in.ViewerUserID == experimentOwner || (in.ViewerRole == "admin" && experimentStatus == "completed")) {
		return DownloadOutput{}, ErrForbidden
	}
	if attachmentStatus != "completed" {
		return DownloadOutput{}, ErrInvalidInput
	}

	out.ExpiresAt = time.Now().UTC().Add(s.downloadURLTTL)
	downloadURL, err := s.signer.SignDownload(out.ObjectKey, out.ExpiresAt)
	if err != nil {
		return DownloadOutput{}, fmt.Errorf("sign download URL: %w", err)
	}
	out.DownloadURL = downloadURL

	return out, nil
}

// ---------------------------------------------------------------------------
// List attachments for an experiment
// ---------------------------------------------------------------------------

type AttachmentInfo struct {
	ID           string     `json:"id"`
	ExperimentID string     `json:"experimentId"`
	ObjectKey    string     `json:"objectKey"`
	SizeBytes    int64      `json:"sizeBytes"`
	MimeType     string     `json:"mimeType"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
}

func (s *Service) ListByExperiment(ctx context.Context, experimentID, viewerUserID, viewerRole string) ([]AttachmentInfo, error) {
	if strings.TrimSpace(experimentID) == "" {
		return nil, ErrInvalidInput
	}

	// Check the viewer has access (owner or admin on completed experiments only)
	var ownerID, experimentStatus string
	err := s.db.QueryRowContext(ctx,
		`SELECT owner_user_id::text, status FROM experiments WHERE id = $1::uuid`, experimentID,
	).Scan(&ownerID, &experimentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("check experiment access: %w", err)
	}
	if ownerID != viewerUserID {
		if viewerRole != "admin" || experimentStatus != "completed" {
			return nil, ErrForbidden
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, experiment_id::text, object_key, size_bytes, mime_type, status, created_at, completed_at
		FROM attachments
		WHERE experiment_id = $1::uuid
		ORDER BY created_at DESC
	`, experimentID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()

	var out []AttachmentInfo
	for rows.Next() {
		var a AttachmentInfo
		if err := rows.Scan(&a.ID, &a.ExperimentID, &a.ObjectKey, &a.SizeBytes, &a.MimeType, &a.Status, &a.CreatedAt, &a.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		out = append(out, a)
	}
	if out == nil {
		out = []AttachmentInfo{}
	}
	return out, nil
}

func (s *Service) Reconcile(ctx context.Context, in ReconcileInput) (ReconcileOutput, error) {
	if strings.TrimSpace(in.ActorUserID) == "" {
		return ReconcileOutput{}, ErrInvalidInput
	}
	if in.StaleAfter <= 0 {
		in.StaleAfter = 24 * time.Hour
	}
	if in.Limit <= 0 {
		in.Limit = 500
	}
	if in.Limit > 2000 {
		in.Limit = 2000
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconcileOutput{}, fmt.Errorf("begin reconcile tx: %w", err)
	}
	defer tx.Rollback()

	out := ReconcileOutput{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO attachment_reconcile_runs (
			triggered_by_user_id,
			started_at,
			stale_after_seconds,
			scan_limit
		) VALUES (
			$1::uuid,
			NOW(),
			$2,
			$3
		)
		RETURNING id::text, started_at
	`, in.ActorUserID, int64(in.StaleAfter.Seconds()), in.Limit).Scan(&out.RunID, &out.StartedAt)
	if err != nil {
		return ReconcileOutput{}, fmt.Errorf("insert reconcile run: %w", err)
	}

	staleCutoff := time.Now().UTC().Add(-in.StaleAfter)
	staleRows, err := tx.QueryContext(ctx, `
		SELECT id::text
		FROM attachments
		WHERE status = 'initiated'
		  AND created_at < $1
		ORDER BY created_at ASC
		LIMIT $2
	`, staleCutoff, in.Limit)
	if err != nil {
		return ReconcileOutput{}, fmt.Errorf("query stale initiated attachments: %w", err)
	}
	var staleAttachmentIDs []string
	for staleRows.Next() {
		var attachmentID string
		if err := staleRows.Scan(&attachmentID); err != nil {
			staleRows.Close()
			return ReconcileOutput{}, fmt.Errorf("scan stale initiated attachment: %w", err)
		}
		staleAttachmentIDs = append(staleAttachmentIDs, attachmentID)
	}
	if err := staleRows.Err(); err != nil {
		staleRows.Close()
		return ReconcileOutput{}, fmt.Errorf("iterate stale initiated attachments: %w", err)
	}
	if err := staleRows.Close(); err != nil {
		return ReconcileOutput{}, fmt.Errorf("close stale initiated attachments rows: %w", err)
	}

	for _, attachmentID := range staleAttachmentIDs {
		if err := insertReconcileFinding(ctx, tx, out.RunID, &attachmentID, findingTypeInitiatedStale, map[string]any{
			"cutoff": staleCutoff.Format(time.RFC3339Nano),
		}); err != nil {
			return ReconcileOutput{}, fmt.Errorf("insert stale initiated finding: %w", err)
		}
		out.StaleInitiatedCount++
	}

	missingRows, err := tx.QueryContext(ctx, `
		SELECT id::text
		FROM attachments
		WHERE status = 'completed'
		  AND (checksum IS NULL OR BTRIM(checksum) = '')
		ORDER BY completed_at DESC NULLS LAST
		LIMIT $1
	`, in.Limit)
	if err != nil {
		return ReconcileOutput{}, fmt.Errorf("query missing checksum attachments: %w", err)
	}
	var missingChecksumAttachmentIDs []string
	for missingRows.Next() {
		var attachmentID string
		if err := missingRows.Scan(&attachmentID); err != nil {
			missingRows.Close()
			return ReconcileOutput{}, fmt.Errorf("scan missing checksum attachment: %w", err)
		}
		missingChecksumAttachmentIDs = append(missingChecksumAttachmentIDs, attachmentID)
	}
	if err := missingRows.Err(); err != nil {
		missingRows.Close()
		return ReconcileOutput{}, fmt.Errorf("iterate missing checksum attachments: %w", err)
	}
	if err := missingRows.Close(); err != nil {
		return ReconcileOutput{}, fmt.Errorf("close missing checksum rows: %w", err)
	}

	for _, attachmentID := range missingChecksumAttachmentIDs {
		if err := insertReconcileFinding(ctx, tx, out.RunID, &attachmentID, findingTypeCompletedMissingChecksum, map[string]any{}); err != nil {
			return ReconcileOutput{}, fmt.Errorf("insert missing checksum finding: %w", err)
		}
		out.MissingChecksumCount++
	}

	completedRows, err := tx.QueryContext(ctx, `
		SELECT id::text, object_key, size_bytes, COALESCE(checksum, '')
		FROM attachments
		WHERE status = 'completed'
		ORDER BY completed_at DESC NULLS LAST
		LIMIT $1
	`, in.Limit)
	if err != nil {
		return ReconcileOutput{}, fmt.Errorf("query completed attachments for object drift: %w", err)
	}
	type completedAttachment struct {
		id        string
		objectKey string
		sizeBytes int64
		checksum  string
	}
	var completedAttachments []completedAttachment
	for completedRows.Next() {
		var item completedAttachment
		if err := completedRows.Scan(&item.id, &item.objectKey, &item.sizeBytes, &item.checksum); err != nil {
			completedRows.Close()
			return ReconcileOutput{}, fmt.Errorf("scan completed attachment for object drift: %w", err)
		}
		completedAttachments = append(completedAttachments, item)
	}
	if err := completedRows.Err(); err != nil {
		completedRows.Close()
		return ReconcileOutput{}, fmt.Errorf("iterate completed attachments for object drift: %w", err)
	}
	if err := completedRows.Close(); err != nil {
		return ReconcileOutput{}, fmt.Errorf("close completed attachment rows: %w", err)
	}

	for _, item := range completedAttachments {
		if s.inspector == nil {
			continue
		}

		probe, err := s.inspector.Probe(ctx, item.objectKey)
		if err != nil {
			attachmentID := item.id
			if err := insertReconcileFinding(ctx, tx, out.RunID, &attachmentID, findingTypeObjectProbeFailed, map[string]any{
				"objectKey": item.objectKey,
				"error":     err.Error(),
			}); err != nil {
				return ReconcileOutput{}, fmt.Errorf("insert object probe failure finding: %w", err)
			}
			out.ObjectProbeErrorCount++
			continue
		}

		if !probe.Exists {
			attachmentID := item.id
			if err := insertReconcileFinding(ctx, tx, out.RunID, &attachmentID, findingTypeCompletedMissingObject, map[string]any{
				"objectKey": item.objectKey,
			}); err != nil {
				return ReconcileOutput{}, fmt.Errorf("insert missing object finding: %w", err)
			}
			out.MissingObjectCount++
			continue
		}

		expectedChecksum := normalizeChecksum(item.checksum)
		observedChecksum := normalizeChecksum(probe.Checksum)
		sizeMismatch := item.sizeBytes > 0 && probe.SizeBytes > 0 && item.sizeBytes != probe.SizeBytes
		checksumMismatch := expectedChecksum != "" && observedChecksum != "" && expectedChecksum != observedChecksum
		if sizeMismatch || checksumMismatch {
			attachmentID := item.id
			if err := insertReconcileFinding(ctx, tx, out.RunID, &attachmentID, findingTypeCompletedIntegrityMismatch, map[string]any{
				"objectKey":         item.objectKey,
				"expectedSizeBytes": item.sizeBytes,
				"observedSizeBytes": probe.SizeBytes,
				"expectedChecksum":  expectedChecksum,
				"observedChecksum":  observedChecksum,
				"sizeMismatch":      sizeMismatch,
				"checksumMismatch":  checksumMismatch,
			}); err != nil {
				return ReconcileOutput{}, fmt.Errorf("insert integrity mismatch finding: %w", err)
			}
			out.IntegrityMismatchCount++
		}
	}

	if s.inspector != nil {
		inventory, err := s.inspector.List(ctx, in.Limit)
		if err != nil {
			if !errors.Is(err, ErrObjectListingUnsupported) {
				if err := insertReconcileFinding(ctx, tx, out.RunID, nil, findingTypeObjectListingFailed, map[string]any{
					"error": err.Error(),
				}); err != nil {
					return ReconcileOutput{}, fmt.Errorf("insert object listing failure finding: %w", err)
				}
				out.ObjectListingErrorCount++
			}
		} else {
			for _, item := range inventory {
				objectKey := strings.TrimSpace(item.ObjectKey)
				if objectKey == "" {
					continue
				}

				var exists bool
				if err := tx.QueryRowContext(ctx, `
					SELECT EXISTS(
						SELECT 1
						FROM attachments
						WHERE object_key = $1
					)
				`, objectKey).Scan(&exists); err != nil {
					return ReconcileOutput{}, fmt.Errorf("check orphan object existence for %s: %w", objectKey, err)
				}
				if exists {
					continue
				}

				if err := insertReconcileFinding(ctx, tx, out.RunID, nil, findingTypeOrphanObject, map[string]any{
					"objectKey": objectKey,
					"sizeBytes": item.SizeBytes,
					"checksum":  normalizeChecksum(item.Checksum),
				}); err != nil {
					return ReconcileOutput{}, fmt.Errorf("insert orphan object finding: %w", err)
				}
				out.OrphanObjectCount++
			}
		}
	}

	out.TotalFindingsCreated =
		out.StaleInitiatedCount +
			out.MissingChecksumCount +
			out.MissingObjectCount +
			out.OrphanObjectCount +
			out.IntegrityMismatchCount +
			out.ObjectProbeErrorCount +
			out.ObjectListingErrorCount
	out.FinishedAt = time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE attachment_reconcile_runs
		SET finished_at = $2,
			stale_initiated_count = $3,
			missing_checksum_count = $4,
			total_findings = $5
		WHERE id = $1::uuid
	`, out.RunID, out.FinishedAt, out.StaleInitiatedCount, out.MissingChecksumCount, out.TotalFindingsCreated); err != nil {
		return ReconcileOutput{}, fmt.Errorf("update reconcile run summary: %w", err)
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, in.ActorUserID, "attachment.reconcile.run", "attachment_reconcile_run", out.RunID, map[string]any{
		"staleInitiatedCount":     out.StaleInitiatedCount,
		"missingChecksumCount":    out.MissingChecksumCount,
		"missingObjectCount":      out.MissingObjectCount,
		"orphanObjectCount":       out.OrphanObjectCount,
		"integrityMismatchCount":  out.IntegrityMismatchCount,
		"objectProbeErrorCount":   out.ObjectProbeErrorCount,
		"objectListingErrorCount": out.ObjectListingErrorCount,
		"scanLimit":               in.Limit,
		"staleAfterSeconds":       int64(in.StaleAfter.Seconds()),
	}); err != nil {
		return ReconcileOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return ReconcileOutput{}, fmt.Errorf("commit reconcile tx: %w", err)
	}

	return out, nil
}

func insertReconcileFinding(ctx context.Context, tx *sql.Tx, runID string, attachmentID *string, findingType string, details any) error {
	if details == nil {
		details = map[string]any{}
	}
	blob, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("marshal reconcile finding details: %w", err)
	}

	var attachmentValue any
	if attachmentID != nil && strings.TrimSpace(*attachmentID) != "" {
		attachmentValue = strings.TrimSpace(*attachmentID)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO attachment_reconcile_findings (
			run_id,
			attachment_id,
			finding_type,
			details
		) VALUES (
			$1::uuid,
			$2::uuid,
			$3,
			$4::jsonb
		)
	`, runID, attachmentValue, findingType, blob); err != nil {
		return err
	}
	return nil
}

func experimentOwner(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, experimentID string) (string, error) {
	var ownerUserID string
	err := q.QueryRowContext(ctx, `
		SELECT owner_user_id::text
		FROM experiments
		WHERE id = $1::uuid
	`, experimentID).Scan(&ownerUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("load experiment owner: %w", err)
	}
	return ownerUserID, nil
}
