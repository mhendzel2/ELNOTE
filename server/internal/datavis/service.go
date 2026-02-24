package datavis

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type DataExtract struct {
	ID            string     `json:"dataExtractId"`
	AttachmentID  string     `json:"attachmentId"`
	ExperimentID  string     `json:"experimentId"`
	ColumnHeaders []string   `json:"columnHeaders"`
	RowCount      int        `json:"rowCount"`
	SampleRows    [][]string `json:"sampleRows"`
	ParsedAt      time.Time  `json:"parsedAt"`
}

type ChartConfig struct {
	ID            string         `json:"chartConfigId"`
	ExperimentID  string         `json:"experimentId"`
	DataExtractID string         `json:"dataExtractId"`
	CreatorUserID string         `json:"creatorUserId"`
	ChartType     string         `json:"chartType"`
	Title         string         `json:"title"`
	XColumn       string         `json:"xColumn"`
	YColumns      []string       `json:"yColumns"`
	Options       map[string]any `json:"options"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type ParseCSVInput struct {
	AttachmentID string
	ExperimentID string
	CSVData      []byte
	ActorUserID  string
}

type CreateChartInput struct {
	ExperimentID  string
	DataExtractID string
	CreatorUserID string
	DeviceID      string
	ChartType     string
	Title         string
	XColumn       string
	YColumns      []string
	Options       map[string]any
}

type CreateChartOutput struct {
	ChartConfigID string    `json:"chartConfigId"`
	CreatedAt     time.Time `json:"createdAt"`
}

type DataPreviewInput struct {
	AttachmentID string
	UserID       string
	Role         string
	MaxRows      int
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

// ParseCSV parses CSV data from an attachment and stores the extract.
func (s *Service) ParseCSV(ctx context.Context, in ParseCSVInput) (*DataExtract, error) {
	reader := csv.NewReader(bytes.NewReader(in.CSVData))
	reader.TrimLeadingSpace = true

	// Read headers
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read CSV headers: %v", ErrInvalidInput, err)
	}

	// Read all rows
	var allRows [][]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: CSV parse error: %v", ErrInvalidInput, err)
		}
		allRows = append(allRows, record)
	}

	// Keep sample of first 100 rows
	sampleRows := allRows
	if len(sampleRows) > 100 {
		sampleRows = sampleRows[:100]
	}

	headersJSON, _ := json.Marshal(headers)
	sampleJSON, _ := json.Marshal(sampleRows)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// attachment_id is nullable — pass nil when no attachment is associated
	var attachParam any
	if in.AttachmentID != "" {
		attachParam = in.AttachmentID
	}

	var extract DataExtract
	var nullableAttachID sql.NullString
	err = tx.QueryRowContext(ctx,
		`INSERT INTO data_extracts (attachment_id, experiment_id, column_headers, row_count, sample_rows)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, COALESCE(attachment_id::text,''), experiment_id, row_count, parsed_at`,
		attachParam, in.ExperimentID, headersJSON, len(allRows), sampleJSON,
	).Scan(&extract.ID, &nullableAttachID, &extract.ExperimentID, &extract.RowCount, &extract.ParsedAt)
	if err != nil {
		return nil, fmt.Errorf("insert data extract: %w", err)
	}
	extract.AttachmentID = nullableAttachID.String
	extract.ColumnHeaders = headers
	extract.SampleRows = sampleRows

	if err := internaldb.AppendAuditEvent(ctx, tx, in.ActorUserID, "data.extract_created", "attachment", in.AttachmentID, map[string]any{
		"dataExtractId": extract.ID,
		"attachmentId":  in.AttachmentID,
	}); err != nil {
		return nil, fmt.Errorf("append data.extract_created audit event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &extract, nil
}

func (s *Service) GetDataExtract(ctx context.Context, extractID, userID, role string) (*DataExtract, error) {
	var extract DataExtract
	var headersJSON, sampleJSON []byte
	var nullAttach sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT de.id, COALESCE(de.attachment_id::text,''), de.experiment_id, de.column_headers, de.row_count, de.sample_rows, de.parsed_at
		 FROM data_extracts de
		 JOIN experiments e ON e.id = de.experiment_id
		 WHERE de.id = $1 AND (e.owner_user_id = $2 OR ($3 = 'admin' AND e.status = 'completed'))`,
		extractID, userID, role,
	).Scan(&extract.ID, &nullAttach, &extract.ExperimentID, &headersJSON, &extract.RowCount, &sampleJSON, &extract.ParsedAt)
	extract.AttachmentID = nullAttach.String
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query data extract: %w", err)
	}

	json.Unmarshal(headersJSON, &extract.ColumnHeaders)
	json.Unmarshal(sampleJSON, &extract.SampleRows)

	return &extract, nil
}

