package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	internaldb "github.com/mjhen/elnote/server/internal/db"
)

type drillArtifact struct {
	RunID          string           `json:"runId"`
	StartedAtUTC   time.Time        `json:"startedAtUtc"`
	FinishedAtUTC  time.Time        `json:"finishedAtUtc"`
	APIBaseURL     string           `json:"apiBaseUrl"`
	AdminEmail     string           `json:"adminEmail"`
	DeviceName     string           `json:"deviceName"`
	RestoreCommand string           `json:"restoreCommand,omitempty"`
	Sample         sampleAttachment `json:"sample"`
	Checks         drillChecks      `json:"checks"`
	Steps          []drillStep      `json:"steps"`
	Success        bool             `json:"success"`
	Error          string           `json:"error,omitempty"`
	GapsAndActions []string         `json:"gapsAndActions,omitempty"`
}

type sampleAttachment struct {
	AttachmentID string `json:"attachmentId"`
	ExperimentID string `json:"experimentId"`
	ObjectKey    string `json:"objectKey"`
	Checksum     string `json:"checksum"`
	SizeBytes    int64  `json:"sizeBytes"`
}

type drillChecks struct {
	LoginOK                bool   `json:"loginOk"`
	DownloadMetadataOK     bool   `json:"downloadMetadataOk"`
	SignedURLFetchOK       bool   `json:"signedUrlFetchOk"`
	SignedURLFetchStatus   int    `json:"signedUrlFetchStatus"`
	SignedURLFetchURL      string `json:"signedUrlFetchUrl,omitempty"`
	SignedURLContentLength int64  `json:"signedUrlContentLength,omitempty"`
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	sourceDSN := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	apiBaseURL := strings.TrimSpace(os.Getenv("OBJECT_DRILL_API_BASE_URL"))
	if apiBaseURL == "" {
		apiBaseURL = "http://localhost:8080"
	}
	adminEmail := strings.TrimSpace(os.Getenv("OBJECT_DRILL_ADMIN_EMAIL"))
	if adminEmail == "" {
		adminEmail = "labadmin"
	}
	adminPassword := strings.TrimSpace(os.Getenv("OBJECT_DRILL_ADMIN_PASSWORD"))
	deviceName := strings.TrimSpace(os.Getenv("OBJECT_DRILL_DEVICE_NAME"))
	if deviceName == "" {
		deviceName = "ops-object-drill"
	}
	artifactDir := filepath.Clean(filepath.Join("..", "docs", "drills", "object-storage"))
	attachmentID := strings.TrimSpace(os.Getenv("OBJECT_DRILL_ATTACHMENT_ID"))
	restoreCommand := strings.TrimSpace(os.Getenv("OBJECT_DRILL_RESTORE_CMD"))
	maxDownloadBytes := int64(1024)

	flag.StringVar(&sourceDSN, "source-dsn", sourceDSN, "source Postgres DSN (defaults to DATABASE_URL)")
	flag.StringVar(&apiBaseURL, "api-base-url", apiBaseURL, "API base URL")
	flag.StringVar(&adminEmail, "admin-email", adminEmail, "admin email for login")
	flag.StringVar(&adminPassword, "admin-password", adminPassword, "admin password for login")
	flag.StringVar(&deviceName, "device-name", deviceName, "device name for login")
	flag.StringVar(&artifactDir, "artifact-dir", artifactDir, "directory for drill evidence artifacts")
	flag.StringVar(&attachmentID, "attachment-id", attachmentID, "optional completed attachment id to validate")
	flag.StringVar(&restoreCommand, "restore-command", restoreCommand, "optional shell command to restore sample object before validation")
	flag.Int64Var(&maxDownloadBytes, "max-download-bytes", maxDownloadBytes, "max bytes to fetch from signed URL for verification")
	flag.Parse()

	if sourceDSN == "" {
		fmt.Fprintln(os.Stderr, "source dsn is required (set --source-dsn or DATABASE_URL)")
		os.Exit(2)
	}
	if strings.TrimSpace(adminPassword) == "" {
		fmt.Fprintln(os.Stderr, "admin password is required (set --admin-password or OBJECT_DRILL_ADMIN_PASSWORD)")
		os.Exit(2)
	}
	if maxDownloadBytes <= 0 {
		maxDownloadBytes = 1024
	}

	runID := time.Now().UTC().Format("20060102T150405Z")
	artifact := drillArtifact{
		RunID:          runID,
		StartedAtUTC:   time.Now().UTC(),
		APIBaseURL:     strings.TrimRight(strings.TrimSpace(apiBaseURL), "/"),
		AdminEmail:     adminEmail,
		DeviceName:     deviceName,
		RestoreCommand: restoreCommand,
		Checks:         drillChecks{},
		GapsAndActions: []string{
			"This drill validates signed URL retrieval for a restored sample object. Configure storage-native snapshot/version restore outside this command if required.",
		},
	}

	db, err := internaldb.Open(ctx, sourceDSN)
	if err != nil {
		finishWithError(artifactDir, &artifact, fmt.Errorf("connect source db: %w", err))
	}
	defer db.Close()

	artifact.Sample, err = loadSampleAttachment(ctx, db, attachmentID)
	if err != nil {
		finishWithError(artifactDir, &artifact, err)
	}

	if restoreCommand != "" {
		artifact.Steps = append(artifact.Steps, runStep(ctx, "restore_sample_object", func() (string, error) {
			return runShellCommand(ctx, restoreCommand)
		}))
		if !artifact.Steps[len(artifact.Steps)-1].Success {
			finishWithError(artifactDir, &artifact, errors.New(artifact.Steps[len(artifact.Steps)-1].Error))
		}
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}

	var accessToken string
	artifact.Steps = append(artifact.Steps, runStep(ctx, "api_login", func() (string, error) {
		token, err := login(ctx, httpClient, artifact.APIBaseURL, adminEmail, adminPassword, deviceName)
		if err != nil {
			return "", err
		}
		accessToken = token
		artifact.Checks.LoginOK = true
		return "login succeeded", nil
	}))
	if !artifact.Steps[len(artifact.Steps)-1].Success {
		finishWithError(artifactDir, &artifact, errors.New(artifact.Steps[len(artifact.Steps)-1].Error))
	}

	var downloadURL string
	artifact.Steps = append(artifact.Steps, runStep(ctx, "request_signed_download_url", func() (string, error) {
		url, err := getDownloadURL(ctx, httpClient, artifact.APIBaseURL, accessToken, artifact.Sample.AttachmentID)
		if err != nil {
			return "", err
		}
		downloadURL = url
		artifact.Checks.DownloadMetadataOK = true
		artifact.Checks.SignedURLFetchURL = downloadURL
		return "download URL issued", nil
	}))
	if !artifact.Steps[len(artifact.Steps)-1].Success {
		finishWithError(artifactDir, &artifact, errors.New(artifact.Steps[len(artifact.Steps)-1].Error))
	}

	artifact.Steps = append(artifact.Steps, runStep(ctx, "fetch_signed_url", func() (string, error) {
		status, contentLength, err := fetchSignedURL(ctx, httpClient, downloadURL, maxDownloadBytes)
		artifact.Checks.SignedURLFetchStatus = status
		artifact.Checks.SignedURLContentLength = contentLength
		if err != nil {
			return "", err
		}
		artifact.Checks.SignedURLFetchOK = true
		return fmt.Sprintf("signed URL fetch succeeded (status=%d)", status), nil
	}))
	if !artifact.Steps[len(artifact.Steps)-1].Success {
		finishWithError(artifactDir, &artifact, errors.New(artifact.Steps[len(artifact.Steps)-1].Error))
	}

	artifact.Success = true
	artifact.FinishedAtUTC = time.Now().UTC()
	path, err := writeArtifact(artifactDir, &artifact)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write artifact: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Object-storage drill completed successfully. Artifact: %s\n", path)
}

