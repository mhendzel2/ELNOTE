package search

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid input")
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type ExperimentResult struct {
	ExperimentID string    `json:"experimentId"`
	OwnerUserID  string    `json:"ownerUserId"`
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	Snippet      string    `json:"snippet"`
	Rank         float64   `json:"rank"`
	CreatedAt    time.Time `json:"createdAt"`
}

type ProtocolResult struct {
	ProtocolID  string    `json:"protocolId"`
	OwnerUserID string    `json:"ownerUserId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Rank        float64   `json:"rank"`
	CreatedAt   time.Time `json:"createdAt"`
}

type SearchInput struct {
	Query      string
	UserID     string
	Role       string
	Status     string // optional filter: draft, completed, or empty for all
	DateFrom   *time.Time
	DateTo     *time.Time
	Tags       []string
	Limit      int
	Offset     int
}

type SearchOutput struct {
	Experiments []ExperimentResult `json:"experiments"`
	Protocols   []ProtocolResult   `json:"protocols,omitempty"`
	TotalCount  int                `json:"totalCount"`
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Search(ctx context.Context, in SearchInput) (*SearchOutput, error) {
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return nil, fmt.Errorf("%w: query is required", ErrInvalidInput)
	}
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}
	if in.Offset < 0 {
		in.Offset = 0
	}

	tsQuery := toTSQuery(q)

	out := &SearchOutput{
		Experiments: []ExperimentResult{},
		Protocols:   []ProtocolResult{},
	}

	// --- Search experiments ---
	expQuery, expArgs := s.buildExperimentSearchQuery(tsQuery, in)
	rows, err := s.db.QueryContext(ctx, expQuery, expArgs...)
	if err != nil {
		return nil, fmt.Errorf("search experiments: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r ExperimentResult
		if err := rows.Scan(&r.ExperimentID, &r.OwnerUserID, &r.Title, &r.Status, &r.Snippet, &r.Rank, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan experiment result: %w", err)
		}
		out.Experiments = append(out.Experiments, r)
	}

	// --- Search protocols ---
	protQuery, protArgs := s.buildProtocolSearchQuery(tsQuery, in)
	pRows, err := s.db.QueryContext(ctx, protQuery, protArgs...)
	if err != nil {
		return nil, fmt.Errorf("search protocols: %w", err)
	}
	defer pRows.Close()

	for pRows.Next() {
		var r ProtocolResult
		if err := pRows.Scan(&r.ProtocolID, &r.OwnerUserID, &r.Title, &r.Description, &r.Status, &r.Rank, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan protocol result: %w", err)
		}
		out.Protocols = append(out.Protocols, r)
	}

	out.TotalCount = len(out.Experiments) + len(out.Protocols)
	return out, nil
}

func (s *Service) SearchExperimentContent(ctx context.Context, query, userID, role string, limit int) ([]ExperimentResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("%w: query is required", ErrInvalidInput)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	tsQuery := toTSQuery(q)

	// Search inside experiment entry bodies
	var sqlStr string
	var args []any
	if role == "admin" {
		sqlStr = `
			SELECT DISTINCT e.id, e.owner_user_id, e.title, e.status,
				ts_headline('english', ee.body, to_tsquery('english', $1), 'MaxWords=40,MinWords=20') AS snippet,
				ts_rank(ee.search_vector, to_tsquery('english', $1)) AS rank,
				e.created_at
			FROM experiment_entries ee
			JOIN experiments e ON e.id = ee.experiment_id
			WHERE ee.search_vector @@ to_tsquery('english', $1)
			  AND e.status = 'completed'
			ORDER BY rank DESC
			LIMIT $2`
		args = []any{tsQuery, limit}
	} else {
		sqlStr = `
			SELECT DISTINCT e.id, e.owner_user_id, e.title, e.status,
				ts_headline('english', ee.body, to_tsquery('english', $1), 'MaxWords=40,MinWords=20') AS snippet,
				ts_rank(ee.search_vector, to_tsquery('english', $1)) AS rank,
				e.created_at
			FROM experiment_entries ee
			JOIN experiments e ON e.id = ee.experiment_id
			WHERE ee.search_vector @@ to_tsquery('english', $1)
			  AND e.owner_user_id = $2
			ORDER BY rank DESC
			LIMIT $3`
		args = []any{tsQuery, userID, limit}
	}

	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("search content: %w", err)
	}
	defer rows.Close()

	var results []ExperimentResult
	for rows.Next() {
		var r ExperimentResult
		if err := rows.Scan(&r.ExperimentID, &r.OwnerUserID, &r.Title, &r.Status, &r.Snippet, &r.Rank, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		results = append(results, r)
	}
	if results == nil {
		results = []ExperimentResult{}
	}
	return results, nil
}

func (s *Service) buildExperimentSearchQuery(tsQuery string, in SearchInput) (string, []any) {
	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("e.search_vector @@ to_tsquery('english', $%d)", argIdx))
	args = append(args, tsQuery)
	argIdx++

	// Role-based visibility
	if in.Role == "admin" {
		conditions = append(conditions, "e.status = 'completed'")
	} else {
		conditions = append(conditions, fmt.Sprintf("e.owner_user_id = $%d", argIdx))
		args = append(args, in.UserID)
		argIdx++
	}

	if in.Status != "" {
		conditions = append(conditions, fmt.Sprintf("e.status = $%d", argIdx))
		args = append(args, in.Status)
		argIdx++
	}

	if in.DateFrom != nil {
		conditions = append(conditions, fmt.Sprintf("e.created_at >= $%d", argIdx))
		args = append(args, *in.DateFrom)
		argIdx++
	}
	if in.DateTo != nil {
		conditions = append(conditions, fmt.Sprintf("e.created_at <= $%d", argIdx))
		args = append(args, *in.DateTo)
		argIdx++
	}

	if len(in.Tags) > 0 {
		conditions = append(conditions, fmt.Sprintf(
			`e.id IN (SELECT et.experiment_id FROM experiment_tags et JOIN tags t ON t.id = et.tag_id WHERE t.name = ANY($%d))`,
			argIdx,
		))
		args = append(args, in.Tags)
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT e.id, e.owner_user_id, e.title, e.status,
			ts_headline('english', e.title, to_tsquery('english', $1), 'MaxWords=40,MinWords=10') AS snippet,
			ts_rank(e.search_vector, to_tsquery('english', $1)) AS rank,
			e.created_at
		FROM experiments e
		WHERE %s
		ORDER BY rank DESC
		LIMIT $%d OFFSET $%d`,
		strings.Join(conditions, " AND "),
		argIdx, argIdx+1,
	)

	args = append(args, in.Limit, in.Offset)

	return query, args
}

