package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"go.uber.org/zap"
)

var logger *zap.Logger = zap.NewNop()

// SetLogger installs the package logger used by the response helpers.
func SetLogger(l *zap.Logger) {
	if l != nil {
		logger = l
	}
}

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already on the wire, so there is nothing to do
		// but record it.
		logger.Error("failed to encode response", zap.Error(err))
	}
}

func respondError(w http.ResponseWriter, status int, message string, code ...string) {
	body := errorBody{Error: message}
	if len(code) > 0 {
		body.Code = code[0]
	}
	respondJSON(w, status, body)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_body")
		return false
	}
	return true
}

// readAll reads a reader fully with a hard cap, so a corrupt or hostile file
// cannot exhaust memory.
func readAll(r io.Reader) ([]byte, error) {
	const maxBytes = 20 << 20
	return io.ReadAll(io.LimitReader(r, maxBytes))
}

// hashFor fingerprints AI inputs for the result cache.
func hashFor(data []byte, text string) string {
	h := sha256.New()
	h.Write(data)
	h.Write([]byte{0})
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}
