package admin

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

type Service struct {
	db   *sql.DB
	sync *syncer.Service
}

func NewService(db *sql.DB, syncService *syncer.Service) *Service {
	return &Service{db: db, sync: syncService}
}

type CreateCommentInput struct {
	ExperimentID string
	AdminUserID  string
	DeviceID     string
	Body         string
}

type CreateCommentOutput struct {
	CommentID    string    `json:"commentId"`
	ExperimentID string    `json:"experimentId"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Comment struct {
	CommentID    string    `json:"commentId"`
	ExperimentID string    `json:"experimentId"`
	AuthorUserID string    `json:"authorUserId"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"createdAt"`
}

type CreateProposalInput struct {
	SourceExperimentID string
	AdminUserID        string
	DeviceID           string
	Title              string
	Body               string
}

type CreateProposalOutput struct {
	ProposalID          string    `json:"proposalId"`
	SourceExperimentID  string    `json:"sourceExperimentId"`
	CreatedAt           time.Time `json:"createdAt"`
}

type Proposal struct {
	ProposalID         string    `json:"proposalId"`
	SourceExperimentID string    `json:"sourceExperimentId"`
	ProposerUserID     string    `json:"proposerUserId"`
	Title              string    `json:"title"`
	Body               string    `json:"body"`
	CreatedAt          time.Time `json:"createdAt"`
}

func (s *Service) CreateComment(ctx context.Context, in CreateCommentInput) (CreateCommentOutput, error) {
	if strings.TrimSpace(in.ExperimentID) == "" || strings.TrimSpace(in.AdminUserID) == "" || strings.TrimSpace(in.Body) == "" {
		return CreateCommentOutput{}, ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CreateCommentOutput{}, fmt.Errorf("begin create comment tx: %w", err)
	}
	defer tx.Rollback()

	ownerUserID, err := getCompletedExperimentOwner(ctx, tx, in.ExperimentID)
	if err != nil {
		return CreateCommentOutput{}, err
	}

	out := CreateCommentOutput{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO record_comments (
			experiment_id,
			author_user_id,
			body
		) VALUES (
			$1::uuid,
			$2::uuid,
			$3
		)
		RETURNING id::text, experiment_id::text, created_at
	`, in.ExperimentID, in.AdminUserID, in.Body).Scan(&out.CommentID, &out.ExperimentID, &out.CreatedAt)
	if err != nil {
		return CreateCommentOutput{}, fmt.Errorf("insert comment: %w", err)
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, in.AdminUserID, "comment.create", "record_comment", out.CommentID, map[string]any{
		"experimentId": in.ExperimentID,
	}); err != nil {
		return CreateCommentOutput{}, err
	}

	if _, err := s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   ownerUserID,
		ActorUserID:   in.AdminUserID,
		DeviceID:      in.DeviceID,
		EventType:     "comment.created",
		AggregateType: "record_comment",
		AggregateID:   out.CommentID,
		Payload: map[string]any{
			"experimentId": in.ExperimentID,
		},
	}); err != nil {
		return CreateCommentOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return CreateCommentOutput{}, fmt.Errorf("commit create comment tx: %w", err)
	}

	return out, nil
}

