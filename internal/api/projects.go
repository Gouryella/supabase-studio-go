package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type endpointInfo struct {
	host     string
	protocol string
	origin   string
}

func (api *API) getProjectName() string {
	api.mu.RLock()
	defer api.mu.RUnlock()

	if api.projectName == "" {
		return api.cfg.DefaultProjectName
	}
	return api.projectName
}

func (api *API) setProjectName(name string) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.projectName = name
}

func (api *API) getProjectDiskSize() int {
	api.mu.RLock()
	defer api.mu.RUnlock()

	if api.projectDiskSize <= 0 {
		if api.cfg.DefaultProjectDiskSizeGB <= 0 {
			return 8
		}
		return api.cfg.DefaultProjectDiskSizeGB
	}
	return api.projectDiskSize
}

func (api *API) setProjectDiskSize(size int) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.projectDiskSize = size
}

func (api *API) projectEndpoint() endpointInfo {
	publicURL := api.cfg.SupabasePublicURL
	if publicURL == "" {
		publicURL = "http://localhost:8000"
	}
	parsed, err := url.Parse(publicURL)
	if err != nil {
		return endpointInfo{host: "localhost:8000", protocol: "http", origin: "http://localhost:8000"}
	}
	return endpointInfo{
		host:     parsed.Host,
		protocol: strings.TrimSuffix(parsed.Scheme, ":"),
		origin:   parsed.Scheme + "://" + parsed.Host,
	}
}

func (api *API) projectRestURL() string {
	endpoint := api.projectEndpoint()
	return endpoint.origin + "/rest/v1/"
}

func (api *API) defaultProject() map[string]any {
	diskSize := api.getProjectDiskSize()

	return map[string]any{
		"id":                  1,
		"ref":                 "default",
		"name":                api.getProjectName(),
		"organization_id":     1,
		"cloud_provider":      "localhost",
		"status":              "ACTIVE_HEALTHY",
		"region":              "local",
		"inserted_at":         "2021-08-02T06:40:40.646Z",
		"volumeSizeGb":        diskSize,
		"disk_volume_size_gb": diskSize,
	}
}

func (api *API) handleProjectsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	writeJSON(w, http.StatusOK, []any{api.defaultProject()})
}

func (api *API) handleProjectDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	response := api.defaultProject()
	response["connectionString"] = ""
	response["restUrl"] = api.projectRestURL()
	writeJSON(w, http.StatusOK, response)
}

func (api *API) handleProjectUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeMethodNotAllowed(w, r, "PATCH")
		return
	}

	var payload struct {
		Name string `json:"name"`
	}

	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{"message": "Invalid request body"},
		})
		return
	}

	name := strings.TrimSpace(payload.Name)
	switch {
	case len(name) < 3:
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{"message": "Project name must be at least 3 characters long"},
		})
		return
	case len(name) > 64:
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{"message": "Project name must be at most 64 characters long"},
		})
		return
	}

	if err := api.updateProjectName(name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{"message": "Failed to persist project settings"},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":   1,
		"ref":  "default",
		"name": name,
	})
}

func (api *API) handleProjectDisk(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"attributes": map[string]any{
				"iops":             3000,
				"size_gb":          api.getProjectDiskSize(),
				"throughput_mbps":  125,
				"throughput_mibps": 125,
				"type":             "gp3",
			},
		})
	case http.MethodPost:
		var payload struct {
			Attributes struct {
				SizeGB int `json:"size_gb"`
			} `json:"attributes"`
		}

		if err := decodeJSON(r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]any{"message": "Invalid request body"},
			})
			return
		}

		if payload.Attributes.SizeGB < 1 || payload.Attributes.SizeGB > 65536 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]any{"message": "Disk size must be between 1 GB and 65536 GB"},
			})
			return
		}

		if err := api.updateProjectDiskSize(payload.Attributes.SizeGB); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": map[string]any{"message": "Failed to persist disk settings"},
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"attributes": map[string]any{
				"iops":             3000,
				"size_gb":          payload.Attributes.SizeGB,
				"throughput_mbps":  125,
				"throughput_mibps": 125,
				"type":             "gp3",
			},
			"last_modified_at": time.Now().UTC().Format(time.RFC3339),
		})
	default:
		writeMethodNotAllowed(w, r, "GET, POST")
	}
}

