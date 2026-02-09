package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Gouryella/supabase-studio-go/internal/config"
)

func TestEnsureManagedFoldersCreatesConfiguredDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	edgeDir := filepath.Join(tmpDir, "edge-functions")
	snippetsDir := filepath.Join(tmpDir, "snippets")

	api := &API{
		cfg: config.Config{
			EdgeFunctionsFolder: edgeDir,
			SnippetsFolder:      snippetsDir,
		},
	}

	if err := api.ensureManagedFolders(); err != nil {
		t.Fatalf("expected folders to be created, got: %v", err)
	}

	if info, err := os.Stat(edgeDir); err != nil || !info.IsDir() {
		t.Fatalf("expected edge functions directory to exist")
	}
	if info, err := os.Stat(snippetsDir); err != nil || !info.IsDir() {
		t.Fatalf("expected snippets directory to exist")
	}
}

func TestEnsureManagedFoldersSkipsEmptyValues(t *testing.T) {
	api := &API{}
	if err := api.ensureManagedFolders(); err != nil {
		t.Fatalf("expected empty folder config to be ignored, got: %v", err)
	}
}
