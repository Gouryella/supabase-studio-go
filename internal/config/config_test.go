package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadUsesServiceRoleFallbackKeys(t *testing.T) {
	t.Setenv("SUPABASE_SERVICE_KEY", "")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role-key")
	t.Setenv("SERVICE_ROLE_KEY", "legacy-role-key")

	cfg := Load()

	if cfg.SupabaseServiceKey != "service-role-key" {
		t.Fatalf("expected SUPABASE_SERVICE_ROLE_KEY fallback, got %q", cfg.SupabaseServiceKey)
	}
}

func TestLoadUsesLegacyServiceRoleKeyWhenOthersMissing(t *testing.T) {
	t.Setenv("SUPABASE_SERVICE_KEY", "")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "")
	t.Setenv("SERVICE_ROLE_KEY", "legacy-role-key")

	cfg := Load()

	if cfg.SupabaseServiceKey != "legacy-role-key" {
		t.Fatalf("expected SERVICE_ROLE_KEY fallback, got %q", cfg.SupabaseServiceKey)
	}
}

func TestLoadUsesServiceKeyFallbackWhenRoleKeysMissing(t *testing.T) {
	t.Setenv("SUPABASE_SERVICE_KEY", "")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "")
	t.Setenv("SERVICE_ROLE_KEY", "")
	t.Setenv("SERVICE_KEY", "service-key-fallback")

	cfg := Load()

	if cfg.SupabaseServiceKey != "service-key-fallback" {
		t.Fatalf("expected SERVICE_KEY fallback, got %q", cfg.SupabaseServiceKey)
	}
}

func TestLoadUsesExplicitStateFilePath(t *testing.T) {
	t.Setenv("SUPABASE_STUDIO_GO_STATE_FILE", "/tmp/custom-state.json")
	t.Setenv("STUDIO_GO_STATE_FILE", "/tmp/legacy-state.json")

	cfg := Load()

	if cfg.StateFilePath != "/tmp/custom-state.json" {
		t.Fatalf("expected explicit state file path, got %q", cfg.StateFilePath)
	}
}

func TestLoadUsesLegacyStateFileEnvFallback(t *testing.T) {
	t.Setenv("SUPABASE_STUDIO_GO_STATE_FILE", "")
	t.Setenv("STUDIO_GO_STATE_FILE", "/tmp/legacy-state.json")

	cfg := Load()

	if cfg.StateFilePath != "/tmp/legacy-state.json" {
		t.Fatalf("expected legacy state file path fallback, got %q", cfg.StateFilePath)
	}
}

func TestLoadUsesAbsoluteDefaultStateFilePath(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	t.Setenv("SUPABASE_STUDIO_GO_STATE_FILE", "")
	t.Setenv("STUDIO_GO_STATE_FILE", "")
	t.Setenv("HOME", t.TempDir())

	cfg := Load()

	if !filepath.IsAbs(cfg.StateFilePath) {
		t.Fatalf("expected absolute state file path, got %q", cfg.StateFilePath)
	}

	if !strings.HasSuffix(cfg.StateFilePath, filepath.Join("supabase-studio-go", "state.json")) {
		t.Fatalf("expected state file path suffix supabase-studio-go/state.json, got %q", cfg.StateFilePath)
	}
}

func TestLoadMigratesLegacyStateFileWhenPresent(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	t.Setenv("HOME", t.TempDir())

	expectedPath := expectedDefaultStateFilePath(t)
	legacyPath := filepath.Join(".supabase-studio-go", "state.json")
	legacyContent := []byte(`{"project_name":"Legacy"}`)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("failed to create legacy state directory: %v", err)
	}
	if err := os.WriteFile(legacyPath, legacyContent, 0o644); err != nil {
		t.Fatalf("failed to write legacy state file: %v", err)
	}

	t.Setenv("SUPABASE_STUDIO_GO_STATE_FILE", "")
	t.Setenv("STUDIO_GO_STATE_FILE", "")

	cfg := Load()

	if cfg.StateFilePath != expectedPath {
		t.Fatalf("expected default state file path %q, got %q", expectedPath, cfg.StateFilePath)
	}

	migratedContent, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("expected migrated state file at %q: %v", expectedPath, err)
	}
	if string(migratedContent) != string(legacyContent) {
		t.Fatalf("expected migrated content %q, got %q", string(legacyContent), string(migratedContent))
	}
}

func TestLoadDoesNotOverwriteDefaultStateFileWithLegacyFile(t *testing.T) {
	restore := chdirTemp(t)
	defer restore()

	t.Setenv("HOME", t.TempDir())

	expectedPath := expectedDefaultStateFilePath(t)
	defaultContent := []byte(`{"project_name":"Current"}`)
	if err := os.MkdirAll(filepath.Dir(expectedPath), 0o755); err != nil {
		t.Fatalf("failed to create default state directory: %v", err)
	}
	if err := os.WriteFile(expectedPath, defaultContent, 0o644); err != nil {
		t.Fatalf("failed to write default state file: %v", err)
	}

	legacyPath := filepath.Join(".supabase-studio-go", "state.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("failed to create legacy state directory: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"project_name":"Legacy"}`), 0o644); err != nil {
		t.Fatalf("failed to write legacy state file: %v", err)
	}

	t.Setenv("SUPABASE_STUDIO_GO_STATE_FILE", "")
	t.Setenv("STUDIO_GO_STATE_FILE", "")

	cfg := Load()

	if cfg.StateFilePath != expectedPath {
		t.Fatalf("expected default state file path %q, got %q", expectedPath, cfg.StateFilePath)
	}

	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read default state file: %v", err)
	}
	if string(content) != string(defaultContent) {
		t.Fatalf("expected default state content %q, got %q", string(defaultContent), string(content))
	}
}

func expectedDefaultStateFilePath(t *testing.T) string {
	t.Helper()

	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "supabase-studio-go", "state.json")
	}

	return filepath.Join(os.TempDir(), "supabase-studio-go", "state.json")
}

func chdirTemp(t *testing.T) func() {
	t.Helper()

	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}

	return func() {
		if err := os.Chdir(originalCwd); err != nil {
			t.Fatalf("failed to restore cwd: %v", err)
		}
	}
}
