package api

import (
	"net/http"
)

func (api *API) handleOrganizations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	response := []map[string]any{
		{
			"id":            1,
			"name":          api.cfg.DefaultOrganizationName,
			"slug":          "default-org-slug",
			"billing_email": "billing@supabase.co",
			"plan": map[string]any{
				"id":   "enterprise",
				"name": "Enterprise",
			},
		},
	}
	writeJSON(w, http.StatusOK, response)
}

func (api *API) handleOrgSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	response := map[string]any{
		"billing_cycle_anchor":  0,
		"current_period_end":    0,
		"current_period_start":  0,
		"next_invoice_at":       0,
		"usage_billing_enabled": false,
		"plan": map[string]any{
			"id":   "enterprise",
			"name": "Enterprise",
		},
		"addons":                []any{},
		"project_addons":        []any{},
		"payment_method_type":   "",
		"billing_via_partner":   false,
		"billing_partner":       "fly",
		"scheduled_plan_change": nil,
		"customer_balance":      0,
	}
	writeJSON(w, http.StatusOK, response)
}

func (api *API) handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	project := api.defaultProject()
	response := map[string]any{
		"id":            1,
		"primary_email": "johndoe@supabase.io",
		"username":      "johndoe",
		"first_name":    "John",
		"last_name":     "Doe",
		"organizations": []any{
			map[string]any{
				"id":            1,
				"name":          api.cfg.DefaultOrganizationName,
				"slug":          "default-org-slug",
				"billing_email": "billing@supabase.co",
				"projects": []any{
					map[string]any{
						"id":               project["id"],
						"ref":              project["ref"],
						"name":             project["name"],
						"organization_id":  project["organization_id"],
						"cloud_provider":   project["cloud_provider"],
						"status":           project["status"],
						"region":           project["region"],
						"inserted_at":      project["inserted_at"],
						"connectionString": "",
					},
				},
			},
		},
	}
	writeJSON(w, http.StatusOK, response)
}

func (api *API) handlePropsProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	project := api.defaultProject()
	response := map[string]any{
		"project": map[string]any{
			"id":              project["id"],
			"ref":             project["ref"],
			"name":            project["name"],
			"organization_id": project["organization_id"],
			"cloud_provider":  project["cloud_provider"],
			"status":          project["status"],
			"region":          project["region"],
			"inserted_at":     project["inserted_at"],
			"services":        []any{},
		},
	}
	writeJSON(w, http.StatusOK, response)
}

func (api *API) handlePropsProjectAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	project := api.defaultProject()
	endpoint := api.projectEndpoint()
	response := map[string]any{
		"project": map[string]any{
			"id":                         project["id"],
			"ref":                        project["ref"],
			"name":                       project["name"],
			"organization_id":            project["organization_id"],
			"cloud_provider":             project["cloud_provider"],
			"status":                     project["status"],
			"region":                     project["region"],
			"inserted_at":                project["inserted_at"],
			"api_key_supabase_encrypted": "",
			"db_host":                    "localhost",
			"db_name":                    "postgres",
			"db_port":                    5432,
			"db_ssl":                     false,
			"db_user":                    "postgres",
			"services": []any{
				map[string]any{
					"id":   1,
					"name": "Default API",
					"app": map[string]any{
						"id":   1,
						"name": "Auto API",
					},
					"app_config": map[string]any{
						"db_schema":        "public",
						"endpoint":         endpoint.host,
						"realtime_enabled": true,
					},
					"service_api_keys": []any{
						map[string]any{
							"api_key_encrypted": "-",
							"name":              "service_role key",
							"tags":              "service_role",
						},
						map[string]any{
							"api_key_encrypted": "-",
							"name":              "anon key",
							"tags":              "anon",
						},
					},
				},
			},
		},
		"autoApiService": map[string]any{
			"id":   1,
			"name": "Default API",
			"project": map[string]any{
				"ref": "default",
			},
			"app": map[string]any{
				"id":   1,
				"name": "Auto API",
			},
			"app_config": map[string]any{
				"db_schema":        "public",
				"endpoint":         endpoint.host,
				"realtime_enabled": true,
			},
			"protocol":      endpoint.protocol,
			"endpoint":      endpoint.host,
			"restUrl":       api.projectRestURL(),
			"defaultApiKey": api.cfg.SupabaseAnonKey,
			"serviceApiKey": api.cfg.SupabaseServiceKey,
			"service_api_keys": []any{
				map[string]any{
					"api_key_encrypted": "-",
					"name":              "service_role key",
					"tags":              "service_role",
				},
				map[string]any{
					"api_key_encrypted": "-",
					"name":              "anon key",
					"tags":              "anon",
				},
			},
		},
	}

	writeJSON(w, http.StatusOK, response)
}

func (api *API) handlePropsOrg(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	response := map[string]any{
		"organization": map[string]any{
			"id":            1,
			"name":          api.cfg.DefaultOrganizationName,
			"slug":          "default-org-slug",
			"billing_email": "billing@supabase.co",
			"plan": map[string]any{
				"id":   "enterprise",
				"name": "Enterprise",
			},
		},
	}
	writeJSON(w, http.StatusOK, response)
}

func (api *API) handleGithubConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": []any{}})
}

func (api *API) handleGithubAuthorization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (api *API) handleGithubRepositories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repositories": []any{}})
}

func (api *API) handleIntegrationBySlug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	writeJSON(w, http.StatusOK, []any{})
}

func (api *API) handleTelemetryEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (api *API) handleDatabasePooling(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"project": map[string]any{
				"db_port":           6543,
				"pool_mode":         "transaction",
				"pgbouncer_enabled": true,
				"pgbouncer_status":  "COMING_UP",
			},
		})
	case http.MethodPatch:
		writeJSON(w, http.StatusOK, map[string]any{})
	default:
		writeMethodNotAllowed(w, r, "GET, PATCH")
	}
}
