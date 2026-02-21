package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}
