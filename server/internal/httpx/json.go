package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const maxBodyBytes = 1 << 20 // 1 MiB

type ErrorResponse struct {
	Error string `json:"error"`
}

func DecodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return err
	}
	if len(payload) > maxBodyBytes {
		return errors.New("request body exceeds 1 MiB limit")
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{Error: message})
}
