package api

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type pgMetaError struct {
	Message        string `json:"message"`
	Code           string `json:"code"`
	FormattedError string `json:"formattedError"`
}

func (api *API) pgMetaProxy(endpoint string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if api.cfg.StudioPgMetaURL == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"message": "STUDIO_PG_META_URL is required",
			})
			return
		}

		query := r.URL.RawQuery
		target := fmt.Sprintf("%s/%s", strings.TrimSuffix(api.cfg.StudioPgMetaURL, "/"), endpoint)
		if query != "" {
			target = target + "?" + query
		}

		headers, err := api.pgMetaHeaders(r, false)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
			return
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
			return
		}
		req.Header = headers

		resp, err := api.client.Do(req)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			message := extractErrorMessage(body)
			writeJSON(w, resp.StatusCode, map[string]any{"message": message})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
	}
}

func (api *API) handlePgMetaQuery(w http.ResponseWriter, r *http.Request) {
	if api.cfg.StudioPgMetaURL == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"message": "STUDIO_PG_META_URL is required",
		})
		return
	}

	var payload struct {
		Query string `json:"query"`
	}
	if err := decodeJSON(r, &payload); err != nil || payload.Query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"message":        "Invalid request body",
			"formattedError": "Invalid request body",
		})
		return
	}

	headers, err := api.pgMetaHeaders(r, false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
		return
	}

	body, _ := json.Marshal(map[string]any{
		"query": payload.Query,
	})

	target := fmt.Sprintf("%s/query", strings.TrimSuffix(api.cfg.StudioPgMetaURL, "/"))
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
		return
	}
	req.Header = headers

	resp, err := api.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var pgErr pgMetaError
		if err := json.Unmarshal(respBody, &pgErr); err == nil && pgErr.Message != "" {
			writeJSON(w, resp.StatusCode, map[string]any{
				"message":        pgErr.Message,
				"formattedError": pgErr.FormattedError,
			})
			return
		}
		message := extractErrorMessage(respBody)
		writeJSON(w, resp.StatusCode, map[string]any{
			"message":        message,
			"formattedError": message,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func (api *API) pgMetaExecute(r *http.Request, query string, readOnly bool) ([]byte, *pgMetaError, int, error) {
	headers, err := api.pgMetaHeaders(r, readOnly)
	if err != nil {
		return nil, nil, http.StatusInternalServerError, err
	}
	body, _ := json.Marshal(map[string]any{"query": query})
	target := fmt.Sprintf("%s/query", strings.TrimSuffix(api.cfg.StudioPgMetaURL, "/"))
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, nil, http.StatusInternalServerError, err
	}
	req.Header = headers
	resp, err := api.client.Do(req)
	if err != nil {
		return nil, nil, http.StatusInternalServerError, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var pgErr pgMetaError
		if err := json.Unmarshal(respBody, &pgErr); err == nil && pgErr.Message != "" {
			return respBody, &pgErr, resp.StatusCode, nil
		}
		return respBody, nil, resp.StatusCode, nil
	}
	return respBody, nil, resp.StatusCode, nil
}

func (api *API) pgMetaHeaders(r *http.Request, readOnly bool) (http.Header, error) {
	headers := http.Header{}
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	if auth := r.Header.Get("Authorization"); auth != "" {
		headers.Set("Authorization", auth)
	}
	if cookie := r.Header.Get("cookie"); cookie != "" {
		headers.Set("cookie", cookie)
	}

	connectionString := api.pgMetaConnectionString(readOnly)
	encrypted, err := encryptString(connectionString, api.cfg.PgMetaCryptoKey)
	if err != nil {
		return nil, err
	}
	headers.Set("x-connection-encrypted", encrypted)

	if api.cfg.SupabaseServiceKey != "" {
		headers.Set("apiKey", api.cfg.SupabaseServiceKey)
	}

	return headers, nil
}

func (api *API) pgMetaConnectionString(readOnly bool) string {
	user := api.cfg.PostgresUserReadWrite
	if readOnly {
		user = api.cfg.PostgresUserReadOnly
	}
	return fmt.Sprintf("postgresql://%s:%s@%s:%s/%s",
		user,
		api.cfg.PostgresPassword,
		api.cfg.PostgresHost,
		api.cfg.PostgresPort,
		api.cfg.PostgresDatabase,
	)
}

// encryptString matches CryptoJS AES encryption with passphrase (OpenSSL compatible).
func encryptString(value, passphrase string) (string, error) {
	if passphrase == "" {
		return "", errors.New("missing encryption key")
	}

	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	key, iv := evpBytesToKey([]byte(passphrase), salt, 32, 16)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	padded := pkcs7Pad([]byte(value), block.BlockSize())
	ciphertext := make([]byte, len(padded))

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	out := append([]byte("Salted__"), salt...)
	out = append(out, ciphertext...)
	return base64.StdEncoding.EncodeToString(out), nil
}

func evpBytesToKey(password, salt []byte, keyLen, ivLen int) ([]byte, []byte) {
	totalLen := keyLen + ivLen
	var derived []byte
	var prev []byte

	for len(derived) < totalLen {
		hash := md5.New()
		_, _ = hash.Write(prev)
		_, _ = hash.Write(password)
		_, _ = hash.Write(salt)
		prev = hash.Sum(nil)
		derived = append(derived, prev...)
	}

	return derived[:keyLen], derived[keyLen:totalLen]
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

func extractErrorMessage(body []byte) string {
	if len(body) == 0 {
		return "Internal Server Error"
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if msg, ok := payload["message"].(string); ok && msg != "" {
			return msg
		}
		if msg, ok := payload["error"].(string); ok && msg != "" {
			return msg
		}
		if errObj, ok := payload["error"].(map[string]any); ok {
			if msg, ok := errObj["message"].(string); ok && msg != "" {
				return msg
			}
		}
	}
	return string(bytes.TrimSpace(body))
}

func rewritePublicURL(input, publicBase string) string {
	if input == "" || publicBase == "" {
		return input
	}
	parsedInput, err := url.Parse(input)
	if err != nil {
		return input
	}
	publicURL, err := url.Parse(publicBase)
	if err != nil {
		return input
	}
	parsedInput.Scheme = publicURL.Scheme
	parsedInput.Host = publicURL.Host
	return parsedInput.String()
}
