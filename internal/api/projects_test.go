package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gouryella/supabase-studio-go/internal/config"
)

func testAPIHandler() http.Handler {
	return NewRouter(config.Config{
		DefaultProjectName:       "Default Project",
		DefaultProjectDiskSizeGB: 8,
		SupabasePublicURL:        "http://localhost:8000",
		StateFilePath:            "",
	})
}

func TestProjectUpdateRejectsInvalidName(t *testing.T) {
	handler := testAPIHandler()

	req := httptest.NewRequest(http.MethodPatch, "/platform/projects/default", strings.NewReader(`{"name":"ab"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestProjectUpdatePersistsInRuntimeResponses(t *testing.T) {
	handler := testAPIHandler()

	updateReq := httptest.NewRequest(http.MethodPatch, "/platform/projects/default", strings.NewReader(`{"name":"My Local Project"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", updateRec.Code)
	}

	assertProjectName := func(path string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 for %s, got %d", path, rec.Code)
		}

		var payload struct{ Name string }
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode %s response: %v", path, err)
		}

		if payload.Name != "My Local Project" {
			t.Fatalf("expected name to be updated in %s, got %q", path, payload.Name)
		}
	}

	assertProjectName("/platform/projects/default")
	assertProjectName("/platform/projects/default/settings")
}

func TestProjectUpdatePersistsAcrossRouterRestart(t *testing.T) {
	cfg := config.Config{
		DefaultProjectName:       "Default Project",
		DefaultProjectDiskSizeGB: 8,
		SupabasePublicURL:        "http://localhost:8000",
		StateFilePath:            filepath.Join(t.TempDir(), "supabase-studio-go-state.json"),
	}

	handler := NewRouter(cfg)

	updateReq := httptest.NewRequest(http.MethodPatch, "/platform/projects/default", strings.NewReader(`{"name":"Persistent Name"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", updateRec.Code)
	}

	// Simulate process restart by constructing a new router with the same state file.
	restartedHandler := NewRouter(cfg)
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/platform/projects/default", nil)
	restartedHandler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", getRec.Code)
	}

	var payload struct{ Name string }
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if payload.Name != "Persistent Name" {
		t.Fatalf("expected persisted name after restart, got %q", payload.Name)
	}
}

func TestProjectResizePersistsInRuntimeResponses(t *testing.T) {
	handler := testAPIHandler()

	resizeReq := httptest.NewRequest(http.MethodPost, "/platform/projects/default/resize", strings.NewReader(`{"volume_size_gb":16}`))
	resizeReq.Header.Set("Content-Type", "application/json")
	resizeRec := httptest.NewRecorder()
	handler.ServeHTTP(resizeRec, resizeReq)

	if resizeRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resizeRec.Code)
	}

	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/platform/projects/default", nil)
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", getRec.Code)
	}

	var payload struct {
		VolumeSizeGB int `json:"volumeSizeGb"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if payload.VolumeSizeGB != 16 {
		t.Fatalf("expected runtime disk size to be 16GB, got %dGB", payload.VolumeSizeGB)
	}
}

func TestProjectResizePersistsAcrossRouterRestart(t *testing.T) {
	cfg := config.Config{
		DefaultProjectName:       "Default Project",
		DefaultProjectDiskSizeGB: 8,
		SupabasePublicURL:        "http://localhost:8000",
		StateFilePath:            filepath.Join(t.TempDir(), "supabase-studio-go-state.json"),
	}

	handler := NewRouter(cfg)

	resizeReq := httptest.NewRequest(http.MethodPost, "/platform/projects/default/resize", strings.NewReader(`{"volume_size_gb":24}`))
	resizeReq.Header.Set("Content-Type", "application/json")
	resizeRec := httptest.NewRecorder()
	handler.ServeHTTP(resizeRec, resizeReq)

	if resizeRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resizeRec.Code)
	}

	restartedHandler := NewRouter(cfg)
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/platform/projects/default", nil)
	restartedHandler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", getRec.Code)
	}

	var payload struct {
		VolumeSizeGB int `json:"volumeSizeGb"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if payload.VolumeSizeGB != 24 {
		t.Fatalf("expected persisted disk size after restart to be 24GB, got %dGB", payload.VolumeSizeGB)
	}
}
