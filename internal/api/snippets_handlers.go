package api

import (
	"net/http"
	"strconv"
	"strings"
)

func (api *API) handleSnippets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.handleSnippetsGet(w, r)
	case http.MethodPut:
		api.handleSnippetsPut(w, r)
	case http.MethodDelete:
		api.handleSnippetsDelete(w, r)
	default:
		writeMethodNotAllowed(w, r, "GET, PUT, DELETE")
	}
}

func (api *API) handleSnippetsGet(w http.ResponseWriter, r *http.Request) {
	visibility := r.URL.Query().Get("visibility")
	if visibility == "project" {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}

	limit := parseLimit(r)
	if limit < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"data": []any{}})
		return
	}

	cursor := r.URL.Query().Get("cursor")
	sortBy := r.URL.Query().Get("sort_by")
	sortOrder := r.URL.Query().Get("sort_order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	nextCursor, snippets, err := api.getSnippets(r.URL.Query().Get("name"), limit, cursor, sortBy, sortOrder, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"data": []any{}})
		return
	}
	resp := map[string]any{"data": snippets}
	if nextCursor != "" {
		resp["cursor"] = nextCursor
	}
	writeJSON(w, http.StatusOK, resp)
}

func (api *API) handleSnippetsPut(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	id, _ := payload["id"].(string)
	updated, err := api.updateSnippet(id, payload)
	if err == nil {
		writeJSON(w, http.StatusOK, updated)
		return
	}

	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	name, _ := payload["name"].(string)
	contentPayload, _ := payload["content"].(map[string]any)
	sql, _ := contentPayload["sql"].(string)
	folderID := (*string)(nil)
	if folderRaw, ok := payload["folder_id"].(string); ok {
		folderID = &folderRaw
	}
	newSnippet := snippet{
		ID:   payload["id"].(string),
		Name: name,
		Content: snippetContent{
			SQL:           sql,
			ContentID:     "",
			SchemaVersion: "1.0",
		},
		FolderID: folderID,
	}
	saved, err := api.saveSnippet(newSnippet)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to create snippet"})
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (api *API) handleSnippetsDelete(w http.ResponseWriter, r *http.Request) {
	ids := r.URL.Query().Get("ids")
	if ids == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Snippet IDs are required"})
		return
	}
	idsList := strings.Split(ids, ",")
	var deleted []map[string]any
	for _, id := range idsList {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if err := api.deleteSnippet(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to delete snippets"})
			return
		}
		deleted = append(deleted, map[string]any{"id": id})
	}
	writeJSON(w, http.StatusOK, deleted)
}

func (api *API) handleSnippetCount(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	_, snippets, err := api.getSnippets(name, 0, "", "", "desc", nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "Failed to get count"})
		return
	}
	if name != "" {
		writeJSON(w, http.StatusOK, map[string]any{"count": len(snippets)})
		return
	}

	shared := 0
	favorites := 0
	private := 0
	for _, snippet := range snippets {
		if snippet.Visibility == "project" {
			shared++
		}
		if snippet.Favorite {
			favorites++
		}
		if snippet.Visibility == "user" {
			private++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"shared":    shared,
		"favorites": favorites,
		"private":   private,
	})
}

func (api *API) handleSnippetFolders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		folders, err := api.getFolders(nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
			return
		}
		cursor, snippets, err := api.getSnippets(r.URL.Query().Get("name"), parseLimit(r), r.URL.Query().Get("cursor"), r.URL.Query().Get("sort_by"), r.URL.Query().Get("sort_order"), nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
			return
		}
		resp := map[string]any{"data": map[string]any{"folders": folders, "contents": snippets}}
		if cursor != "" {
			resp["cursor"] = cursor
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var payload map[string]any
		if err := decodeJSON(r, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
			return
		}
		name, _ := payload["name"].(string)
		folder, err := api.createFolder(name)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, folder)
	case http.MethodDelete:
		ids := r.URL.Query().Get("ids")
		if ids == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Folder IDs are required"})
			return
		}
		for _, id := range strings.Split(ids, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if err := api.deleteFolder(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{})
	default:
		writeMethodNotAllowed(w, r, "GET, POST, DELETE")
	}
}

func (api *API) handleSnippetFolderByID(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPatch {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET, PATCH")
		return
	}
	folderID := chiURLParam(r, "id")
	folders, err := api.getFolders(&folderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
		return
	}
	cursor, snippets, err := api.getSnippets(r.URL.Query().Get("name"), parseLimit(r), r.URL.Query().Get("cursor"), r.URL.Query().Get("sort_by"), r.URL.Query().Get("sort_order"), &folderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error()})
		return
	}
	resp := map[string]any{"data": map[string]any{"folders": folders, "contents": snippets}}
	if cursor != "" {
		resp["cursor"] = cursor
	}
	writeJSON(w, http.StatusOK, resp)
}

func (api *API) handleSnippetItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	id := chiURLParam(r, "id")
	snippet, err := api.getSnippet(id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "Content not found."})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"data": nil, "error": map[string]any{"message": "Internal Server Error"}})
		return
	}
	writeJSON(w, http.StatusOK, snippet)
}

func parseLimit(r *http.Request) int {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return 0
	}
	parsed, err := strconv.Atoi(limitStr)
	if err != nil {
		return -1 // Signal invalid input
	}
	return parsed
}
