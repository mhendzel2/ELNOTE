package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AccessClaims struct {
	Sub      string `json:"sub"`
	Role     string `json:"role"`
	DeviceID string `json:"device_id"`
	Iss      string `json:"iss"`
	Iat      int64  `json:"iat"`
	Exp      int64  `json:"exp"`
}

type TokenManager struct {
	secret          []byte
	issuer          string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewTokenManager(secret, issuer string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{
		secret:          []byte(secret),
		issuer:          issuer,
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}
}

func (m *TokenManager) IssueAccessToken(userID, role, deviceID string) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(m.accessTokenTTL)

	claims := AccessClaims{
		Sub:      userID,
		Role:     role,
		DeviceID: deviceID,
		Iss:      m.issuer,
		Iat:      now.Unix(),
		Exp:      expiresAt.Unix(),
	}

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal jwt header: %w", err)
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal jwt claims: %w", err)
	}

	headerPart := base64.RawURLEncoding.EncodeToString(headerBytes)
	payloadPart := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signingInput := headerPart + "." + payloadPart
	signature := signHMACSHA256(m.secret, signingInput)
	signaturePart := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + signaturePart, expiresAt, nil
}

func (m *TokenManager) ParseAccessToken(token string) (AccessClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return AccessClaims{}, errors.New("invalid token format")
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := signHMACSHA256(m.secret, signingInput)
	providedSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return AccessClaims{}, errors.New("invalid token signature encoding")
	}

	if !hmac.Equal(expectedSig, providedSig) {
		return AccessClaims{}, errors.New("invalid token signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AccessClaims{}, errors.New("invalid token payload encoding")
	}

	var claims AccessClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return AccessClaims{}, errors.New("invalid token payload")
	}

	now := time.Now().UTC().Unix()
	if claims.Exp < now {
		return AccessClaims{}, errors.New("token is expired")
	}
	if claims.Iss != m.issuer {
		return AccessClaims{}, errors.New("invalid token issuer")
	}

	return claims, nil
}

func (m *TokenManager) IssueRefreshToken() (token string, tokenHash string, expiresAt time.Time, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", time.Time{}, fmt.Errorf("generate refresh token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	tokenHash = hex.EncodeToString(sum[:])
	expiresAt = time.Now().UTC().Add(m.refreshTokenTTL)
	return token, tokenHash, expiresAt, nil
}

func (m *TokenManager) HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func signHMACSHA256(secret []byte, signingInput string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}
