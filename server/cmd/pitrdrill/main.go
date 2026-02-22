package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	internaldb "github.com/mjhen/elnote/server/internal/db"
	"github.com/mjhen/elnote/server/internal/ops"
)

type drillArtifact struct {
	RunID                 string      `json:"runId"`
	StartedAtUTC          time.Time   `json:"startedAtUtc"`
	FinishedAtUTC         time.Time   `json:"finishedAtUtc"`
	SourceDatabase        string      `json:"sourceDatabase"`
	RestoreDatabase       string      `json:"restoreDatabase"`
	TargetRecoveryTimeUTC time.Time   `json:"targetRecoveryTimeUtc"`
	BackupFile            string      `json:"backupFile"`
	Checks                drillChecks `json:"checks"`
	Steps                 []drillStep `json:"steps"`
	Success               bool        `json:"success"`
	Error                 string      `json:"error,omitempty"`
	GapsAndActions        []string    `json:"gapsAndActions,omitempty"`
}

type drillChecks struct {
	CoreTables  map[string]bool              `json:"coreTables"`
	TableCounts map[string]int64             `json:"tableCounts"`
	AuditVerify *ops.AuditVerificationResult `json:"auditVerify,omitempty"`
}

type drillStep struct {
	Name       string    `json:"name"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Success    bool      `json:"success"`
	Output     string    `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	sourceDSN := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	artifactDir := filepath.Clean(filepath.Join("..", "docs", "drills", "pitr"))
	restoreDBPrefix := "elnote_restore_drill"
	targetOffset := 2 * time.Minute
	keepRestoreDB := false

	flag.StringVar(&sourceDSN, "source-dsn", sourceDSN, "source Postgres DSN (defaults to DATABASE_URL)")
	flag.StringVar(&artifactDir, "artifact-dir", artifactDir, "directory for drill evidence artifacts")
	flag.StringVar(&restoreDBPrefix, "restore-db-prefix", restoreDBPrefix, "prefix for temporary restore database")
	flag.DurationVar(&targetOffset, "target-offset", targetOffset, "how far back from now to set the target recovery timestamp")
	flag.BoolVar(&keepRestoreDB, "keep-restore-db", keepRestoreDB, "keep restored drill database instead of dropping it")
	flag.Parse()

	if sourceDSN == "" {
		fmt.Fprintln(os.Stderr, "source DSN is required (set --source-dsn or DATABASE_URL)")
		os.Exit(2)
	}
	if targetOffset < 0 {
		targetOffset = 0
	}

	runID := time.Now().UTC().Format("20060102T150405Z")
	artifact := drillArtifact{
		RunID:                 runID,
		StartedAtUTC:          time.Now().UTC(),
		TargetRecoveryTimeUTC: time.Now().UTC().Add(-targetOffset),
		Checks: drillChecks{
			CoreTables:  map[string]bool{},
			TableCounts: map[string]int64{},
		},
		GapsAndActions: []string{
			"This workflow validates logical backup restore using pg_dump/pg_restore. Configure and drill WAL replay separately for full PITR coverage.",
		},
	}

	tmpDir, err := os.MkdirTemp("", "elnote-pitr-drill-*")
	if err != nil {
		finishWithError(ctx, artifactDir, &artifact, fmt.Errorf("create temp dir: %w", err))
	}
	defer os.RemoveAll(tmpDir)

	dumpPath := filepath.Join(tmpDir, "backup.dump")
	artifact.BackupFile = dumpPath

	sourceDBName, err := databaseNameFromDSN(sourceDSN)
	if err != nil {
		finishWithError(ctx, artifactDir, &artifact, fmt.Errorf("parse source dsn: %w", err))
	}
	artifact.SourceDatabase = sourceDBName

	restoreDBName := fmt.Sprintf("%s_%s", sanitizeDBIdentifier(restoreDBPrefix), strings.ToLower(runID))
	artifact.RestoreDatabase = restoreDBName

	adminDSN, err := withDatabaseName(sourceDSN, "postgres")
	if err != nil {
		finishWithError(ctx, artifactDir, &artifact, fmt.Errorf("build admin dsn: %w", err))
	}
	restoreDSN, err := withDatabaseName(sourceDSN, restoreDBName)
	if err != nil {
		finishWithError(ctx, artifactDir, &artifact, fmt.Errorf("build restore dsn: %w", err))
	}

	adminDB, err := internaldb.Open(ctx, adminDSN)
	if err != nil {
		finishWithError(ctx, artifactDir, &artifact, fmt.Errorf("connect admin db: %w", err))
	}
	defer adminDB.Close()

	defer func() {
		if keepRestoreDB {
			return
		}
		_ = dropDatabase(context.Background(), adminDB, restoreDBName)
	}()

	artifact.Steps = append(artifact.Steps, runStep(ctx, "pg_dump", func() (string, error) {
		return runCommand(ctx, "pg_dump",
			"--format=custom",
			"--no-owner",
			"--no-privileges",
			"--dbname="+sourceDSN,
			"--file="+dumpPath,
		)
	}))
	if !artifact.Steps[len(artifact.Steps)-1].Success {
		finishWithError(ctx, artifactDir, &artifact, errors.New(artifact.Steps[len(artifact.Steps)-1].Error))
	}

	artifact.Steps = append(artifact.Steps, runStep(ctx, "prepare_restore_database", func() (string, error) {
		if err := dropDatabase(ctx, adminDB, restoreDBName); err != nil {
			return "", err
		}
		if _, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %s`, quoteIdent(restoreDBName))); err != nil {
			return "", fmt.Errorf("create restore database: %w", err)
		}
		return fmt.Sprintf("created database %s", restoreDBName), nil
	}))
	if !artifact.Steps[len(artifact.Steps)-1].Success {
		finishWithError(ctx, artifactDir, &artifact, errors.New(artifact.Steps[len(artifact.Steps)-1].Error))
	}

	artifact.Steps = append(artifact.Steps, runStep(ctx, "pg_restore", func() (string, error) {
		return runCommand(ctx, "pg_restore",
			"--clean",
			"--if-exists",
			"--no-owner",
			"--no-privileges",
			"--dbname="+restoreDSN,
			dumpPath,
		)
	}))
	if !artifact.Steps[len(artifact.Steps)-1].Success {
		finishWithError(ctx, artifactDir, &artifact, errors.New(artifact.Steps[len(artifact.Steps)-1].Error))
	}

	restoreDB, err := internaldb.Open(ctx, restoreDSN)
	if err != nil {
		finishWithError(ctx, artifactDir, &artifact, fmt.Errorf("connect restore db: %w", err))
	}
	defer restoreDB.Close()

	artifact.Steps = append(artifact.Steps, runStep(ctx, "verify_core_tables", func() (string, error) {
		checks, counts, err := verifyCoreTables(ctx, restoreDB)
		if err != nil {
			return "", err
		}
		artifact.Checks.CoreTables = checks
		artifact.Checks.TableCounts = counts
		for table, ok := range checks {
			if !ok {
				return "", fmt.Errorf("required table missing: %s", table)
			}
		}
		return "core tables present", nil
	}))
	if !artifact.Steps[len(artifact.Steps)-1].Success {
		finishWithError(ctx, artifactDir, &artifact, errors.New(artifact.Steps[len(artifact.Steps)-1].Error))
	}

	artifact.Steps = append(artifact.Steps, runStep(ctx, "verify_audit_chain", func() (string, error) {
		verify, err := ops.NewService(restoreDB).VerifyAuditHashChain(ctx)
		if err != nil {
			return "", err
		}
		artifact.Checks.AuditVerify = &verify
		if !verify.Valid {
			return "", fmt.Errorf("audit chain invalid: %s (brokenAt=%d)", verify.Message, verify.BrokenAtEventID)
		}
		return fmt.Sprintf("audit chain valid (%d events)", verify.CheckedEvents), nil
	}))
	if !artifact.Steps[len(artifact.Steps)-1].Success {
		finishWithError(ctx, artifactDir, &artifact, errors.New(artifact.Steps[len(artifact.Steps)-1].Error))
	}

	artifact.Success = true
	artifact.FinishedAtUTC = time.Now().UTC()
	path, writeErr := writeArtifact(artifactDir, &artifact)
	if writeErr != nil {
		fmt.Fprintf(os.Stderr, "write artifact: %v\n", writeErr)
		os.Exit(1)
	}

	fmt.Printf("PITR drill completed successfully. Artifact: %s\n", path)
}

func finishWithError(ctx context.Context, artifactDir string, artifact *drillArtifact, err error) {
	artifact.Success = false
	artifact.Error = err.Error()
	artifact.FinishedAtUTC = time.Now().UTC()
	path, writeErr := writeArtifact(artifactDir, artifact)
	if writeErr != nil {
		fmt.Fprintf(os.Stderr, "drill failed: %v\nwrite artifact failed: %v\n", err, writeErr)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "drill failed: %v\nartifact: %s\n", err, path)
	_ = ctx
	os.Exit(1)
}

func runStep(ctx context.Context, name string, fn func() (string, error)) drillStep {
	step := drillStep{Name: name, StartedAt: time.Now().UTC()}
	out, err := fn()
	step.FinishedAt = time.Now().UTC()
	if out != "" {
		step.Output = out
	}
	if err != nil {
		step.Success = false
		step.Error = err.Error()
		return step
	}
	step.Success = true
	_ = ctx
	return step
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s failed: %w (output: %s)", name, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func verifyCoreTables(ctx context.Context, db *sql.DB) (map[string]bool, map[string]int64, error) {
	tables := []string{
		"users",
		"experiments",
		"experiment_entries",
		"attachments",
		"audit_log",
		"sync_events",
	}
	checks := make(map[string]bool, len(tables))
	counts := make(map[string]int64, len(tables))

	for _, table := range tables {
		var reg sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1)`, "public."+table).Scan(&reg); err != nil {
			return nil, nil, fmt.Errorf("check table %s: %w", table, err)
		}
		checks[table] = reg.Valid && reg.String != ""
		if !checks[table] {
			continue
		}

		var count int64
		query := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, quoteIdent(table))
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return nil, nil, fmt.Errorf("count table %s: %w", table, err)
		}
		counts[table] = count
	}

	return checks, counts, nil
}

