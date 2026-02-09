package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

func (api *API) handleMigrations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.handleListMigrations(w, r)
	case http.MethodPost:
		api.handleApplyMigration(w, r)
	default:
		writeMethodNotAllowed(w, r, "GET, POST")
	}
}

func (api *API) handleListMigrations(w http.ResponseWriter, r *http.Request) {
	query := "select version, name from supabase_migrations.schema_migrations order by version"
	body, pgErr, status, err := api.pgMetaExecute(r, query, false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error(), "formattedError": err.Error()})
		return
	}
	if pgErr != nil {
		if pgErr.Code == "42P01" {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		writeJSON(w, status, map[string]any{"message": pgErr.Message, "formattedError": pgErr.FormattedError})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

func (api *API) handleApplyMigration(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Query string `json:"query"`
		Name  string `json:"name"`
	}
	if err := decodeJSON(r, &payload); err != nil || payload.Query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "Invalid request body", "formattedError": "Invalid request body"})
		return
	}

	initQuery := `begin;

create schema if not exists supabase_migrations;
create table if not exists supabase_migrations.schema_migrations (version text not null primary key);
alter table supabase_migrations.schema_migrations add column if not exists statements text[];
alter table supabase_migrations.schema_migrations add column if not exists name text;

commit;`

	if _, pgErr, status, err := api.pgMetaExecute(r, initQuery, false); err != nil || pgErr != nil {
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error(), "formattedError": err.Error()})
		} else {
			writeJSON(w, status, map[string]any{"message": pgErr.Message, "formattedError": pgErr.FormattedError})
		}
		return
	}

	applyQuery := buildMigrationQuery(payload.Query, payload.Name)
	body, pgErr, status, err := api.pgMetaExecute(r, applyQuery, false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": err.Error(), "formattedError": err.Error()})
		return
	}
	if pgErr != nil {
		writeJSON(w, status, map[string]any{"message": pgErr.Message, "formattedError": pgErr.FormattedError})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

func buildMigrationQuery(query, name string) string {
	dollar := "$" + randomString(20) + "$"
	quote := func(value string) string {
		if value == "" {
			return "''"
		}
		return dollar + value + dollar
	}
	return strings.Join([]string{
		"begin;",
		query + ";",
		"insert into supabase_migrations.schema_migrations (version, name, statements)",
		"values (",
		"  to_char(current_timestamp, 'YYYYMMDDHH24MISS'),",
		"  " + quote(name) + ",",
		"  array[" + quote(query) + "]",
		");",
		"commit;",
	}, "\n")
}

func randomString(length int) string {
	bytes := make([]byte, length)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}