func (s *Service) ListComments(ctx context.Context, experimentID, viewerUserID, viewerRole string) ([]Comment, error) {
	if strings.TrimSpace(experimentID) == "" || strings.TrimSpace(viewerUserID) == "" {
		return nil, ErrInvalidInput
	}

	if err := authorizeExperimentRead(ctx, s.db, experimentID, viewerUserID, viewerRole); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, experiment_id::text, author_user_id::text, body, created_at
		FROM record_comments
		WHERE experiment_id = $1::uuid
		ORDER BY created_at ASC
	`, experimentID)
	if err != nil {
		return nil, fmt.Errorf("query comments: %w", err)
	}
	defer rows.Close()

	comments := make([]Comment, 0)
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.CommentID, &c.ExperimentID, &c.AuthorUserID, &c.Body, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate comments: %w", err)
	}

	return comments, nil
}

func (s *Service) CreateProposal(ctx context.Context, in CreateProposalInput) (CreateProposalOutput, error) {
	if strings.TrimSpace(in.SourceExperimentID) == "" || strings.TrimSpace(in.AdminUserID) == "" || strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.Body) == "" {
		return CreateProposalOutput{}, ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CreateProposalOutput{}, fmt.Errorf("begin create proposal tx: %w", err)
	}
	defer tx.Rollback()

	ownerUserID, err := getCompletedExperimentOwner(ctx, tx, in.SourceExperimentID)
	if err != nil {
		return CreateProposalOutput{}, err
	}

	out := CreateProposalOutput{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO experiment_proposals (
			source_experiment_id,
			proposer_user_id,
			title,
			body
		) VALUES (
			$1::uuid,
			$2::uuid,
			$3,
			$4
		)
		RETURNING id::text, source_experiment_id::text, created_at
	`, in.SourceExperimentID, in.AdminUserID, strings.TrimSpace(in.Title), in.Body).Scan(&out.ProposalID, &out.SourceExperimentID, &out.CreatedAt)
	if err != nil {
		return CreateProposalOutput{}, fmt.Errorf("insert proposal: %w", err)
	}

	if err := internaldb.AppendAuditEvent(ctx, tx, in.AdminUserID, "proposal.create", "experiment_proposal", out.ProposalID, map[string]any{
		"sourceExperimentId": in.SourceExperimentID,
		"title":              in.Title,
	}); err != nil {
		return CreateProposalOutput{}, err
	}

	if _, err := s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   ownerUserID,
		ActorUserID:   in.AdminUserID,
		DeviceID:      in.DeviceID,
		EventType:     "proposal.created",
		AggregateType: "experiment_proposal",
		AggregateID:   out.ProposalID,
		Payload: map[string]any{
			"sourceExperimentId": in.SourceExperimentID,
			"title":              in.Title,
		},
	}); err != nil {
		return CreateProposalOutput{}, err
	}

	if err := tx.Commit(); err != nil {
		return CreateProposalOutput{}, fmt.Errorf("commit create proposal tx: %w", err)
	}

	return out, nil
}

func (s *Service) ListProposals(ctx context.Context, sourceExperimentID, viewerUserID, viewerRole string) ([]Proposal, error) {
	if strings.TrimSpace(sourceExperimentID) == "" || strings.TrimSpace(viewerUserID) == "" {
		return nil, ErrInvalidInput
	}

	if err := authorizeExperimentRead(ctx, s.db, sourceExperimentID, viewerUserID, viewerRole); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, source_experiment_id::text, proposer_user_id::text, title, body, created_at
		FROM experiment_proposals
		WHERE source_experiment_id = $1::uuid
		ORDER BY created_at ASC
	`, sourceExperimentID)
	if err != nil {
		return nil, fmt.Errorf("query proposals: %w", err)
	}
	defer rows.Close()

	proposals := make([]Proposal, 0)
	for rows.Next() {
		var p Proposal
		if err := rows.Scan(&p.ProposalID, &p.SourceExperimentID, &p.ProposerUserID, &p.Title, &p.Body, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan proposal: %w", err)
		}
		proposals = append(proposals, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate proposals: %w", err)
	}

	return proposals, nil
}

func authorizeExperimentRead(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, experimentID, viewerUserID, viewerRole string) error {
	var ownerID, status string
	err := q.QueryRowContext(ctx, `
		SELECT owner_user_id::text, status
		FROM experiments
		WHERE id = $1::uuid
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

func getCompletedExperimentOwner(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, experimentID string) (string, error) {
	var ownerUserID, status string
	err := q.QueryRowContext(ctx, `
		SELECT owner_user_id::text, status
		FROM experiments
		WHERE id = $1::uuid
	`, experimentID).Scan(&ownerUserID, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("lookup experiment for admin action: %w", err)
	}

	if status != "completed" {
		return "", ErrForbidden
	}

	return ownerUserID, nil
}
