package api

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type persistedState struct {
	ProjectName       string `json:"project_name"`
	ProjectDiskSizeGB int    `json:"project_disk_size_gb"`
}

func (api *API) loadStateFromDisk() error {
	if strings.TrimSpace(api.stateFilePath) == "" {
		return nil
	}

	bytes, err := os.ReadFile(api.stateFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var state persistedState
	if err := json.Unmarshal(bytes, &state); err != nil {
		return err
	}

	name := strings.TrimSpace(state.ProjectName)
	if name == "" {
		name = api.cfg.DefaultProjectName
	}
	api.setProjectName(name)

	if state.ProjectDiskSizeGB > 0 {
		api.setProjectDiskSize(state.ProjectDiskSizeGB)
	}

	return nil
}

func (api *API) persistStateToDisk() error {
	if strings.TrimSpace(api.stateFilePath) == "" {
		return nil
	}

	dir := filepath.Dir(api.stateFilePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	payload := persistedState{
		ProjectName:       api.getProjectName(),
		ProjectDiskSizeGB: api.getProjectDiskSize(),
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	tmpPath := api.stateFilePath + ".tmp"
	if err := os.WriteFile(tmpPath, bytes, 0o644); err != nil {
		return err
	}

	return os.Rename(tmpPath, api.stateFilePath)
}

func (api *API) updateProjectName(name string) error {
	previous := api.getProjectName()
	api.setProjectName(name)

	if err := api.persistStateToDisk(); err != nil {
		api.setProjectName(previous)
		return err
	}

	return nil
}

func (api *API) updateProjectDiskSize(size int) error {
	previous := api.getProjectDiskSize()
	api.setProjectDiskSize(size)

	if err := api.persistStateToDisk(); err != nil {
		api.setProjectDiskSize(previous)
		return err
	}

	return nil
}