func writeArtifact(artifactDir string, artifact *drillArtifact) (string, error) {
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return "", fmt.Errorf("create artifact dir: %w", err)
	}

	filePath := filepath.Join(artifactDir, fmt.Sprintf("pitr-drill-%s.json", artifact.RunID))
	blob, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal artifact: %w", err)
	}
	if err := os.WriteFile(filePath, blob, 0o644); err != nil {
		return "", fmt.Errorf("write artifact file: %w", err)
	}
	return filePath, nil
}

func dropDatabase(ctx context.Context, adminDB *sql.DB, dbName string) error {
	if _, err := adminDB.ExecContext(ctx,
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`,
		dbName,
	); err != nil {
		return fmt.Errorf("terminate existing db sessions: %w", err)
	}

	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteIdent(dbName))); err != nil {
		return fmt.Errorf("drop database %s: %w", dbName, err)
	}
	return nil
}

func quoteIdent(v string) string {
	return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
}

func sanitizeDBIdentifier(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "elnote_restore_drill"
	}
	var b strings.Builder
	for _, ch := range v {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "elnote_restore_drill"
	}
	return out
}

func databaseNameFromDSN(dsn string) (string, error) {
	u, err := parseDSN(dsn)
	if err != nil {
		return "", err
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", fmt.Errorf("missing database name in dsn")
	}
	return name, nil
}

func withDatabaseName(dsn, dbName string) (string, error) {
	u, err := parseDSN(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + dbName
	return u.String(), nil
}

func parseDSN(dsn string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("invalid dsn: missing scheme")
	}
	return u, nil
}
