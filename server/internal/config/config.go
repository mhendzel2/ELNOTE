package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                    string
	DatabaseURL                 string
	JWTSecret                   string
	JWTIssuer                   string
	AccessTokenTTL              time.Duration
	RefreshTokenTTL             time.Duration
	MigrationsDir               string
	AutoMigrate                 bool
	RequireTLS                  bool
	ObjectStorePublicBaseURL    string
	ObjectStoreBucket           string
	ObjectStoreSignSecret       string
	AttachmentUploadURLTTL      time.Duration
	AttachmentDownloadURLTTL    time.Duration
	DefaultReconcileStaleAfter  time.Duration
	DefaultReconcileScanLimit   int
	ReconcileScheduleEnabled    bool
	ReconcileScheduleInterval   time.Duration
	ReconcileScheduleRunOnStart bool
	ReconcileScheduleActorEmail string
	SearchResultLimit           int
	PreviewMaxSizeBytes         int64
	NotificationRetentionDays   int
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:                   getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:                strings.TrimSpace(os.Getenv("DATABASE_URL")),
		JWTSecret:                  strings.TrimSpace(os.Getenv("JWT_SECRET")),
		JWTIssuer:                  getEnv("JWT_ISSUER", "elnote-api"),
		AccessTokenTTL:             getDurationEnv("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:            getDurationEnv("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		MigrationsDir:              getEnv("MIGRATIONS_DIR", "./migrations"),
		AutoMigrate:                getBoolEnv("AUTO_MIGRATE", true),
		RequireTLS:                 getBoolEnv("REQUIRE_TLS", false),
		ObjectStorePublicBaseURL:   getEnv("OBJECT_STORE_PUBLIC_BASE_URL", "http://localhost:9000"),
		ObjectStoreBucket:          getEnv("OBJECT_STORE_BUCKET", "elnote"),
		ObjectStoreSignSecret:      strings.TrimSpace(os.Getenv("OBJECT_STORE_SIGN_SECRET")),
		AttachmentUploadURLTTL:     getDurationEnv("ATTACHMENT_UPLOAD_URL_TTL", 15*time.Minute),
		AttachmentDownloadURLTTL:   getDurationEnv("ATTACHMENT_DOWNLOAD_URL_TTL", 15*time.Minute),
		DefaultReconcileStaleAfter: getDurationEnv("RECONCILE_STALE_AFTER", 24*time.Hour),
		DefaultReconcileScanLimit:  getIntEnv("RECONCILE_SCAN_LIMIT", 500),
		ReconcileScheduleEnabled:   getBoolEnv("RECONCILE_SCHEDULE_ENABLED", true),
		ReconcileScheduleInterval:  getDurationEnv("RECONCILE_SCHEDULE_INTERVAL", 24*time.Hour),
		ReconcileScheduleRunOnStart: getBoolEnv("RECONCILE_SCHEDULE_RUN_ON_STARTUP", false),
		ReconcileScheduleActorEmail: getEnv("RECONCILE_SCHEDULE_ACTOR_EMAIL", "labadmin"),
		SearchResultLimit:         getIntEnv("SEARCH_RESULT_LIMIT", 50),
		PreviewMaxSizeBytes:       int64(getIntEnv("PREVIEW_MAX_SIZE_BYTES", 10*1024*1024)),
		NotificationRetentionDays: getIntEnv("NOTIFICATION_RETENTION_DAYS", 90),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, errors.New("JWT_SECRET must be at least 32 characters")
	}
	if cfg.ObjectStoreSignSecret == "" {
		cfg.ObjectStoreSignSecret = cfg.JWTSecret
	}
	if cfg.DefaultReconcileScanLimit <= 0 {
		cfg.DefaultReconcileScanLimit = 500
	}
	if cfg.ReconcileScheduleInterval <= 0 {
		cfg.ReconcileScheduleInterval = 24 * time.Hour
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func getBoolEnv(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getIntEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}