func (api *API) handleProjectDiskUtilization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	const bytesPerGiB = int64(1024 * 1024 * 1024)

	databaseSizeBytes, err := api.queryInt64FromPgMeta(
		r,
		`select coalesce(sum(pg_database_size(datname)) filter (where datname not in ('template0', 'template1')), 0)::bigint as db_size_bytes from pg_database`,
		"db_size_bytes",
		true,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{"message": "Failed to query database size"},
		})
		return
	}

	walSizeBytes, err := api.queryInt64FromPgMeta(
		r,
		`select coalesce(sum(size), 0)::bigint as wal_size_bytes from pg_ls_waldir()`,
		"wal_size_bytes",
		true,
	)
	if err != nil {
		walSizeBytes = 0
	}

	systemBytes, err := api.queryInt64FromPgMeta(
		r,
		`select coalesce(sum(pg_database_size(datname)) filter (where datname in ('template0', 'template1')), 0)::bigint as system_size_bytes from pg_database`,
		"system_size_bytes",
		true,
	)
	if err != nil {
		systemBytes = 0
	}

	totalSizeBytes := int64(api.getProjectDiskSize()) * bytesPerGiB
	usedBytes := databaseSizeBytes + walSizeBytes + systemBytes
	if usedBytes > totalSizeBytes {
		usedBytes = totalSizeBytes
	}
	if usedBytes < 0 {
		usedBytes = 0
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"metrics": map[string]any{
			"fs_avail_bytes": totalSizeBytes - usedBytes,
			"fs_size_bytes":  totalSizeBytes,
			"fs_used_bytes":  usedBytes,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (api *API) handleProjectResize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload struct {
		VolumeSizeGB int `json:"volume_size_gb"`
	}

	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{"message": "Invalid request body"},
		})
		return
	}

	if payload.VolumeSizeGB < 1 || payload.VolumeSizeGB > 65536 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{"message": "Disk size must be between 1 GB and 65536 GB"},
		})
		return
	}

	if err := api.updateProjectDiskSize(payload.VolumeSizeGB); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{"message": "Failed to persist disk settings"},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"volume_size_gb": payload.VolumeSizeGB,
	})
}

func (api *API) queryInt64FromPgMeta(
	r *http.Request,
	sql string,
	column string,
	readOnly bool,
) (int64, error) {
	body, pgErr, _, err := api.pgMetaExecute(r, sql, readOnly)
	if err != nil {
		return 0, err
	}
	if pgErr != nil {
		return 0, fmt.Errorf("pg-meta query failed: %s", pgErr.Message)
	}

	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	return int64FromAny(rows[0][column])
}

func int64FromAny(value any) (int64, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case float64:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, nil
		}
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, strconv.ErrSyntax
	}
}

func (api *API) handleProjectSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	endpoint := api.projectEndpoint()
	response := map[string]any{
		"app_config": map[string]any{
			"db_schema":        "public",
			"endpoint":         endpoint.host,
			"storage_endpoint": endpoint.host,
			"protocol":         endpoint.protocol,
		},
		"cloud_provider":    "AWS",
		"db_dns_name":       "-",
		"db_host":           "localhost",
		"db_ip_addr_config": "legacy",
		"db_name":           "postgres",
		"db_port":           5432,
		"db_user":           "postgres",
		"inserted_at":       "2021-08-02T06:40:40.646Z",
		"jwt_secret":        api.cfg.AuthJWTSecret,
		"name":              api.getProjectName(),
		"ref":               "default",
		"region":            "ap-southeast-1",
		"service_api_keys": []any{
			map[string]any{
				"api_key": api.cfg.SupabaseServiceKey,
				"name":    "service_role key",
				"tags":    "service_role",
			},
			map[string]any{
				"api_key": api.cfg.SupabaseAnonKey,
				"name":    "anon key",
				"tags":    "anon",
			},
		},
		"ssl_enforced": false,
		"status":       "ACTIVE_HEALTHY",
	}

	writeJSON(w, http.StatusOK, response)
}

func (api *API) handleProjectDatabases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	response := []any{
		map[string]any{
			"cloud_provider":              "localhost",
			"connectionString":            "",
			"connection_string_read_only": "",
			"db_host":                     "127.0.0.1",
			"db_name":                     "postgres",
			"db_port":                     5432,
			"db_user":                     "postgres",
			"identifier":                  "default",
			"inserted_at":                 "",
			"region":                      "local",
			"restUrl":                     api.projectRestURL(),
			"size":                        "",
			"status":                      "ACTIVE_HEALTHY",
		},
	}
	writeJSON(w, http.StatusOK, response)
}

func (api *API) handleProjectRest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET, HEAD")
		return
	}

	target := strings.TrimSuffix(api.cfg.SupabaseURL, "/") + "/rest/v1/"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "Internal Server Error"}})
		return
	}
	req.Header.Set("apikey", api.cfg.SupabaseServiceKey)
	resp, err := api.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "Internal Server Error"}})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "Internal Server Error"}})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func (api *API) handleProjectGraphql(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	authorization := r.Header.Get("x-graphql-authorization")
	if authorization == "" {
		authorization = "Bearer " + api.cfg.SupabaseAnonKey
	}
	body, _ := readRawBody(r)
	target := strings.TrimSuffix(api.cfg.SupabaseURL, "/") + "/graphql/v1"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "Internal Server Error"}})
		return
	}
	req.Header.Set("apikey", api.cfg.SupabaseServiceKey)
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json")

	resp, err := api.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "Internal Server Error"}})
		return
	}
	defer resp.Body.Close()
	bodyResp, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "Internal Server Error"}})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(bodyResp)
}

func (api *API) handleProjectTempAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_key": api.cfg.SupabaseServiceKey})
}

func (api *API) handleProjectInfraMonitoring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":       []any{},
		"yAxisLimit": 0,
		"format":     "%",
		"total":      0,
	})
}

