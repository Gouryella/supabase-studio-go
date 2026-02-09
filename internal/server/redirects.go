package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/Gouryella/supabase-studio-go/internal/config"
	"github.com/go-chi/chi/v5"
)

type redirectRule struct {
	source    string
	target    string
	permanent bool
}

func registerRedirects(r chi.Router, cfg config.Config) {
	maintenanceMode := strings.EqualFold(os.Getenv("MAINTENANCE_MODE"), "true")
	if maintenanceMode {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if strings.HasPrefix(req.URL.Path, "/maintenance") || strings.HasPrefix(req.URL.Path, "/img") {
					next.ServeHTTP(w, req)
					return
				}
				http.Redirect(w, req, "/maintenance", http.StatusTemporaryRedirect)
			})
		})
	} else {
		r.Get("/maintenance", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/", http.StatusTemporaryRedirect)
		})
	}

	if cfg.IsPlatform {
		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Query().Get("next") == "new-project" {
				http.Redirect(w, req, "/new/new-project", http.StatusTemporaryRedirect)
				return
			}
			http.Redirect(w, req, "/org", http.StatusTemporaryRedirect)
		})
		r.Get("/register", redirectHandler("/sign-up", false))
		r.Get("/signup", redirectHandler("/sign-up", false))
		r.Get("/signin", redirectHandler("/sign-in", false))
		r.Get("/login", redirectHandler("/sign-in", false))
		r.Get("/log-in", redirectHandler("/sign-in", false))
	} else {
		r.Get("/", redirectHandler("/project/default", false))
		r.Get("/register", redirectHandler("/project/default", false))
		r.Get("/signup", redirectHandler("/project/default", false))
		r.Get("/signin", redirectHandler("/project/default", false))
		r.Get("/login", redirectHandler("/project/default", false))
		r.Get("/log-in", redirectHandler("/project/default", false))
	}

	common := []redirectRule{
		{source: "/project/{ref}/auth", target: "/project/{ref}/auth/users", permanent: true},
		{source: "/project/{ref}/auth/advanced", target: "/project/{ref}/auth/performance", permanent: true},
		{source: "/project/{ref}/database", target: "/project/{ref}/database/tables", permanent: true},
		{source: "/project/{ref}/database/graphiql", target: "/project/{ref}/api/graphiql", permanent: true},
		{source: "/project/{ref}/storage", target: "/project/{ref}/storage/files", permanent: true},
		{source: "/project/{ref}/storage/buckets", target: "/project/{ref}/storage/files", permanent: true},
		{source: "/project/{ref}/storage/policies", target: "/project/{ref}/storage/files/policies", permanent: true},
		{source: "/project/{ref}/storage/buckets/{bucketId}", target: "/project/{ref}/storage/files/buckets/{bucketId}", permanent: true},
		{source: "/project/{ref}/settings/api-keys/new", target: "/project/{ref}/settings/api-keys", permanent: true},
		{source: "/project/{ref}/settings/storage", target: "/project/{ref}/storage/files/settings", permanent: true},
		{source: "/project/{ref}/storage/settings", target: "/project/{ref}/storage/files/settings", permanent: true},
		{source: "/project/{ref}/settings/database", target: "/project/{ref}/database/settings", permanent: true},
		{source: "/project/{ref}/settings", target: "/project/{ref}/settings/general", permanent: true},
		{source: "/project/{ref}/auth/settings", target: "/project/{ref}/auth/users", permanent: true},
		{source: "/project/{ref}/settings/jwt/signing-keys", target: "/project/{ref}/settings/jwt", permanent: true},
		{source: "/project/{ref}/database/api-logs", target: "/project/{ref}/logs/edge-logs", permanent: true},
		{source: "/project/{ref}/database/postgres-logs", target: "/project/{ref}/logs/postgres-logs", permanent: true},
		{source: "/project/{ref}/database/postgrest-logs", target: "/project/{ref}/logs/postgrest-logs", permanent: true},
		{source: "/project/{ref}/database/pgbouncer-logs", target: "/project/{ref}/logs/pooler-logs", permanent: true},
		{source: "/project/{ref}/logs/pgbouncer-logs", target: "/project/{ref}/logs/pooler-logs", permanent: true},
		{source: "/project/{ref}/database/realtime-logs", target: "/project/{ref}/logs/realtime-logs", permanent: true},
		{source: "/project/{ref}/storage/logs", target: "/project/{ref}/logs/storage-logs", permanent: true},
		{source: "/project/{ref}/auth/logs", target: "/project/{ref}/logs/auth-logs", permanent: true},
		{source: "/project/{ref}/logs-explorer", target: "/project/{ref}/logs/explorer", permanent: true},
		{source: "/project/{ref}/sql/templates", target: "/project/{ref}/sql", permanent: true},
		{source: "/org/{slug}/settings", target: "/org/{slug}/general", permanent: true},
		{source: "/project/{ref}/settings/billing/update", target: "/org/_/billing", permanent: true},
		{source: "/project/{ref}/settings/billing/update/free", target: "/org/_/billing", permanent: true},
		{source: "/project/{ref}/settings/billing/update/pro", target: "/org/_/billing", permanent: true},
		{source: "/project/{ref}/settings/billing/update/team", target: "/org/_/billing", permanent: true},
		{source: "/project/{ref}/settings/billing/update/enterprise", target: "/org/_/billing", permanent: true},
		{source: "/project/{ref}/reports/linter", target: "/project/{ref}/database/linter", permanent: true},
		{source: "/project/{ref}/reports", target: "/project/{ref}/observability", permanent: true},
		{source: "/project/{ref}/reports/{path:.*}", target: "/project/{ref}/observability/{path}", permanent: true},
		{source: "/project/{ref}/query-performance", target: "/project/{ref}/observability/query-performance", permanent: true},
		{source: "/project/{ref}/advisors/query-performance", target: "/project/{ref}/observability/query-performance", permanent: true},
		{source: "/project/{ref}/database/query-performance", target: "/project/{ref}/observability/query-performance", permanent: true},
		{source: "/project/{ref}/auth/column-privileges", target: "/project/{ref}/database/column-privileges", permanent: true},
		{source: "/project/{ref}/database/linter", target: "/project/{ref}/database/security-advisor", permanent: true},
		{source: "/project/{ref}/database/security-advisor", target: "/project/{ref}/advisors/security", permanent: true},
		{source: "/project/{ref}/database/performance-advisor", target: "/project/{ref}/advisors/performance", permanent: true},
		{source: "/project/{ref}/database/webhooks", target: "/project/{ref}/integrations/webhooks/overview", permanent: true},
		{source: "/project/{ref}/database/wrappers", target: "/project/{ref}/integrations?category=wrapper", permanent: true},
		{source: "/project/{ref}/database/cron-jobs", target: "/project/{ref}/integrations/cron", permanent: true},
		{source: "/project/{ref}/api/graphiql", target: "/project/{ref}/integrations/graphiql", permanent: true},
		{source: "/project/{ref}/settings/vault/secrets", target: "/project/{ref}/integrations/vault/secrets", permanent: true},
		{source: "/project/{ref}/settings/vault/keys", target: "/project/{ref}/integrations/vault/keys", permanent: true},
		{source: "/project/{ref}/integrations/cron-jobs", target: "/project/{ref}/integrations/cron", permanent: true},
		{source: "/project/{ref}/settings/warehouse", target: "/project/{ref}/settings/general", permanent: true},
		{source: "/project/{ref}/settings/functions", target: "/project/{ref}/functions/secrets", permanent: true},
		{source: "/org/{slug}/invoices", target: "/org/{slug}/billing#invoices", permanent: true},
		{source: "/projects", target: "/organizations", permanent: false},
		{source: "/project/{ref}/settings/auth", target: "/project/{ref}/auth/providers", permanent: true},
	}

	for _, rule := range common {
		r.Get(rule.source, redirectHandler(rule.target, rule.permanent))
	}

	r.Get("/project/{ref}/settings/billing/subscription", func(w http.ResponseWriter, req *http.Request) {
		panel := req.URL.Query().Get("panel")
		switch panel {
		case "subscriptionPlan":
			http.Redirect(w, req, "/org/_/billing?panel=subscriptionPlan", http.StatusPermanentRedirect)
		case "pitr":
			http.Redirect(w, req, "/project/"+chi.URLParam(req, "ref")+"/settings/addons?panel=pitr", http.StatusPermanentRedirect)
		case "computeInstance":
			http.Redirect(w, req, "/project/"+chi.URLParam(req, "ref")+"/settings/compute-and-disk", http.StatusPermanentRedirect)
		case "customDomain":
			http.Redirect(w, req, "/project/"+chi.URLParam(req, "ref")+"/settings/addons?panel=customDomain", http.StatusPermanentRedirect)
		default:
			http.Redirect(w, req, "/org/_/billing", http.StatusPermanentRedirect)
		}
	})
}

func redirectHandler(target string, permanent bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		finalTarget := interpolateTarget(target, r)
		status := http.StatusTemporaryRedirect
		if permanent {
			status = http.StatusPermanentRedirect
		}
		http.Redirect(w, r, finalTarget, status)
	}
}

func interpolateTarget(target string, r *http.Request) string {
	params := chi.RouteContext(r.Context()).URLParams
	for i, key := range params.Keys {
		target = strings.ReplaceAll(target, "{"+key+"}", params.Values[i])
	}
	return target
}
