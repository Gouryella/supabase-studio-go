package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Gouryella/supabase-studio-go/internal/config"
)

func TestAuthHeadersIncludeAPIKeyAndBearerToken(t *testing.T) {
	api := &API{
		cfg: config.Config{
			SupabaseServiceKey: "service-role",
		},
	}

	headers := api.authHeaders()

	if got := headers.Get("apikey"); got != "service-role" {
		t.Fatalf("expected apikey header, got %q", got)
	}
	if got := headers.Get("Authorization"); got != "Bearer service-role" {
		t.Fatalf("expected Authorization bearer header, got %q", got)
	}
}

func TestAuthHeadersOmitAuthWhenServiceKeyMissing(t *testing.T) {
	api := &API{}

	headers := api.authHeaders()

	if got := headers.Get("apikey"); got != "" {
		t.Fatalf("expected empty apikey header when key missing, got %q", got)
	}
	if got := headers.Get("Authorization"); got != "" {
		t.Fatalf("expected empty Authorization header when key missing, got %q", got)
	}
}

func TestAuthProxyRetriesWithAPIKeyQueryWhenHeaderNotDetected(t *testing.T) {
	var calls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"No API key found in request"}`))
			return
		}
		if got := r.URL.Query().Get("apikey"); got != "service-role" {
			t.Fatalf("expected apikey query on retry, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":"u_1"}}`))
	}))
	defer srv.Close()

	api := &API{
		cfg: config.Config{
			SupabaseURL:        srv.URL,
			SupabaseServiceKey: "service-role",
		},
		client: srv.Client(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/platform/auth/default/users", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()

	api.authProxy(rr, req, http.MethodPost, "/admin/users", []byte(`{}`))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rr.Code, rr.Body.String())
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
	if !strings.Contains(rr.Body.String(), `"id":"u_1"`) {
		t.Fatalf("expected user payload, got %s", rr.Body.String())
	}
}

func TestAuthProxyReturnsConfigErrorWhenServiceKeyMissing(t *testing.T) {
	api := &API{
		cfg: config.Config{
			SupabaseURL: "http://localhost:9999",
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/platform/auth/default/users", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()

	api.authProxy(rr, req, http.MethodPost, "/admin/users", []byte(`{}`))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d with body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Missing service key") {
		t.Fatalf("expected missing service key message, got %s", rr.Body.String())
	}
}
