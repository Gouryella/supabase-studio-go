package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Gouryella/supabase-studio-go/internal/config"
)

func TestListFunctionsReturnsEmptyWhenFolderMissing(t *testing.T) {
	tmpDir := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to switch working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousWD)
	})

	api := &API{
		cfg: config.Config{
			EdgeFunctionsFolder: filepath.Join(tmpDir, "does-not-exist"),
		},
	}

	functions, err := api.listFunctions()
	if err != nil {
		t.Fatalf("expected missing function folder to be non-fatal, got: %v", err)
	}
	if len(functions) != 0 {
		t.Fatalf("expected empty functions list, got %d entries", len(functions))
	}
}

func TestListFunctionsFallsBackToSupabaseFunctionsFolder(t *testing.T) {
	tmpDir := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to switch working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousWD)
	})

	functionDir := filepath.Join(tmpDir, "supabase", "functions", "hello")
	if err := os.MkdirAll(functionDir, 0o755); err != nil {
		t.Fatalf("failed to create function directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(functionDir, "index.ts"), []byte("export default 1"), 0o644); err != nil {
		t.Fatalf("failed to write function entrypoint: %v", err)
	}

	api := &API{}

	functions, err := api.listFunctions()
	if err != nil {
		t.Fatalf("expected fallback function directory to be readable, got: %v", err)
	}
	if len(functions) != 1 {
		t.Fatalf("expected one function, got %d", len(functions))
	}
	if got := functions[0]["slug"]; got != "hello" {
		t.Fatalf("expected slug 'hello', got %v", got)
	}
}
