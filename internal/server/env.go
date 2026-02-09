package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/Gouryella/supabase-studio-go/internal/config"
)

var allowedPublicEnvKeys = map[string]struct{}{
	"NEXT_PUBLIC_API_URL":                          {},
	"NEXT_PUBLIC_AUTH_DEBUG_KEY":                   {},
	"NEXT_PUBLIC_AUTH_DEBUG_PERSISTED_KEY":         {},
	"NEXT_PUBLIC_AUTH_DETECT_SESSION_IN_URL":       {},
	"NEXT_PUBLIC_AUTH_NAVIGATOR_LOCK_KEY":          {},
	"NEXT_PUBLIC_BASE_PATH":                        {},
	"NEXT_PUBLIC_CONFIGCAT_PROXY_URL":              {},
	"NEXT_PUBLIC_CONFIGCAT_SDK_KEY":                {},
	"NEXT_PUBLIC_CONTENT_API_URL":                  {},
	"NEXT_PUBLIC_DISABLED_FEATURES":                {},
	"NEXT_PUBLIC_DOCS_URL":                         {},
	"NEXT_PUBLIC_ENVIRONMENT":                      {},
	"NEXT_PUBLIC_GITHUB_INTEGRATION_APP_NAME":      {},
	"NEXT_PUBLIC_GITHUB_INTEGRATION_CLIENT_ID":     {},
	"NEXT_PUBLIC_GOOGLE_MAPS_KEY":                  {},
	"NEXT_PUBLIC_GOOGLE_TAG_MANAGER_ID":            {},
	"NEXT_PUBLIC_IS_NIMBUS":                        {},
	"NEXT_PUBLIC_IS_PLATFORM":                      {},
	"NEXT_PUBLIC_MCP_URL":                          {},
	"NEXT_PUBLIC_NODE_ENV":                         {},
	"NEXT_PUBLIC_ONGOING_INCIDENT":                 {},
	"NEXT_PUBLIC_POSTHOG_HOST":                     {},
	"NEXT_PUBLIC_POSTHOG_KEY":                      {},
	"NEXT_PUBLIC_POSTHOG_UI_HOST":                  {},
	"NEXT_PUBLIC_SENTRY_DSN":                       {},
	"NEXT_PUBLIC_SENTRY_ENVIRONMENT":               {},
	"NEXT_PUBLIC_SITE_URL":                         {},
	"NEXT_PUBLIC_STORAGE_KEY":                      {},
	"NEXT_PUBLIC_STRIPE_PUBLIC_KEY":                {},
	"NEXT_PUBLIC_SUPABASE_ANON_KEY":                {},
	"NEXT_PUBLIC_SUPABASE_PUBLISHABLE_DEFAULT_KEY": {},
	"NEXT_PUBLIC_SUPABASE_URL":                     {},
	"NEXT_PUBLIC_SUPPORT_ANON_KEY":                 {},
	"NEXT_PUBLIC_SUPPORT_API_URL":                  {},
	"NEXT_PUBLIC_USERCENTRICS_RULESET_ID":          {},
	"NEXT_PUBLIC_VERCEL_BRANCH_URL":                {},
	"NEXT_PUBLIC_VERCEL_ENV":                       {},
}

func envHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env := make(map[string]string)

		for _, pair := range os.Environ() {
			key, value, found := strings.Cut(pair, "=")
			if !found || key == "" {
				continue
			}
			if _, allowed := allowedPublicEnvKeys[key]; allowed {
				env[key] = value
			}
		}

		if _, exists := env["NEXT_PUBLIC_IS_PLATFORM"]; !exists {
			env["NEXT_PUBLIC_IS_PLATFORM"] = "false"
			if cfg.IsPlatform {
				env["NEXT_PUBLIC_IS_PLATFORM"] = "true"
			}
		}

		payload, _ := json.Marshal(env)
		w.Header().Set("Content-Type", "application/javascript")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("window.__env = "))
		w.Write(payload)
		w.Write([]byte(";"))
	}
}