func loadSampleAttachment(ctx context.Context, db *sql.DB, attachmentID string) (sampleAttachment, error) {
	out := sampleAttachment{}
	attachmentID = strings.TrimSpace(attachmentID)
	if attachmentID != "" {
		err := db.QueryRowContext(ctx, `
			SELECT
				a.id::text,
				a.experiment_id::text,
				a.object_key,
				COALESCE(a.checksum, ''),
				a.size_bytes
			FROM attachments a
			JOIN experiments e ON e.id = a.experiment_id
			WHERE a.id = $1::uuid
			  AND a.status = 'completed'
			  AND e.status = 'completed'
			LIMIT 1
		`, attachmentID).Scan(&out.AttachmentID, &out.ExperimentID, &out.ObjectKey, &out.Checksum, &out.SizeBytes)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sampleAttachment{}, fmt.Errorf("attachment %s not found or not completed", attachmentID)
			}
			return sampleAttachment{}, fmt.Errorf("load attachment %s: %w", attachmentID, err)
		}
		return out, nil
	}

	err := db.QueryRowContext(ctx, `
		SELECT
			a.id::text,
			a.experiment_id::text,
			a.object_key,
			COALESCE(a.checksum, ''),
			a.size_bytes
		FROM attachments a
		JOIN experiments e ON e.id = a.experiment_id
		WHERE a.status = 'completed'
		  AND e.status = 'completed'
		ORDER BY a.completed_at DESC NULLS LAST
		LIMIT 1
	`).Scan(&out.AttachmentID, &out.ExperimentID, &out.ObjectKey, &out.Checksum, &out.SizeBytes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sampleAttachment{}, fmt.Errorf("no completed attachment found for drill")
		}
		return sampleAttachment{}, fmt.Errorf("load latest completed attachment: %w", err)
	}
	return out, nil
}

