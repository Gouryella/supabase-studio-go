package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (api *API) storageBaseURL() string {
	return strings.TrimSuffix(api.cfg.SupabaseURL, "/") + "/storage/v1"
}

func (api *API) storageHeaders() http.Header {
	headers := http.Header{}
	if api.cfg.SupabaseServiceKey != "" {
		headers.Set("apikey", api.cfg.SupabaseServiceKey)
		headers.Set("Authorization", "Bearer "+api.cfg.SupabaseServiceKey)
	}
	headers.Set("Content-Type", "application/json")
	return headers
}

func (api *API) handleStorageBuckets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.storageProxy(w, r, http.MethodGet, api.storageBaseURL()+"/bucket", nil)
	case http.MethodPost:
		body, _ := readRawBody(r)
		normalizedBody := normalizeStorageCreateBucketBody(body)
		api.storageProxy(w, r, http.MethodPost, api.storageBaseURL()+"/bucket", normalizedBody)
	default:
		writeMethodNotAllowed(w, r, "GET, POST")
	}
}

func normalizeStorageCreateBucketBody(body []byte) []byte {
	if len(bytes.TrimSpace(body)) == 0 {
		return body
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	bucketID, _ := payload["id"].(string)
	if strings.TrimSpace(bucketID) == "" {
		bucketID, _ = payload["name"].(string)
	}
	if strings.TrimSpace(bucketID) == "" {
		return body
	}

	// Mirror official storage-js behavior: send both id and name.
	payload["id"] = bucketID
	payload["name"] = bucketID

	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return rewritten
}

func (api *API) handleStorageBucket(w http.ResponseWriter, r *http.Request) {
	bucket := chiURLParam(r, "id")
	if bucket == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "Bucket ID is required"}})
		return
	}
	target := api.storageBaseURL() + "/bucket/" + url.PathEscape(bucket)

	switch r.Method {
	case http.MethodGet:
		api.storageProxy(w, r, http.MethodGet, target, nil)
	case http.MethodPatch:
		body, _ := readRawBody(r)
		normalizedBody := normalizeStorageUpdateBucketBody(bucket, body)
		// Mirror official storage-js behavior: updateBucket uses PUT /bucket/{id}.
		api.storageProxy(w, r, http.MethodPut, target, normalizedBody)
	case http.MethodDelete:
		api.storageProxy(w, r, http.MethodDelete, target, nil)
	default:
		writeMethodNotAllowed(w, r, "GET, PATCH, DELETE")
	}
}

func normalizeStorageUpdateBucketBody(bucket string, body []byte) []byte {
	payload := map[string]any{}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return body
		}
	}

	trimmedBucket := strings.TrimSpace(bucket)
	if trimmedBucket != "" {
		// Mirror official storage-js behavior: send both id and name.
		payload["id"] = trimmedBucket
		payload["name"] = trimmedBucket
	}

	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return rewritten
}

func (api *API) handleStorageEmptyBucket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	bucket := chiURLParam(r, "id")
	target := api.storageBaseURL() + "/bucket/" + url.PathEscape(bucket) + "/empty"
	api.storageProxy(w, r, http.MethodPost, target, nil)
}

func (api *API) handleStorageObjectsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	bucket := chiURLParam(r, "id")
	var payload struct {
		Path    string         `json:"path"`
		Options map[string]any `json:"options"`
	}
	_ = decodeJSON(r, &payload)

	// Mirror official storage-js list() defaults.
	bodyMap := map[string]any{
		"limit":  100,
		"offset": 0,
		"sortBy": map[string]any{
			"column": "name",
			"order":  "asc",
		},
		"prefix": payload.Path,
	}
	for k, v := range payload.Options {
		bodyMap[k] = v
	}
	bodyBytes, _ := json.Marshal(bodyMap)
	target := api.storageBaseURL() + "/object/list/" + url.PathEscape(bucket)
	api.storageProxy(w, r, http.MethodPost, target, bodyBytes)
}

func (api *API) handleStorageObjectsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w, r, "DELETE")
		return
	}
	bucket := chiURLParam(r, "id")
	var payload struct {
		Paths []string `json:"paths"`
	}
	_ = decodeJSON(r, &payload)
	bodyBytes, _ := json.Marshal(map[string]any{
		"prefixes": payload.Paths,
	})
	target := api.storageBaseURL() + "/object/" + url.PathEscape(bucket)
	api.storageProxy(w, r, http.MethodDelete, target, bodyBytes)
}

func (api *API) handleStorageObjectsPublicURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	bucket := chiURLParam(r, "id")
	var payload struct {
		Path string `json:"path"`
	}
	_ = decodeJSON(r, &payload)

	publicBase := api.cfg.SupabasePublicURL
	if publicBase == "" {
		publicBase = api.cfg.SupabaseURL
	}
	publicURL := strings.TrimSuffix(publicBase, "/") + "/storage/v1/object/public/" + url.PathEscape(bucket) + "/" + strings.TrimPrefix(payload.Path, "/")

	writeJSON(w, http.StatusOK, map[string]any{"publicUrl": publicURL})
}

func (api *API) handleStorageObjectsSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	bucket := chiURLParam(r, "id")
	var payload struct {
		Path      string         `json:"path"`
		ExpiresIn int            `json:"expiresIn"`
		Options   map[string]any `json:"options"`
	}
	_ = decodeJSON(r, &payload)
	if strings.TrimSpace(payload.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "Path is required"}})
		return
	}
	if payload.ExpiresIn == 0 {
		payload.ExpiresIn = 60 * 60 * 24
	}
	bodyMap := map[string]any{
		"expiresIn": payload.ExpiresIn,
	}
	if transform, ok := payload.Options["transform"]; ok {
		bodyMap["transform"] = transform
	}
	bodyBytes, _ := json.Marshal(bodyMap)
	target := api.storageBaseURL() + "/object/sign/" + url.PathEscape(bucket) + "/" + escapeStorageObjectPath(payload.Path)
	respBody, status, err := api.storageRaw(r, http.MethodPost, target, bodyBytes)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}

	var response map[string]any
	if err := json.Unmarshal(respBody, &response); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(respBody)
		return
	}

	signedURL, _ := response["signedUrl"].(string)
	if signedURL == "" {
		signedURL, _ = response["signedURL"].(string)
	}
	if signedURL != "" {
		response["signedUrl"] = rewriteStorageSignedURL(signedURL, api.cfg.SupabasePublicURL)
		delete(response, "signedURL")
	}
	writeJSON(w, status, response)
}

func rewriteStorageSignedURL(input, publicBase string) string {
	rewritten := rewritePublicURL(input, publicBase)
	if rewritten == "" {
		return rewritten
	}

	parsedURL, err := url.Parse(rewritten)
	if err != nil {
		return rewritten
	}

	if strings.HasPrefix(parsedURL.Path, "/storage/v1/") {
		return rewritten
	}

	if strings.HasPrefix(parsedURL.Path, "/object/") {
		parsedURL.Path = "/storage/v1" + parsedURL.Path
		return parsedURL.String()
	}

	return rewritten
}

func escapeStorageObjectPath(path string) string {
	trimmedPath := strings.TrimPrefix(path, "/")
	if trimmedPath == "" {
		return ""
	}

	pathParts := strings.Split(trimmedPath, "/")
	for idx, part := range pathParts {
		pathParts[idx] = url.PathEscape(part)
	}
	return strings.Join(pathParts, "/")
}

func (api *API) handleStorageObjectsDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	bucket := chiURLParam(r, "id")
	var payload struct {
		Path string `json:"path"`
	}
	_ = decodeJSON(r, &payload)
	target := api.storageBaseURL() + "/object/" + url.PathEscape(bucket) + "/" + strings.TrimPrefix(payload.Path, "/")
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	req.Header = api.storageHeaders()

	resp, err := api.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "Internal Server Error"}})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body)
}

func (api *API) handleStorageObjectsMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	bucket := chiURLParam(r, "id")
	var payload struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	_ = decodeJSON(r, &payload)
	bodyBytes, _ := json.Marshal(map[string]any{
		"bucketId":       bucket,
		"sourceKey":      payload.From,
		"destinationKey": payload.To,
	})
	target := api.storageBaseURL() + "/object/move"
	api.storageProxy(w, r, http.MethodPost, target, bodyBytes)
}

func (api *API) storageProxy(w http.ResponseWriter, r *http.Request, method, target string, body []byte) {
	respBody, status, err := api.storageRaw(r, method, target, body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(respBody)
}

func (api *API) storageRaw(r *http.Request, method, target string, body []byte) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, target, reader)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header = api.storageHeaders()

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return respBody, resp.StatusCode, nil
	}
	return respBody, resp.StatusCode, nil
}