func (s *Service) ListDataExtracts(ctx context.Context, experimentID, userID, role string) ([]DataExtract, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT de.id, COALESCE(de.attachment_id::text,''), de.experiment_id, de.column_headers, de.row_count, de.parsed_at
		 FROM data_extracts de
		 JOIN experiments e ON e.id = de.experiment_id
		 WHERE de.experiment_id = $1 AND (e.owner_user_id = $2 OR ($3 = 'admin' AND e.status = 'completed'))
		 ORDER BY de.parsed_at DESC`,
		experimentID, userID, role,
	)
	if err != nil {
		return nil, fmt.Errorf("query extracts: %w", err)
	}
	defer rows.Close()

	var extracts []DataExtract
	for rows.Next() {
		var de DataExtract
		var headersJSON []byte
		var nullAttach sql.NullString
		if err := rows.Scan(&de.ID, &nullAttach, &de.ExperimentID, &headersJSON, &de.RowCount, &de.ParsedAt); err != nil {
			return nil, fmt.Errorf("scan extract: %w", err)
		}
		de.AttachmentID = nullAttach.String
		json.Unmarshal(headersJSON, &de.ColumnHeaders)
		extracts = append(extracts, de)
	}
	if extracts == nil {
		extracts = []DataExtract{}
	}
	return extracts, nil
}

func (s *Service) CreateChartConfig(ctx context.Context, in CreateChartInput) (*CreateChartOutput, error) {
	validTypes := map[string]bool{"line": true, "scatter": true, "bar": true, "histogram": true, "area": true}
	if !validTypes[in.ChartType] {
		return nil, fmt.Errorf("%w: chartType must be line, scatter, bar, histogram, or area", ErrInvalidInput)
	}
	if strings.TrimSpace(in.XColumn) == "" {
		return nil, fmt.Errorf("%w: xColumn is required", ErrInvalidInput)
	}
	if len(in.YColumns) == 0 {
		return nil, fmt.Errorf("%w: at least one yColumn is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Verify experiment ownership
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
	if expOwner != in.CreatorUserID {
		return nil, ErrForbidden
	}

	yColumnsJSON, _ := json.Marshal(in.YColumns)
	optionsJSON, _ := json.Marshal(in.Options)
	if in.Options == nil {
		optionsJSON = []byte("{}")
	}

	var chartID string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx,
		`INSERT INTO chart_configs (experiment_id, data_extract_id, creator_user_id, chart_type, title, x_column, y_columns, options)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at`,
		in.ExperimentID, in.DataExtractID, in.CreatorUserID, in.ChartType, in.Title, in.XColumn, yColumnsJSON, optionsJSON,
	).Scan(&chartID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert chart config: %w", err)
	}

	payload := map[string]any{
		"chartConfigId": chartID,
		"chartType":     in.ChartType,
		"experimentId":  in.ExperimentID,
	}
	if err := internaldb.AppendAuditEvent(ctx, tx, in.CreatorUserID, "chart.created", "experiment", in.ExperimentID, payload); err != nil {
		return nil, fmt.Errorf("append chart.created audit event: %w", err)
	}

	if _, err := s.sync.AppendEvent(ctx, tx, syncer.AppendEventInput{
		OwnerUserID:   expOwner,
		ActorUserID:   in.CreatorUserID,
		DeviceID:      in.DeviceID,
		EventType:     "chart.created",
		AggregateType: "experiment",
		AggregateID:   in.ExperimentID,
		Payload:       payload,
	}); err != nil {
		return nil, fmt.Errorf("append chart.created sync event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &CreateChartOutput{
		ChartConfigID: chartID,
		CreatedAt:     createdAt,
	}, nil
}

func (s *Service) ListChartConfigs(ctx context.Context, experimentID, userID, role string) ([]ChartConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cc.id, cc.experiment_id, cc.data_extract_id, cc.creator_user_id,
			cc.chart_type, cc.title, cc.x_column, cc.y_columns, cc.options, cc.created_at
		 FROM chart_configs cc
		 JOIN experiments e ON e.id = cc.experiment_id
		 WHERE cc.experiment_id = $1 AND (e.owner_user_id = $2 OR ($3 = 'admin' AND e.status = 'completed'))
		 ORDER BY cc.created_at`,
		experimentID, userID, role,
	)
	if err != nil {
		return nil, fmt.Errorf("query chart configs: %w", err)
	}
	defer rows.Close()

	var configs []ChartConfig
	for rows.Next() {
		var cc ChartConfig
		var yColumnsJSON, optionsJSON []byte
		if err := rows.Scan(&cc.ID, &cc.ExperimentID, &cc.DataExtractID, &cc.CreatorUserID,
			&cc.ChartType, &cc.Title, &cc.XColumn, &yColumnsJSON, &optionsJSON, &cc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chart config: %w", err)
		}
		json.Unmarshal(yColumnsJSON, &cc.YColumns)
		json.Unmarshal(optionsJSON, &cc.Options)
		configs = append(configs, cc)
	}
	if configs == nil {
		configs = []ChartConfig{}
	}
	return configs, nil
}

func (s *Service) GetChartConfig(ctx context.Context, chartID, userID, role string) (*ChartConfig, error) {
	var cc ChartConfig
	var yColumnsJSON, optionsJSON []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT cc.id, cc.experiment_id, cc.data_extract_id, cc.creator_user_id,
			cc.chart_type, cc.title, cc.x_column, cc.y_columns, cc.options, cc.created_at
		 FROM chart_configs cc
		 JOIN experiments e ON e.id = cc.experiment_id
		 WHERE cc.id = $1 AND (e.owner_user_id = $2 OR ($3 = 'admin' AND e.status = 'completed'))`,
		chartID, userID, role,
	).Scan(&cc.ID, &cc.ExperimentID, &cc.DataExtractID, &cc.CreatorUserID,
		&cc.ChartType, &cc.Title, &cc.XColumn, &yColumnsJSON, &optionsJSON, &cc.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query chart: %w", err)
	}
	json.Unmarshal(yColumnsJSON, &cc.YColumns)
	json.Unmarshal(optionsJSON, &cc.Options)
	return &cc, nil
}
