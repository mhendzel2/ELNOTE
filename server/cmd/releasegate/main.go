package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const expectedSchemaVersion = "pilot-uat-go-live-v1"

var requiredChecklistItems = []string{
	"uatRepresentativeWorkflows",
	"forensicExportValidated",
	"runbookFinalized",
	"pilotAcceptanceApproved",
	"auditVerifyPassing",
	"reconcileFindingsTriaged",
	"backupDrillCurrent",
	"objectStorageDrillCurrent",
	"keyRotationValidated",
}

type releaseGateArtifact struct {
	SchemaVersion  string          `json:"schemaVersion"`
	ReleaseVersion string          `json:"releaseVersion"`
	SignedBy       string          `json:"signedBy"`
	SignedRole     string          `json:"signedRole"`
	SignedAtUTC    string          `json:"signedAtUtc"`
	SignatureRef   string          `json:"signatureRef"`
	Checklist      map[string]bool `json:"checklist"`
	Evidence       []evidenceItem  `json:"evidence"`
	Notes          string          `json:"notes,omitempty"`
}

type evidenceItem struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
	URL         string `json:"url,omitempty"`
}

func main() {
	artifactPath := filepath.Clean(filepath.Join("..", "docs", "release-gates", "pilot-uat-go-live.json"))
	flag.StringVar(&artifactPath, "artifact", artifactPath, "path to release gate artifact JSON")
	flag.Parse()

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read artifact %s: %v\n", artifactPath, err)
		os.Exit(1)
	}

	var artifact releaseGateArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		fmt.Fprintf(os.Stderr, "parse artifact %s: %v\n", artifactPath, err)
		os.Exit(1)
	}

	if err := validateArtifact(filepath.Dir(artifactPath), &artifact); err != nil {
		fmt.Fprintf(os.Stderr, "release gate validation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Release gate validated: %s (%s)\n", artifact.ReleaseVersion, artifactPath)
}

func validateArtifact(artifactDir string, artifact *releaseGateArtifact) error {
	if strings.TrimSpace(artifact.SchemaVersion) != expectedSchemaVersion {
		return fmt.Errorf("schemaVersion must be %q", expectedSchemaVersion)
	}
	if strings.TrimSpace(artifact.ReleaseVersion) == "" {
		return fmt.Errorf("releaseVersion is required")
	}
	if strings.TrimSpace(artifact.SignedBy) == "" {
		return fmt.Errorf("signedBy is required")
	}
	if strings.TrimSpace(artifact.SignedRole) == "" {
		return fmt.Errorf("signedRole is required")
	}
	if strings.TrimSpace(artifact.SignatureRef) == "" {
		return fmt.Errorf("signatureRef is required")
	}
	if strings.TrimSpace(artifact.SignedAtUTC) == "" {
		return fmt.Errorf("signedAtUtc is required")
	}
	if _, err := time.Parse(time.RFC3339, artifact.SignedAtUTC); err != nil {
		return fmt.Errorf("signedAtUtc must be RFC3339: %w", err)
	}
	if len(artifact.Checklist) == 0 {
		return fmt.Errorf("checklist is required")
	}

	for _, key := range requiredChecklistItems {
		value, ok := artifact.Checklist[key]
		if !ok {
			return fmt.Errorf("checklist missing %q", key)
		}
		if !value {
			return fmt.Errorf("checklist item %q must be true", key)
		}
	}

	if len(artifact.Evidence) == 0 {
		return fmt.Errorf("at least one evidence entry is required")
	}
	for i, item := range artifact.Evidence {
		if strings.TrimSpace(item.Type) == "" {
			return fmt.Errorf("evidence[%d].type is required", i)
		}
		hasPath := strings.TrimSpace(item.Path) != ""
		hasURL := strings.TrimSpace(item.URL) != ""
		if !hasPath && !hasURL {
			return fmt.Errorf("evidence[%d] requires path or url", i)
		}
		if hasPath {
			resolved := item.Path
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Clean(filepath.Join(artifactDir, "..", "..", item.Path))
			}
			info, err := os.Stat(resolved)
			if err != nil {
				return fmt.Errorf("evidence[%d] path does not exist: %s", i, item.Path)
			}
			if info.IsDir() {
				return fmt.Errorf("evidence[%d] path is a directory: %s", i, item.Path)
			}
		}
	}

	return nil
}
