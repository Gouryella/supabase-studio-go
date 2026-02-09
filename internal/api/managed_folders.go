package api

import (
	"fmt"
	"os"
	"strings"
)

func (api *API) ensureManagedFolders() error {
	folders := []string{
		strings.TrimSpace(api.cfg.EdgeFunctionsFolder),
		strings.TrimSpace(api.cfg.SnippetsFolder),
	}

	for _, folder := range folders {
		if folder == "" {
			continue
		}
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return fmt.Errorf("create managed folder %q: %w", folder, err)
		}
	}

	return nil
}