func (api *API) handleProjectBillingAddons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ref":              "",
		"selected_addons":  []any{},
		"available_addons": []any{},
	})
}

func (api *API) handleProjectConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"db_anon_role":         "anon",
			"db_extra_search_path": "public",
			"db_schema":            "public, storage",
			"jwt_secret":           api.cfg.AuthJWTSecret,
			"max_rows":             100,
			"role_claim_key":       ".role",
		})
	case http.MethodPatch:
		writeJSON(w, http.StatusOK, map[string]any{})
	default:
		writeMethodNotAllowed(w, r, "GET")
	}
}

func (api *API) handleProjectPostgrestConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"db_anon_role":         "anon",
		"db_extra_search_path": "public",
		"db_schema":            "public, storage",
		"jwt_secret":           api.cfg.AuthJWTSecret,
		"max_rows":             100,
		"role_claim_key":       ".role",
	})
}

func (api *API) handleProjectAnalyticsEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "GET, POST")
		return
	}

	name := chiURLParam(r, "name")
	projectRef := chiURLParam(r, "ref")
	if name == "" || projectRef == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "Missing parameters"}})
		return
	}

	params := map[string]string{}
	if r.Method == http.MethodGet {
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				params[key] = values[0]
			}
		}
	} else {
		_ = decodeJSON(r, &params)
	}

	data, err := api.retrieveAnalyticsData(r, name, projectRef, params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (api *API) handleProjectLogDrains(w http.ResponseWriter, r *http.Request) {
	if missing := api.missingLogflareEnv(); len(missing) > 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": strings.Join(missing, ", ") + " env variables are not set"}})
		return
	}

	switch r.Method {
	case http.MethodGet:
		url := api.cfg.LogflareURL + "/api/backends?metadata[type]=log-drain"
		api.logflareProxy(w, r, http.MethodGet, url, nil)
	case http.MethodPost:
		body, _ := readRawBody(r)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		payload["metadata"] = map[string]any{"type": "log-drain"}
		body, _ = json.Marshal(payload)
		url := api.cfg.LogflareURL + "/api/backends"
		respBody, status, err := api.logflareRaw(r, http.MethodPost, url, body)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
			return
		}

		sourcesBody, _, _ := api.logflareRaw(r, http.MethodGet, api.cfg.LogflareURL+"/api/sources", nil)
		var sources []map[string]any
		_ = json.Unmarshal(sourcesBody, &sources)

		var postResult map[string]any
		_ = json.Unmarshal(respBody, &postResult)
		backendID := postResult["id"]

		for _, source := range sources {
			name, _ := source["name"].(string)
			if !isAllowedLogSource(name) {
				continue
			}
			param := map[string]any{
				"backend_id": backendID,
				"lql_string": "~\".*?\"",
				"source_id":  source["id"],
			}
			bodyRule, _ := json.Marshal(param)
			_, _, _ = api.logflareRaw(r, http.MethodPost, api.cfg.LogflareURL+"/api/rules", bodyRule)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(respBody)
	default:
		writeMethodNotAllowed(w, r, "GET, POST, PUT, DELETE")
	}
}

func (api *API) handleProjectLogDrain(w http.ResponseWriter, r *http.Request) {
	if missing := api.missingLogflareEnv(); len(missing) > 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": strings.Join(missing, ", ") + " env variables are not set"}})
		return
	}
	uuid := chiURLParam(r, "uuid")
	if uuid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "Missing uuid"}})
		return
	}
	target := api.cfg.LogflareURL + "/api/backends/" + uuid
	switch r.Method {
	case http.MethodGet:
		api.logflareProxy(w, r, http.MethodGet, target, nil)
	case http.MethodPut:
		body, _ := readRawBody(r)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		delete(payload, "metadata")
		body, _ = json.Marshal(payload)
		api.logflareProxy(w, r, http.MethodPut, target, body)
	case http.MethodDelete:
		_, _, _ = api.logflareRaw(r, http.MethodDelete, target, nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r, "GET, POST")
	}
}

func isAllowedLogSource(name string) bool {
	switch strings.ToLower(name) {
	case "cloudflare.logs.prod",
		"deno-relay-logs",
		"deno-subhosting-events",
		"gotrue.logs.prod",
		"pgbouncer.logs.prod",
		"postgrest.logs.prod",
		"postgres.logs",
		"realtime.logs.prod",
		"storage.logs.prod.2":
		return true
	default:
		return false
	}
}

func (api *API) missingLogflareEnv() []string {
	var missing []string
	if api.cfg.LogflareToken == "" {
		missing = append(missing, "LOGFLARE_PRIVATE_ACCESS_TOKEN")
	}
	if api.cfg.LogflareURL == "" {
		missing = append(missing, "LOGFLARE_URL")
	}
	return missing
}

func (api *API) logflareRaw(r *http.Request, method, target string, body []byte) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, target, reader)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Set("Authorization", "Bearer "+api.cfg.LogflareToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

func (api *API) logflareProxy(w http.ResponseWriter, r *http.Request, method, target string, body []byte) {
	respBody, status, err := api.logflareRaw(r, method, target, body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(respBody)
}
