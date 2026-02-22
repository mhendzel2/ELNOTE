package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mjhen/elnote/server/internal/app"
	"github.com/mjhen/elnote/server/internal/config"
	internaldb "github.com/mjhen/elnote/server/internal/db"
	"github.com/mjhen/elnote/server/internal/migrate"
)

type testEnv struct {
	t          *testing.T
	db         *sql.DB
	app        *app.App
	httpSrv    *httptest.Server
	baseURL    string
	adminToken string
	client     *http.Client
}

func setupIntegrationEnv(t *testing.T) *testEnv {
	t.Helper()

	if strings.TrimSpace(os.Getenv("ELNOTE_INTEGRATION")) != "1" {
		t.Skip("set ELNOTE_INTEGRATION=1 to run integration tests")
	}

	testDSN := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if testDSN == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}

	dbName, err := databaseNameFromDSN(testDSN)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	if !strings.Contains(strings.ToLower(dbName), "test") {
		t.Fatalf("refusing to run integration tests against non-test database name %q", dbName)
	}

	ctx := context.Background()
	db, err := internaldb.Open(ctx, testDSN)
	if err != nil {
		if strings.Contains(err.Error(), "SQLSTATE 3D000") {
			if createErr := ensureDatabaseExists(ctx, testDSN, dbName); createErr != nil {
				t.Fatalf("create test db %s: %v", dbName, createErr)
			}
			db, err = internaldb.Open(ctx, testDSN)
		}
		if err != nil {
			t.Fatalf("open test db: %v", err)
		}
	}

	if err := resetDatabase(ctx, db); err != nil {
		t.Fatalf("reset test db: %v", err)
	}

	if err := migrate.Run(ctx, db, migrationsDir(t)); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	cfg := config.Config{
		HTTPAddr:                    ":0",
		DatabaseURL:                 testDSN,
		JWTSecret:                   "integration-jwt-secret-abcdefghijklmnopqrstuvwxyz",
		JWTIssuer:                   "elnote-integration",
		AccessTokenTTL:              15 * time.Minute,
		RefreshTokenTTL:             24 * time.Hour,
		MigrationsDir:               migrationsDir(t),
		AutoMigrate:                 false,
		RequireTLS:                  false,
		ObjectStorePublicBaseURL:    "http://object-store.local",
		ObjectStoreBucket:           "elnote",
		ObjectStoreSignSecret:       "integration-sign-secret-abcdefghijklmnopqrstuvwxyz",
		AttachmentUploadURLTTL:      15 * time.Minute,
		AttachmentDownloadURLTTL:    15 * time.Minute,
		DefaultReconcileStaleAfter:  24 * time.Hour,
		DefaultReconcileScanLimit:   500,
		ReconcileScheduleEnabled:    false,
		ReconcileScheduleInterval:   24 * time.Hour,
		ReconcileScheduleRunOnStart: false,
		ReconcileScheduleActorEmail: "labadmin",
		SearchResultLimit:           50,
		PreviewMaxSizeBytes:         10 * 1024 * 1024,
		NotificationRetentionDays:   90,
	}

	application, err := app.New(cfg, db)
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	if err := application.SeedDefaultAdmin(ctx); err != nil {
		t.Fatalf("seed default admin: %v", err)
	}

	httpSrv := httptest.NewServer(application)
	env := &testEnv{
		t:       t,
		db:      db,
		app:     application,
		httpSrv: httpSrv,
		baseURL: httpSrv.URL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}

	t.Cleanup(func() {
		httpSrv.Close()
		_ = application.Close()
	})

	env.adminToken = env.login("labadmin", "CCI#3341", "integration-admin")
	return env
}

func resetDatabase(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;`)
	return err
}

func migrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "migrations"))
}

func databaseNameFromDSN(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", fmt.Errorf("missing database name in dsn")
	}
	return name, nil
}

func ensureDatabaseExists(ctx context.Context, testDSN, dbName string) error {
	adminDSN, err := withDatabaseName(testDSN, "postgres")
	if err != nil {
		return err
	}

	adminDB, err := internaldb.Open(ctx, adminDSN)
	if err != nil {
		return err
	}
	defer adminDB.Close()

	_, err = adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %s`, quoteIdent(dbName)))
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return err
	}
	return nil
}

func withDatabaseName(dsn, dbName string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + dbName
	return u.String(), nil
}

func quoteIdent(v string) string {
	return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
}

func (e *testEnv) login(email, password, deviceName string) string {
	e.t.Helper()
	status, _, _, body := e.doJSON(http.MethodPost, "/v1/auth/login", "", map[string]any{
		"email":      email,
		"password":   password,
		"deviceName": deviceName,
	})
	if status != http.StatusOK {
		e.t.Fatalf("login %s failed: status=%d body=%v", email, status, body)
	}
	m := asMap(e.t, body)
	token, ok := m["accessToken"].(string)
	if !ok || token == "" {
		e.t.Fatalf("missing accessToken in login response: %v", m)
	}
	return token
}

func (e *testEnv) createUser(email, password, role string) string {
	e.t.Helper()
	status, _, _, body := e.doJSON(http.MethodPost, "/v1/users", e.adminToken, map[string]any{
		"email":    email,
		"password": password,
		"role":     role,
	})
	if status != http.StatusCreated {
		e.t.Fatalf("create user %s failed: status=%d body=%v", email, status, body)
	}
	m := asMap(e.t, body)
	userID, ok := m["userId"].(string)
	if !ok || userID == "" {
		e.t.Fatalf("missing userId in create user response: %v", m)
	}
	return userID
}

func (e *testEnv) createExperiment(token, title, body string) map[string]any {
	e.t.Helper()
	status, _, _, resp := e.doJSON(http.MethodPost, "/v1/experiments", token, map[string]any{
		"title":        title,
		"originalBody": body,
	})
	if status != http.StatusCreated {
		e.t.Fatalf("create experiment failed: status=%d body=%v", status, resp)
	}
	return asMap(e.t, resp)
}

func (e *testEnv) doJSON(method, path, token string, body any) (int, http.Header, []byte, any) {
	e.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, e.baseURL+path, bodyReader)
	if err != nil {
		e.t.Fatalf("create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("http request failed (%s %s): %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read response body: %v", err)
	}

	var decoded any
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			decoded = string(raw)
		}
	}

	return resp.StatusCode, resp.Header.Clone(), raw, decoded
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T (%v)", v, v)
	}
	return m
}

func asSlice(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("expected slice response, got %T (%v)", v, v)
	}
	return s
}

func getString(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	s, ok := m[key].(string)
	if !ok {
		t.Fatalf("expected string field %q in %v", key, m)
	}
	return s
}

func getBool(t *testing.T, m map[string]any, key string) bool {
	t.Helper()
	b, ok := m[key].(bool)
	if !ok {
		t.Fatalf("expected bool field %q in %v", key, m)
	}
	return b
}
