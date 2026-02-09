package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (api *API) authBaseURL() string {
	return strings.TrimSuffix(api.cfg.SupabaseURL, "/") + "/auth/v1"
}

func (api *API) authHeaders() http.Header {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	if api.cfg.SupabaseServiceKey != "" {
		headers.Set("Authorization", "Bearer "+api.cfg.SupabaseServiceKey)
		headers.Set("apikey", api.cfg.SupabaseServiceKey)
	}
	return headers
}

func (api *API) handleAuthInvite(w http.ResponseWriter, r *http.Request) {
	api.authProxySimple(w, r, "/invite")
}

func (api *API) handleAuthMagicLink(w http.ResponseWriter, r *http.Request) {
	api.authProxySimple(w, r, "/magiclink")
}

func (api *API) handleAuthRecover(w http.ResponseWriter, r *http.Request) {
	api.authProxySimple(w, r, "/recover")
}

func (api *API) handleAuthOTP(w http.ResponseWriter, r *http.Request) {
	api.authProxySimple(w, r, "/otp")
}

func (api *API) authProxySimple(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	body, _ := readRawBody(r)
	api.authProxy(w, r, http.MethodPost, path, body)
}

func (api *API) handleAuthUsersCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	body, _ := readRawBody(r)
	api.authProxy(w, r, http.MethodPost, "/admin/users", body)
}

func (api *API) handleAuthUser(w http.ResponseWriter, r *http.Request) {
	userID := chiURLParam(r, "id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "Missing user id"}})
		return
	}

	switch r.Method {
	case http.MethodGet:
		api.authProxy(w, r, http.MethodGet, "/admin/users/"+userID, nil)
	case http.MethodPut:
		body, _ := readRawBody(r)
		api.authProxy(w, r, http.MethodPut, "/admin/users/"+userID, body)
	case http.MethodDelete:
		api.authProxy(w, r, http.MethodDelete, "/admin/users/"+userID, nil)
	default:
		writeMethodNotAllowed(w, r, "GET, PUT, DELETE")
	}
}

func (api *API) handleAuthUserFactors(w http.ResponseWriter, r *http.Request) {
	respondNotImplemented(w, "MFA factor management is not available in the Go runtime")
}

func (api *API) authProxy(w http.ResponseWriter, r *http.Request, method, path string, body []byte) {
	if strings.TrimSpace(api.cfg.SupabaseServiceKey) == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"message": "Missing service key. Set SUPABASE_SERVICE_KEY (or SUPABASE_SERVICE_ROLE_KEY / SERVICE_ROLE_KEY / SERVICE_KEY).",
		})
		return
	}

	target := api.authBaseURL() + path
	resp, respBody, err := api.doAuthRequest(r, method, target, body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
		return
	}
	defer resp.Body.Close()

	// Some proxies strip custom headers before forwarding to Kong/Gotrue.
	// Retry once with `apikey` as query parameter to mirror key-auth query mode.
	if isNoAPIKeyResponse(resp.StatusCode, respBody) && strings.TrimSpace(api.cfg.SupabaseServiceKey) != "" {
		retryTarget := withAPIKeyQuery(target, api.cfg.SupabaseServiceKey)
		retryResp, retryBody, retryErr := api.doAuthRequest(r, method, retryTarget, body)
		if retryErr == nil {
			resp.Body.Close()
			resp = retryResp
			respBody = retryBody
			defer resp.Body.Close()
		}
	}

	if resp.StatusCode >= 400 {
		var parsed map[string]any
		if err := json.Unmarshal(respBody, &parsed); err == nil {
			if msg, ok := parsed["message"].(string); ok {
				writeJSON(w, resp.StatusCode, map[string]any{"message": msg})
				return
			}
			if errObj, ok := parsed["error"].(string); ok {
				writeJSON(w, resp.StatusCode, map[string]any{"message": errObj})
				return
			}
		}
		writeJSON(w, resp.StatusCode, map[string]any{"message": "Internal Server Error"})
		return
	}

	if path == "/admin/users" {
		var parsed map[string]any
		if err := json.Unmarshal(respBody, &parsed); err == nil {
			if user, ok := parsed["user"]; ok {
				writeJSON(w, http.StatusOK, user)
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func (api *API) doAuthRequest(r *http.Request, method, target string, body []byte) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, target, reader)
	if err != nil {
		return nil, nil, err
	}
	req.Header = api.authHeaders()

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, nil, err
	}
	return resp, respBody, nil
}

func isNoAPIKeyResponse(statusCode int, body []byte) bool {
	if statusCode != http.StatusUnauthorized {
		return false
	}
	return strings.Contains(strings.ToLower(string(body)), "no api key found in request")
}

func withAPIKeyQuery(target, apiKey string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return target
	}
	query := parsed.Query()
	query.Set("apikey", apiKey)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
