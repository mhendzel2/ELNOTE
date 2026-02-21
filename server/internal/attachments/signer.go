package attachments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type URLSigner interface {
	SignUpload(objectKey string, expiresAt time.Time) (string, error)
	SignDownload(objectKey string, expiresAt time.Time) (string, error)
}

type HMACURLSigner struct {
	baseURL string
	bucket  string
	secret  []byte
}

func NewHMACURLSigner(baseURL, bucket, secret string) (*HMACURLSigner, error) {
	baseURL = strings.TrimSpace(baseURL)
	bucket = strings.TrimSpace(bucket)
	secret = strings.TrimSpace(secret)
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}
	if bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	if secret == "" {
		return nil, fmt.Errorf("secret is required")
	}

	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("parse baseURL: %w", err)
	}

	return &HMACURLSigner{
		baseURL: strings.TrimRight(baseURL, "/"),
		bucket:  bucket,
		secret:  []byte(secret),
	}, nil
}

func (s *HMACURLSigner) SignUpload(objectKey string, expiresAt time.Time) (string, error) {
	return s.signURL("put", objectKey, expiresAt)
}

func (s *HMACURLSigner) SignDownload(objectKey string, expiresAt time.Time) (string, error) {
	return s.signURL("get", objectKey, expiresAt)
}

func (s *HMACURLSigner) signURL(operation, objectKey string, expiresAt time.Time) (string, error) {
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return "", fmt.Errorf("objectKey is required")
	}
	if strings.Contains(objectKey, "..") {
		return "", fmt.Errorf("objectKey must not contain '..'")
	}

	expiresUnix := expiresAt.UTC().Unix()
	canonical := operation + "\n" + s.bucket + "\n" + objectKey + "\n" + strconv.FormatInt(expiresUnix, 10)
	sig := signHMACSHA256Hex(s.secret, canonical)

	escapedObjectKey := escapeObjectKeyPath(objectKey)
	signedURL := fmt.Sprintf(
		"%s/%s/%s?op=%s&exp=%d&sig=%s",
		s.baseURL,
		url.PathEscape(s.bucket),
		escapedObjectKey,
		url.QueryEscape(operation),
		expiresUnix,
		url.QueryEscape(sig),
	)

	return signedURL, nil
}

func escapeObjectKeyPath(objectKey string) string {
	parts := strings.Split(objectKey, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func signHMACSHA256Hex(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
