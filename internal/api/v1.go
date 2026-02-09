package api

import (
	"io"
	"net/http"
)

func (api *API) handleV1ApiKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	writeJSON(w, http.StatusOK, []any{
		map[string]any{
			"name":        "anon",
			"api_key":     api.cfg.SupabaseAnonKey,
			"id":          "anon",
			"type":        "legacy",
			"hash":        "",
			"prefix":      "",
			"description": "Legacy anon API key",
		},
		map[string]any{
			"name":        "service_role",
			"api_key":     api.cfg.SupabaseServiceKey,
			"id":          "service_role",
			"type":        "legacy",
			"hash":        "",
			"prefix":      "",
			"description": "Legacy service_role API key",
		},
	})
}

func (api *API) handleFunctions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	functions, err := api.listFunctions()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	writeJSON(w, http.StatusOK, functions)
}

func (api *API) handleFunctionBySlug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	slug := chiURLParam(r, "slug")
	if slug == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "Missing function 'slug' parameter"}})
		return
	}
	function, err := api.getFunctionBySlug(slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "Function not found"}})
		return
	}
	writeJSON(w, http.StatusOK, function)
}

func (api *API) handleTypescriptTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	included := "public,graphql_public,storage"
	excluded := "auth,cron,extensions,graphql,net,pgsodium,pgsodium_masks,realtime,supabase_functions,supabase_migrations,vault,_analytics,_realtime"
	target := api.cfg.StudioPgMetaURL + "/generators/typescript?included_schema=" + included + "&excluded_schemas=" + excluded

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
		writeJSON(w, resp.StatusCode, map[string]any{"message": "Failed to generate types"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
