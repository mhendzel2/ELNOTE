package signatures

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mjhen/elnote/server/internal/auth"
	internaldb "github.com/mjhen/elnote/server/internal/db"
	"github.com/mjhen/elnote/server/internal/syncer"
)

var (
	ErrForbidden       = errors.New("forbidden")
	ErrNotFound        = errors.New("not found")
	ErrInvalidInput    = errors.New("invalid input")
	ErrInvalidPassword = errors.New("invalid password")
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type Signature struct {
	ID            string    `json:"signatureId"`
	ExperimentID  string    `json:"experimentId"`
	SignerUserID  string    `json:"signerUserId"`
	SignerEmail   string    `json:"signerEmail"`
	SignatureType string    `json:"signatureType"`
	ContentHash   string    `json:"contentHash"`
	SignedAt      time.Time `json:"signedAt"`
}

type SignInput struct {
	ExperimentID  string
	SignerUserID  string
	SignatureType string // "author" or "witness"
	Password      string // re-enter password to sign
	DeviceID      string
}

type SignOutput struct {
	SignatureID string    `json:"signatureId"`
	ContentHash string    `json:"contentHash"`
	SignedAt    time.Time `json:"signedAt"`
}

type VerifyOutput struct {
	ExperimentID   string      `json:"experimentId"`
	Signatures     []Signature `json:"signatures"`
	ContentHash    string      `json:"currentContentHash"`
	IntegrityValid bool        `json:"integrityValid"`
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

func (s *Service) Sign(ctx context.Context, in SignInput) (*SignOutput, error) {
	if in.SignatureType != "author" && in.SignatureType != "witness" {
		return nil, fmt.Errorf("%w: signatureType must be author or witness", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Verify password for signing
	var passwordHash string
	err = tx.QueryRowContext(ctx,
		`SELECT password_hash FROM users WHERE id = $1`, in.SignerUserID,
	).Scan(&passwordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query user: %w", err)
	}
	if ok, verr := auth.VerifyPassword(passwordHash, in.Password); verr != nil || !ok {
		return nil, fmt.Errorf("%w: invalid signing password", ErrForbidden)
	}

	// Get experiment + effective body
	var expOwner, expStatus, effectiveBody string
	err = tx.QueryRowContext(ctx,
		`SELECT e.owner_user_id, e.status,
			(SELECT ee.body FROM experiment_entries ee
			 WHERE ee.experiment_id = e.id
			 ORDER BY ee.created_at DESC LIMIT 1)
		 FROM experiments e WHERE e.id = $1`,
		in.ExperimentID,
	).Scan(&expOwner, &expStatus, &effectiveBody)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query experiment: %w", err)
	}

	if expStatus != "completed" {
		return nil, fmt.Errorf("%w: experiment must be completed before signing", ErrInvalidInput)
	}

	// Author signs own; witness signs others'
	if in.SignatureType == "author" && expOwner != in.SignerUserID {
		return nil, fmt.Errorf("%w: only the owner can author-sign", ErrForbidden)
	}
	if in.SignatureType == "witness" && expOwner == in.SignerUserID {
		return nil, fmt.Errorf("%w: owner cannot witness their own experiment", ErrForbidden)
	}

	// Compute content hash
	hash := sha256.Sum256([]byte(effectiveBody))
	contentHash := hex.EncodeToString(hash[:])

	var sigID string
	var signedAt time.Time
	err = tx.QueryRowContext(ctx,
		`INSERT INTO experiment_signatures (experiment_id, signer_user_id, signature_type, content_hash)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, signed_at`,
		in.ExperimentID, in.SignerUserID, in.SignatureType, contentHash,
	).Scan(&sigID, &signedAt)
	if err != nil {
		return nil, fmt.Errorf("insert signature: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"signatureId":   sigID,
		"signatureType": in.SignatureType,
		"contentHash":   contentHash,
		"experimentId":  in.ExperimentID,
	})
	internaldb.AppendAuditEvent(ctx, tx, in.SignerUserID, "experiment.signed", "experiment", in.ExperimentID, payload)

	s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   expOwner,
		ActorUserID:   in.SignerUserID,
		DeviceID:      in.DeviceID,
		EventType:     "experiment.signed",
		AggregateType: "experiment",
		AggregateID:   in.ExperimentID,
		Payload:       payload,
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &SignOutput{
		SignatureID: sigID,
		ContentHash: contentHash,
		SignedAt:    signedAt,
	}, nil
}

func (s *Service) ListSignatures(ctx context.Context, experimentID, userID, role string) ([]Signature, error) {
	// Access check
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
		`SELECT s.id, s.experiment_id, s.signer_user_id, u.email, s.signature_type, s.content_hash, s.signed_at
		 FROM experiment_signatures s
		 JOIN users u ON u.id = s.signer_user_id
		 WHERE s.experiment_id = $1
		 ORDER BY s.signed_at`,
		experimentID,
	)
	if err != nil {
		return nil, fmt.Errorf("query signatures: %w", err)
	}
	defer rows.Close()

	var sigs []Signature
	for rows.Next() {
		var sig Signature
		if err := rows.Scan(&sig.ID, &sig.ExperimentID, &sig.SignerUserID, &sig.SignerEmail, &sig.SignatureType, &sig.ContentHash, &sig.SignedAt); err != nil {
			return nil, fmt.Errorf("scan signature: %w", err)
		}
		sigs = append(sigs, sig)
	}
	if sigs == nil {
		sigs = []Signature{}
	}
	return sigs, nil
}

func (s *Service) VerifySignatures(ctx context.Context, experimentID, userID, role string) (*VerifyOutput, error) {
	sigs, err := s.ListSignatures(ctx, experimentID, userID, role)
	if err != nil {
		return nil, err
	}

	// Get current effective body hash
	var effectiveBody string
	err = s.db.QueryRowContext(ctx,
		`SELECT ee.body FROM experiment_entries ee
		 WHERE ee.experiment_id = $1
		 ORDER BY ee.created_at DESC LIMIT 1`,
		experimentID,
	).Scan(&effectiveBody)
	if err != nil {
		return nil, fmt.Errorf("query effective body: %w", err)
	}

	hash := sha256.Sum256([]byte(effectiveBody))
	currentHash := hex.EncodeToString(hash[:])

	// Check that all signature content hashes match
	valid := true
	for _, sig := range sigs {
		if sig.ContentHash != currentHash {
			valid = false
			break
		}
	}

	return &VerifyOutput{
		ExperimentID:   experimentID,
		Signatures:     sigs,
		ContentHash:    currentHash,
		IntegrityValid: valid,
	}, nil
}
