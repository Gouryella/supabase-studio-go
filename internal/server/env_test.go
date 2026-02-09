package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Gouryella/supabase-studio-go/internal/config"
)

func TestEnvHandlerFiltersSensitiveKeys(t *testing.T) {
	t.Setenv("NEXT_PUBLIC_SUPABASE_URL", "https://example.supabase.co")
	t.Setenv("NEXT_PUBLIC_SUPABASE_ANON_KEY", "anon")
	t.Setenv("NEXT_PUBLIC_SUPABASE_SERVICE_ROLE_KEY", "service-role-secret")
	t.Setenv("NEXT_PUBLIC_SUPPORT_SECRET", "should-not-leak")
	t.Setenv("NEXT_PUBLIC_API_TOKEN", "should-not-leak")
	t.Setenv("APP_INTERNAL_ONLY", "not-public")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/env.js", nil)

	envHandler(config.Config{IsPlatform: false}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	body := strings.TrimSpace(rec.Body.String())
	body = strings.TrimPrefix(body, "window.__env = ")
	body = strings.TrimSuffix(body, ";")

	var env map[string]string
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("failed to parse env payload: %v", err)
	}

	// Verify allowed keys are exposed
	assertPresent := func(key string) {
		if env[key] == "" {
			t.Errorf("expected %s to be exposed", key)
		}
	}
	assertPresent("NEXT_PUBLIC_SUPABASE_URL")
	assertPresent("NEXT_PUBLIC_SUPABASE_ANON_KEY")

	// Verify sensitive keys are filtered
	assertAbsent := func(key string) {
		if _, exists := env[key]; exists {
			t.Errorf("expected %s to be filtered", key)
		}
	}
	assertAbsent("NEXT_PUBLIC_SUPABASE_SERVICE_ROLE_KEY")
	assertAbsent("NEXT_PUBLIC_SUPPORT_SECRET")
	assertAbsent("NEXT_PUBLIC_API_TOKEN")
	assertAbsent("APP_INTERNAL_ONLY")

	// Verify default value
	if env["NEXT_PUBLIC_IS_PLATFORM"] != "false" {
		t.Errorf("expected NEXT_PUBLIC_IS_PLATFORM=false, got %q", env["NEXT_PUBLIC_IS_PLATFORM"])
	}
}
