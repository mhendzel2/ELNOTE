package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	saltLen      int    = 16
)

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("password cannot be empty")
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	saltEncoded := base64.RawStdEncoding.EncodeToString(salt)
	hashEncoded := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, saltEncoded, hashEncoded)
	return encoded, nil
}

func VerifyPassword(encodedHash, password string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, errors.New("invalid hash format")
	}
	if parts[1] != "argon2id" {
		return false, errors.New("unsupported hash algorithm")
	}
	if parts[2] != "v=19" {
		return false, errors.New("unsupported argon2 version")
	}

	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false, errors.New("invalid hash parameters")
	}

	memory, err := parseUint32(params[0], "m=")
	if err != nil {
		return false, err
	}
	timeCost, err := parseUint32(params[1], "t=")
	if err != nil {
		return false, err
	}
	threads64, err := parseUint64(params[2], "p=")
	if err != nil {
		return false, err
	}
	threads := uint8(threads64)

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errors.New("invalid salt encoding")
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errors.New("invalid hash encoding")
	}

	derived := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(hash)))
	if subtle.ConstantTimeCompare(hash, derived) == 1 {
		return true, nil
	}
	return false, nil
}

func parseUint32(s, prefix string) (uint32, error) {
	v, err := parseUint64(s, prefix)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}

func parseUint64(s, prefix string) (uint64, error) {
	if !strings.HasPrefix(s, prefix) {
		return 0, fmt.Errorf("invalid parameter: %s", s)
	}
	value, err := strconv.ParseUint(strings.TrimPrefix(s, prefix), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid parameter value: %s", s)
	}
	return value, nil
}