func (s *Service) buildProtocolSearchQuery(tsQuery string, in SearchInput) (string, []any) {
	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("p.search_vector @@ to_tsquery('english', $%d)", argIdx))
	args = append(args, tsQuery)
	argIdx++

	if in.Role == "admin" {
		conditions = append(conditions, "p.status IN ('published','archived')")
	} else {
		conditions = append(conditions, fmt.Sprintf("p.owner_user_id = $%d", argIdx))
		args = append(args, in.UserID)
		argIdx++
	}

	query := fmt.Sprintf(`
		SELECT p.id, p.owner_user_id, p.title, p.description, p.status,
			ts_rank(p.search_vector, to_tsquery('english', $1)) AS rank,
			p.created_at
		FROM protocols p
		WHERE %s
		ORDER BY rank DESC
		LIMIT $%d OFFSET $%d`,
		strings.Join(conditions, " AND "),
		argIdx, argIdx+1,
	)

	args = append(args, in.Limit, in.Offset)

	return query, args
}

// toTSQuery converts user input to a safe tsquery string using & (AND) between words.
func toTSQuery(input string) string {
	words := strings.Fields(input)
	for i, w := range words {
		// Strip non-alphanumeric, add :* for prefix matching
		clean := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, w)
		if clean != "" {
			words[i] = clean + ":*"
		}
	}
	return strings.Join(words, " & ")
}
