package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/mjhen/elnote/server/internal/app"
	"github.com/mjhen/elnote/server/internal/config"
	"github.com/mjhen/elnote/server/internal/db"
	"github.com/mjhen/elnote/server/internal/migrate"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer database.Close()

	if cfg.AutoMigrate {
		if err := migrate.Run(ctx, database, cfg.MigrationsDir); err != nil {
			log.Fatalf("run migrations: %v", err)
		}
	}

	application, err := app.New(cfg, database)
	if err != nil {
		log.Fatalf("build app: %v", err)
	}

	if err := application.Run(ctx); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
