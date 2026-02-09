package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Gouryella/supabase-studio-go/internal/config"
)

func TestStorageBucketCreateSendsIDAndNameLikeOfficialStudio(t *testing.T) {
	requestCount := 0
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/v1/bucket" {
			t.Fatalf("unexpected downstream path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected downstream method: %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		if payload["id"] != "avatars" {
			t.Fatalf("expected request payload id=avatars, got %#v", payload)
		}
		if payload["name"] != "avatars" {
			t.Fatalf("expected request payload name=avatars, got %#v", payload)
		}
		if payload["public"] != false {
			t.Fatalf("expected request payload public=false, got %#v", payload)
		}

		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"avatars","name":"avatars","public":false}`))
	}))
	defer storage.Close()

	handler := NewRouter(config.Config{
		DefaultProjectName:       "Default Project",
		DefaultProjectDiskSizeGB: 8,
		SupabaseURL:              storage.URL,
		SupabaseServiceKey:       "service-role-key",
		StateFilePath:            "",
	})

	req := httptest.NewRequest(http.MethodPost, "/platform/storage/default/buckets", strings.NewReader(`{"id":"avatars","public":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if requestCount != 1 {
		t.Fatalf("expected one downstream request, got %d", requestCount)
	}
}

func TestStorageBucketUpdateUsesPUTAndSendsIDAndNameLikeOfficialStudio(t *testing.T) {
	requestCount := 0
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/v1/bucket/avatars" {
			t.Fatalf("unexpected downstream path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected downstream method: %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		if payload["id"] != "avatars" {
			t.Fatalf("expected request payload id=avatars, got %#v", payload)
		}
		if payload["name"] != "avatars" {
			t.Fatalf("expected request payload name=avatars, got %#v", payload)
		}
		if payload["public"] != true {
			t.Fatalf("expected request payload public=true, got %#v", payload)
		}
		if payload["file_size_limit"] != float64(1024) {
			t.Fatalf("expected request payload file_size_limit=1024, got %#v", payload)
		}

		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer storage.Close()

	handler := NewRouter(config.Config{
		DefaultProjectName:       "Default Project",
		DefaultProjectDiskSizeGB: 8,
		SupabaseURL:              storage.URL,
		SupabaseServiceKey:       "service-role-key",
		StateFilePath:            "",
	})

	req := httptest.NewRequest(http.MethodPatch, "/platform/storage/default/buckets/avatars", strings.NewReader(`{"public":true,"file_size_limit":1024}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if requestCount != 1 {
		t.Fatalf("expected one downstream request, got %d", requestCount)
	}
}

func TestStorageObjectsListSendsPrefixAndOfficialDefaults(t *testing.T) {
	requestCount := 0
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/v1/object/list/avatars" {
			t.Fatalf("unexpected downstream path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected downstream method: %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}

		if prefix, ok := payload["prefix"].(string); !ok || prefix != "" {
			t.Fatalf("expected request payload prefix=\"\", got %#v", payload["prefix"])
		}
		if payload["limit"] != float64(100) {
			t.Fatalf("expected request payload limit=100, got %#v", payload["limit"])
		}
		if payload["offset"] != float64(0) {
			t.Fatalf("expected request payload offset=0, got %#v", payload["offset"])
		}
		sortBy, ok := payload["sortBy"].(map[string]any)
		if !ok {
			t.Fatalf("expected request payload sortBy object, got %#v", payload["sortBy"])
		}
		if sortBy["column"] != "name" || sortBy["order"] != "asc" {
			t.Fatalf("expected request payload sortBy={column:name,order:asc}, got %#v", sortBy)
		}

		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer storage.Close()

	handler := NewRouter(config.Config{
		DefaultProjectName:       "Default Project",
		DefaultProjectDiskSizeGB: 8,
		SupabaseURL:              storage.URL,
		SupabaseServiceKey:       "service-role-key",
		StateFilePath:            "",
	})

	req := httptest.NewRequest(http.MethodPost, "/platform/storage/default/buckets/avatars/objects/list", strings.NewReader(`{"path":"","options":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if requestCount != 1 {
		t.Fatalf("expected one downstream request, got %d", requestCount)
	}
}

func TestStorageObjectsDeleteUsesOfficialRouteAndPayload(t *testing.T) {
	requestCount := 0
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/v1/object/avatars" {
			t.Fatalf("unexpected downstream path: %s", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected downstream method: %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}

		prefixes, ok := payload["prefixes"].([]any)
		if !ok {
			t.Fatalf("expected request payload prefixes array, got %#v", payload["prefixes"])
		}
		if len(prefixes) != 2 {
			t.Fatalf("expected request payload prefixes length=2, got %#v", prefixes)
		}
		if prefixes[0] != "folder/.emptyFolderPlaceholder" || prefixes[1] != "folder/a.txt" {
			t.Fatalf("unexpected request payload prefixes: %#v", prefixes)
		}

		if _, hasBucketID := payload["bucketId"]; hasBucketID {
			t.Fatalf("did not expect bucketId in payload, got %#v", payload)
		}

		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer storage.Close()

	handler := NewRouter(config.Config{
		DefaultProjectName:       "Default Project",
		DefaultProjectDiskSizeGB: 8,
		SupabaseURL:              storage.URL,
		SupabaseServiceKey:       "service-role-key",
		StateFilePath:            "",
	})

	req := httptest.NewRequest(
		http.MethodDelete,
		"/platform/storage/default/buckets/avatars/objects",
		strings.NewReader(`{"paths":["folder/.emptyFolderPlaceholder","folder/a.txt"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if requestCount != 1 {
		t.Fatalf("expected one downstream request, got %d", requestCount)
	}
}

func TestStorageObjectsSignUsesSingleObjectRouteAndNormalizesSignedURL(t *testing.T) {
	requestCount := 0
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/v1/object/sign/avatars/folder/cat.png" {
			t.Fatalf("unexpected downstream path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected downstream method: %s", r.Method)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		if payload["expiresIn"] != float64(3600) {
			t.Fatalf("expected request payload expiresIn=3600, got %#v", payload["expiresIn"])
		}
		if _, hasPath := payload["path"]; hasPath {
			t.Fatalf("did not expect request payload path, got %#v", payload)
		}
		if _, hasPaths := payload["paths"]; hasPaths {
			t.Fatalf("did not expect request payload paths, got %#v", payload)
		}

		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"signedURL":"/object/sign/avatars/folder/cat.png?token=abc"}`))
	}))
	defer storage.Close()

	handler := NewRouter(config.Config{
		DefaultProjectName:       "Default Project",
		DefaultProjectDiskSizeGB: 8,
		SupabaseURL:              storage.URL,
		SupabasePublicURL:        "https://database-eu.fichuo.de",
		SupabaseServiceKey:       "service-role-key",
		StateFilePath:            "",
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/platform/storage/default/buckets/avatars/objects/sign",
		strings.NewReader(`{"path":"folder/cat.png","expiresIn":3600}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	signedURL, _ := response["signedUrl"].(string)
	if signedURL != "https://database-eu.fichuo.de/storage/v1/object/sign/avatars/folder/cat.png?token=abc" {
		t.Fatalf("unexpected response signedUrl=%q", signedURL)
	}
	if _, hasLegacy := response["signedURL"]; hasLegacy {
		t.Fatalf("did not expect legacy signedURL key in response: %#v", response)
	}

	if requestCount != 1 {
		t.Fatalf("expected one downstream request, got %d", requestCount)
	}
}

func TestRewriteStorageSignedURLPrefixesStorageRoute(t *testing.T) {
	got := rewriteStorageSignedURL("/object/sign/avatars/file.png?token=abc", "https://example.com")
	want := "https://example.com/storage/v1/object/sign/avatars/file.png?token=abc"
	if got != want {
		t.Fatalf("unexpected rewritten url=%q want=%q", got, want)
	}
}

func TestRewriteStorageSignedURLDoesNotDoublePrefix(t *testing.T) {
	got := rewriteStorageSignedURL(
		"https://example.com/storage/v1/object/sign/avatars/file.png?token=abc",
		"https://example.com",
	)
	want := "https://example.com/storage/v1/object/sign/avatars/file.png?token=abc"
	if got != want {
		t.Fatalf("unexpected rewritten url=%q want=%q", got, want)
	}
}
