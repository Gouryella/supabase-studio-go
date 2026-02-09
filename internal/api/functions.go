package api

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type functionArtifact struct {
	Slug          string
	EntrypointURL string
	CreatedAt     int64
	UpdatedAt     int64
}

func (api *API) listFunctions() ([]map[string]any, error) {
	artifacts, err := api.loadFunctionArtifacts()
	if err != nil {
		return nil, err
	}
	if len(artifacts) == 0 {
		return []map[string]any{}, nil
	}

	var response []map[string]any
	for _, artifact := range artifacts {
		response = append(response, map[string]any{
			"id":              uuid.NewString(),
			"slug":            artifact.Slug,
			"version":         1,
			"name":            artifact.Slug,
			"status":          "ACTIVE",
			"entrypoint_path": artifact.EntrypointURL,
			"created_at":      artifact.CreatedAt,
			"updated_at":      artifact.UpdatedAt,
		})
	}
	return response, nil
}

func (api *API) getFunctionBySlug(slug string) (map[string]any, error) {
	artifacts, err := api.loadFunctionArtifacts()
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		if artifact.Slug == slug {
			return map[string]any{
				"id":              uuid.NewString(),
				"slug":            artifact.Slug,
				"version":         1,
				"name":            artifact.Slug,
				"status":          "ACTIVE",
				"entrypoint_path": artifact.EntrypointURL,
				"created_at":      artifact.CreatedAt,
				"updated_at":      artifact.UpdatedAt,
			}, nil
		}
	}
	return nil, errors.New("not found")
}

func (api *API) loadFunctionArtifacts() ([]functionArtifact, error) {
	for _, folder := range api.functionFolderCandidates() {
		artifacts, found, err := loadFunctionArtifactsFromFolder(folder)
		if err != nil {
			return nil, err
		}
		if found {
			return artifacts, nil
		}
	}
	return []functionArtifact{}, nil
}

func (api *API) functionFolderCandidates() []string {
	var folders []string
	if configured := strings.TrimSpace(api.cfg.EdgeFunctionsFolder); configured != "" {
		folders = append(folders, configured)
	}
	folders = append(folders, filepath.Join("supabase", "functions"))
	return folders
}

func loadFunctionArtifactsFromFolder(folder string) ([]functionArtifact, bool, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}

	var artifacts []functionArtifact
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "main" {
			continue
		}
		artifact, err := parseFunctionFolder(filepath.Join(folder, entry.Name()))
		if err != nil || artifact.Slug == "" {
			continue
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, true, nil
}

func parseFunctionFolder(folder string) (functionArtifact, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return functionArtifact{}, err
	}
	var entrypoint string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "index") {
			entrypoint = filepath.Join(folder, entry.Name())
			break
		}
	}
	if entrypoint == "" {
		return functionArtifact{}, nil
	}
	info, err := os.Stat(entrypoint)
	if err != nil {
		return functionArtifact{}, err
	}
	entryURL := &url.URL{Scheme: "file", Path: filepath.ToSlash(entrypoint)}
	createdAt := info.ModTime().UnixMilli()
	updatedAt := info.ModTime().UnixMilli()
	return functionArtifact{
		Slug:          filepath.Base(folder),
		EntrypointURL: entryURL.String(),
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}
