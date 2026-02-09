package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (api *API) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeMethodNotAllowed(w, r, "")
}

func writeMethodNotAllowed(w http.ResponseWriter, r *http.Request, allow string) {
	if allow != "" {
		w.Header().Set("Allow", allow)
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"data":  nil,
		"error": map[string]any{"message": "Method " + r.Method + " Not Allowed"},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("missing body")
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(target)
}

func readRawBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if strings.EqualFold(parts[0], "bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func respondNotImplemented(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"data":  nil,
		"error": map[string]any{"message": message},
	})
}

func chiURLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}