func login(ctx context.Context, client *http.Client, apiBaseURL, email, password, deviceName string) (string, error) {
	reqBody := map[string]any{
		"email":      email,
		"password":   password,
		"deviceName": deviceName,
	}
	blob, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(apiBaseURL, "/")+"/v1/auth/login", bytes.NewReader(blob))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return "", fmt.Errorf("login response missing accessToken")
	}
	return out.AccessToken, nil
}

func getDownloadURL(ctx context.Context, client *http.Client, apiBaseURL, accessToken, attachmentID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiBaseURL, "/")+"/v1/attachments/"+attachmentID+"/download", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download metadata request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download metadata failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode download metadata response: %w", err)
	}
	if strings.TrimSpace(out.DownloadURL) == "" {
		return "", fmt.Errorf("download metadata missing downloadUrl")
	}
	return out.DownloadURL, nil
}

func fetchSignedURL(ctx context.Context, client *http.Client, downloadURL string, maxBytes int64) (int, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("signed URL request failed: %w", err)
	}
	defer resp.Body.Close()

	contentLength := resp.ContentLength
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBytes))

	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent:
		return resp.StatusCode, contentLength, nil
	default:
		return resp.StatusCode, contentLength, fmt.Errorf("signed URL fetch returned status %d", resp.StatusCode)
	}
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

func runShellCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("restore command failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func finishWithError(artifactDir string, artifact *drillArtifact, err error) {
	artifact.Success = false
	artifact.Error = err.Error()
	artifact.FinishedAtUTC = time.Now().UTC()
	path, writeErr := writeArtifact(artifactDir, artifact)
	if writeErr != nil {
		fmt.Fprintf(os.Stderr, "drill failed: %v\nwrite artifact failed: %v\n", err, writeErr)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "drill failed: %v\nartifact: %s\n", err, path)
	os.Exit(1)
}

func writeArtifact(artifactDir string, artifact *drillArtifact) (string, error) {
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return "", fmt.Errorf("create artifact dir: %w", err)
	}
	filePath := filepath.Join(artifactDir, fmt.Sprintf("object-drill-%s.json", artifact.RunID))
	blob, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal artifact: %w", err)
	}
	if err := os.WriteFile(filePath, blob, 0o644); err != nil {
		return "", fmt.Errorf("write artifact file: %w", err)
	}
	return filePath, nil
}
