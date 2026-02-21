package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Run(ctx context.Context, db *sql.DB, migrationsDir string) error {
	if err := ensureMigrationTable(ctx, db); err != nil {
		return err
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".sql") {
			files = append(files, filepath.Join(migrationsDir, name))
		}
	}
	sort.Strings(files)

	for _, path := range files {
		version := filepath.Base(path)
		applied, err := isApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration tx %s: %w", version, err)
		}

		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", version, err)
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("track migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
	}

	return nil
}

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}
	return nil
}

func isApplied(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE version = $1`, version).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return true, nil
}
