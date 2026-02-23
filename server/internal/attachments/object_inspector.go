package attachments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrObjectListingUnsupported = errors.New("object listing unsupported")

type ObjectProbe struct {
	Exists    bool
	SizeBytes int64
	Checksum  string
}

type ObjectInventoryEntry struct {
	ObjectKey string `json:"objectKey"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	Checksum  string `json:"checksum,omitempty"`
}

type ObjectStoreInspector interface {
	Probe(ctx context.Context, objectKey string) (ObjectProbe, error)
	List(ctx context.Context, limit int) ([]ObjectInventoryEntry, error)
}

type SignedURLObjectInspector struct {
	signer       URLSigner
	client       *http.Client
	inventoryURL string
}

func NewSignedURLObjectInspector(signer URLSigner, inventoryURL string, timeout time.Duration) *SignedURLObjectInspector {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &SignedURLObjectInspector{
		signer:       signer,
		client:       &http.Client{Timeout: timeout},
		inventoryURL: strings.TrimSpace(inventoryURL),
	}
}

func (i *SignedURLObjectInspector) Probe(ctx context.Context, objectKey string) (ObjectProbe, error) {
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return ObjectProbe{}, fmt.Errorf("object key is required")
	}
	if i == nil || i.signer == nil {
		return ObjectProbe{}, fmt.Errorf("object inspector signer is not configured")
	}

	downloadURL, err := i.signer.SignDownload(objectKey, time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		return ObjectProbe{}, fmt.Errorf("sign probe download url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, downloadURL, nil)
	if err != nil {
		return ObjectProbe{}, fmt.Errorf("build probe request: %w", err)
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return ObjectProbe{}, fmt.Errorf("probe object head: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return i.probeWithRangeGet(ctx, downloadURL)
	default:
		return parseProbeResponse(resp)
	}
}

func (i *SignedURLObjectInspector) probeWithRangeGet(ctx context.Context, downloadURL string) (ObjectProbe, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return ObjectProbe{}, fmt.Errorf("build fallback probe request: %w", err)
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := i.client.Do(req)
	if err != nil {
		return ObjectProbe{}, fmt.Errorf("probe object range get: %w", err)
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return parseProbeResponse(resp)
}

func parseProbeResponse(resp *http.Response) (ObjectProbe, error) {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent:
		return ObjectProbe{
			Exists:    true,
			SizeBytes: parseObjectSize(resp),
			Checksum:  parseObjectChecksum(resp.Header),
		}, nil
	case http.StatusNotFound:
		return ObjectProbe{Exists: false}, nil
	default:
		return ObjectProbe{}, fmt.Errorf("probe returned status %d", resp.StatusCode)
	}
}

func parseObjectSize(resp *http.Response) int64 {
	if total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range")); ok {
		return total
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("Content-Length")), 10, 64); err == nil && n >= 0 {
		return n
	}
	return 0
}

func parseObjectChecksum(h http.Header) string {
	checksum := normalizeChecksum(h.Get("X-Amz-Meta-Sha256"))
	if checksum != "" {
		return checksum
	}
	return normalizeChecksum(h.Get("ETag"))
}

func normalizeChecksum(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "W/")
	v = strings.Trim(v, `"'`)
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.ToLower(v), "sha256:")
	return strings.TrimSpace(v)
}

func parseContentRangeTotal(v string) (int64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	parts := strings.Split(v, "/")
	if len(parts) != 2 {
		return 0, false
	}
	totalPart := strings.TrimSpace(parts[1])
	if totalPart == "" || totalPart == "*" {
		return 0, false
	}
	total, err := strconv.ParseInt(totalPart, 10, 64)
	if err != nil || total < 0 {
		return 0, false
	}
	return total, true
}

func (i *SignedURLObjectInspector) List(ctx context.Context, limit int) ([]ObjectInventoryEntry, error) {
	if i == nil || strings.TrimSpace(i.inventoryURL) == "" {
		return nil, ErrObjectListingUnsupported
	}

	u, err := url.Parse(i.inventoryURL)
	if err != nil {
		return nil, fmt.Errorf("parse inventory url: %w", err)
	}
	if limit > 0 {
		q := u.Query()
		q.Set("limit", strconv.Itoa(limit))
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build inventory request: %w", err)
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request inventory: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("inventory returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read inventory response: %w", err)
	}
	return parseInventoryEntries(body, limit)
}

func parseInventoryEntries(body []byte, limit int) ([]ObjectInventoryEntry, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode inventory payload: %w", err)
	}

	var rawItems []any
	switch v := payload.(type) {
	case []any:
		rawItems = v
	case map[string]any:
		if maybe, ok := v["objects"]; ok {
			s, ok := maybe.([]any)
			if !ok {
				return nil, fmt.Errorf("inventory objects field must be an array")
			}
			rawItems = s
			break
		}
		if maybe, ok := v["items"]; ok {
			s, ok := maybe.([]any)
			if !ok {
				return nil, fmt.Errorf("inventory items field must be an array")
			}
			rawItems = s
			break
		}
		return nil, fmt.Errorf("inventory payload must be an array or contain objects/items")
	default:
		return nil, fmt.Errorf("inventory payload has unsupported type %T", payload)
	}

	out := make([]ObjectInventoryEntry, 0, len(rawItems))
	seen := make(map[string]struct{}, len(rawItems))
	for _, raw := range rawItems {
		entry, ok := parseInventoryEntry(raw)
		if !ok {
			continue
		}
		if _, exists := seen[entry.ObjectKey]; exists {
			continue
		}
		seen[entry.ObjectKey] = struct{}{}
		out = append(out, entry)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func parseInventoryEntry(raw any) (ObjectInventoryEntry, bool) {
	switch v := raw.(type) {
	case string:
		key := strings.TrimSpace(v)
		if key == "" {
			return ObjectInventoryEntry{}, false
		}
		return ObjectInventoryEntry{ObjectKey: key}, true
	case map[string]any:
		key := firstString(v, "objectKey", "object_key", "key", "path", "name")
		key = strings.TrimSpace(key)
		if key == "" {
			return ObjectInventoryEntry{}, false
		}
		return ObjectInventoryEntry{
			ObjectKey: key,
			SizeBytes: firstInt64(v, "sizeBytes", "size_bytes", "size", "contentLength", "content_length"),
			Checksum:  normalizeChecksum(firstString(v, "checksum", "sha256", "etag", "hash")),
		}, true
	default:
		return ObjectInventoryEntry{}, false
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		val, ok := m[k]
		if !ok || val == nil {
			continue
		}
		switch v := val.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
	}
	return ""
}

func firstInt64(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		val, ok := m[k]
		if !ok || val == nil {
			continue
		}
		switch v := val.(type) {
		case float64:
			if v >= 0 {
				return int64(v)
			}
		case int64:
			if v >= 0 {
				return v
			}
		case int:
			if v >= 0 {
				return int64(v)
			}
		case json.Number:
			if parsed, err := v.Int64(); err == nil && parsed >= 0 {
				return parsed
			}
		case string:
			if parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && parsed >= 0 {
				return parsed
			}
		}
	}
	return 0
}
