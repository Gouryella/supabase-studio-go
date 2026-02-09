package api

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type snippetContent struct {
	SQL           string `json:"sql"`
	ContentID     string `json:"content_id"`
	SchemaVersion string `json:"schema_version"`
}

type snippetUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type snippet struct {
	ID          string         `json:"id"`
	InsertedAt  string         `json:"inserted_at"`
	UpdatedAt   string         `json:"updated_at"`
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Favorite    bool           `json:"favorite"`
	Content     snippetContent `json:"content"`
	Visibility  string         `json:"visibility"`
	ProjectID   int            `json:"project_id"`
	FolderID    *string        `json:"folder_id"`
	OwnerID     int            `json:"owner_id"`
	Owner       snippetUser    `json:"owner"`
	UpdatedBy   snippetUser    `json:"updated_by"`
}

type folder struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	OwnerID   int     `json:"owner_id"`
	ParentID  *string `json:"parent_id"`
	ProjectID int     `json:"project_id"`
}

type filesystemEntry struct {
	ID        string
	Name      string
	Type      string
	FolderID  *string
	Content   string
	CreatedAt time.Time
}

var (
	errSnippetNotFound             = errors.New("snippet not found")
	errSnippetAlreadyExists        = errors.New("snippet already exists")
	errSnippetExistsInTargetFolder = errors.New("snippet already exists in target folder")
	errFolderNotFound              = errors.New("folder not found")
	errFolderNameRequired          = errors.New("folder name is required")
	errFolderAlreadyExists         = errors.New("folder already exists")
	errLimitExceedsMax             = errors.New("limit cannot exceed 1000")
	errSnippetsFolderEnvNotSet     = errors.New("snippets management folder env var (SNIPPETS_MANAGEMENT_FOLDER) is not set; set it to use snippets properly")
)

func (api *API) snippetsDir() (string, error) {
	if api.cfg.SnippetsFolder == "" {
		return "", errSnippetsFolderEnvNotSet
	}
	if err := os.MkdirAll(api.cfg.SnippetsFolder, 0o755); err != nil {
		return "", err
	}
	return api.cfg.SnippetsFolder, nil
}

func (api *API) getFilesystemEntries() ([]filesystemEntry, error) {
	root, err := api.snippetsDir()
	if err != nil {
		return nil, err
	}

	entries := make([]filesystemEntry, 0)
	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if d.IsDir() {
			if len(parts) > 1 {
				return filepath.SkipDir
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			folderID := deterministicUUID([]string{d.Name()})
			entries = append(entries, filesystemEntry{
				ID:        folderID,
				Name:      d.Name(),
				Type:      "folder",
				FolderID:  nil,
				CreatedAt: info.ModTime(),
			})
			return nil
		}
		if d.Name() == ".DS_Store" || !strings.HasSuffix(d.Name(), ".sql") {
			return nil
		}
		var folderID *string
		var name string
		if len(parts) == 1 {
			name = strings.TrimSuffix(d.Name(), ".sql")
		} else {
			folderName := parts[0]
			id := deterministicUUID([]string{folderName})
			folderID = &id
			name = strings.TrimSuffix(parts[len(parts)-1], ".sql")
		}

		contentBytes, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}

		idInputs := []string{name + ".sql"}
		if folderID != nil {
			idInputs = []string{*folderID, name + ".sql"}
		}

		entries = append(entries, filesystemEntry{
			ID:        deterministicUUID(idInputs),
			Name:      name,
			Type:      "file",
			FolderID:  folderID,
			Content:   string(contentBytes),
			CreatedAt: info.ModTime(),
		})
		return nil
	}

	_ = filepath.WalkDir(root, walk)
	return entries, nil
}

func buildSnippet(name, content string, folderID *string, createdAt time.Time) snippet {
	idInputs := []string{name + ".sql"}
	if folderID != nil {
		idInputs = []string{*folderID, name + ".sql"}
	}
	return snippet{
		ID:          deterministicUUID(idInputs),
		InsertedAt:  createdAt.Format(time.RFC3339),
		UpdatedAt:   createdAt.Format(time.RFC3339),
		Type:        "sql",
		Name:        name,
		Description: "",
		Favorite:    false,
		Content: snippetContent{
			SQL:           content,
			ContentID:     uuid.NewString(),
			SchemaVersion: "1.0",
		},
		Visibility: "user",
		ProjectID:  1,
		FolderID:   folderID,
		OwnerID:    1,
		Owner:      snippetUser{ID: 1, Username: "johndoe"},
		UpdatedBy:  snippetUser{ID: 1, Username: "johndoe"},
	}
}

func (api *API) getSnippets(searchTerm string, limit int, cursor string, sortField string, sortOrder string, folderID *string) (string, []snippet, error) {
	entries, err := api.getFilesystemEntries()
	if err != nil {
		return "", nil, err
	}
	files := make([]filesystemEntry, 0)
	for _, entry := range entries {
		if entry.Type == "file" {
			files = append(files, entry)
		}
	}

	var filtered []filesystemEntry
	if searchTerm != "" {
		lower := strings.ToLower(searchTerm)
		for _, file := range files {
			if strings.Contains(strings.ToLower(file.Name), lower) {
				filtered = append(filtered, file)
			}
		}
	} else {
		for _, file := range files {
			if matchesFolder(file.FolderID, folderID) {
				filtered = append(filtered, file)
			}
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if sortField == "name" {
			return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
		}
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	if sortOrder == "desc" {
		for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
			filtered[i], filtered[j] = filtered[j], filtered[i]
		}
	}

	start := 0
	if cursor != "" {
		for idx, entry := range filtered {
			if entry.ID == cursor {
				start = idx + 1
				break
			}
		}
	}
	filtered = filtered[start:]

	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return "", nil, errLimitExceedsMax
	}

	nextCursor := ""
	if len(filtered) > limit {
		nextCursor = filtered[limit-1].ID
		filtered = filtered[:limit]
	}

	snippets := make([]snippet, 0, len(filtered))
	for _, entry := range filtered {
		snippets = append(snippets, buildSnippet(entry.Name, entry.Content, entry.FolderID, entry.CreatedAt))
	}
	return nextCursor, snippets, nil
}

func (api *API) saveSnippet(newSnippet snippet) (snippet, error) {
	entries, err := api.getFilesystemEntries()
	if err != nil {
		return snippet{}, err
	}

	for _, entry := range entries {
		if entry.ID == newSnippet.ID && entry.Type == "file" {
			return snippet{}, errSnippetAlreadyExists
		}
	}

	if newSnippet.FolderID != nil {
		found := false
		for _, entry := range entries {
			if entry.ID == *newSnippet.FolderID && entry.Type == "folder" {
				found = true
				break
			}
		}
		if !found {
			return snippet{}, errFolderNotFound
		}
	}

	name := sanitizeName(newSnippet.Name)
	content := newSnippet.Content.SQL
	root, err := api.snippetsDir()
	if err != nil {
		return snippet{}, err
	}

	folderPath := root
	if newSnippet.FolderID != nil {
		for _, entry := range entries {
			if entry.ID == *newSnippet.FolderID && entry.Type == "folder" {
				folderPath = filepath.Join(root, entry.Name)
				break
			}
		}
	}

	filePath := filepath.Join(folderPath, name+".sql")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return snippet{}, err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return snippet{}, err
	}
	return buildSnippet(name, content, newSnippet.FolderID, info.ModTime()), nil
}

func (api *API) deleteSnippet(id string) error {
	entries, err := api.getFilesystemEntries()
	if err != nil {
		return err
	}
	var target *filesystemEntry
	for _, entry := range entries {
		if entry.ID == id && entry.Type == "file" {
			target = &entry
			break
		}
	}
	if target == nil {
		return errSnippetNotFound
	}

	root, err := api.snippetsDir()
	if err != nil {
		return err
	}
	filename := target.Name + ".sql"
	paths := []string{root}
	if target.FolderID != nil {
		for _, entry := range entries {
			if entry.ID == *target.FolderID && entry.Type == "folder" {
				paths = append(paths, entry.Name)
			}
		}
	}
	paths = append(paths, filename)
	filePath := filepath.Join(paths...)
	if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (api *API) updateSnippet(id string, updates map[string]any) (snippet, error) {
	entries, err := api.getFilesystemEntries()
	if err != nil {
		return snippet{}, err
	}
	var found filesystemEntry
	foundOk := false
	for _, entry := range entries {
		if entry.ID == id && entry.Type == "file" {
			found = entry
			foundOk = true
			break
		}
	}
	if !foundOk {
		return snippet{}, errSnippetNotFound
	}

	name := found.Name
	if updatesName, ok := updates["name"].(string); ok && updatesName != "" {
		name = updatesName
	}

	folderID := found.FolderID
	if updatesFolder, ok := updates["folder_id"]; ok {
		if updatesFolder == nil {
			folderID = nil
		} else if idString, ok := updatesFolder.(string); ok {
			folderID = &idString
		}
	}

	newIDInputs := []string{name + ".sql"}
	if folderID != nil {
		newIDInputs = []string{*folderID, name + ".sql"}
	}
	newID := deterministicUUID(newIDInputs)

	for _, entry := range entries {
		if entry.ID == newID && entry.Type == "file" && entry.ID != found.ID {
			return snippet{}, errSnippetExistsInTargetFolder
		}
	}

	content := found.Content
	if updatesContent, ok := updates["content"].(map[string]any); ok {
		if sql, ok := updatesContent["sql"].(string); ok {
			content = sql
		}
	}

	if err := api.deleteSnippet(found.ID); err != nil {
		return snippet{}, err
	}

	updatedSnippet := snippet{
		ID:       newID,
		Name:     name,
		Content:  snippetContent{SQL: content, ContentID: uuid.NewString(), SchemaVersion: "1.0"},
		FolderID: folderID,
	}
	return api.saveSnippet(updatedSnippet)
}

func (api *API) getFolders(folderID *string) ([]folder, error) {
	entries, err := api.getFilesystemEntries()
	if err != nil {
		return nil, err
	}
	folders := make([]folder, 0)
	for _, entry := range entries {
		if entry.Type == "folder" && entry.FolderID == nil && folderID == nil {
			folders = append(folders, folder{
				ID:        entry.ID,
				Name:      entry.Name,
				OwnerID:   1,
				ParentID:  nil,
				ProjectID: 1,
			})
		}
	}
	return folders, nil
}

func (api *API) createFolder(name string) (folder, error) {
	root, err := api.snippetsDir()
	if err != nil {
		return folder{}, err
	}
	name = sanitizeName(name)
	if name == "" {
		return folder{}, errFolderNameRequired
	}

	entries, _ := api.getFilesystemEntries()
	for _, entry := range entries {
		if entry.Type == "folder" && entry.Name == name {
			return folder{}, errFolderAlreadyExists
		}
	}

	folderPath := filepath.Join(root, name)
	if err := os.MkdirAll(folderPath, 0o755); err != nil {
		return folder{}, err
	}

	return folder{
		ID:        deterministicUUID([]string{name}),
		Name:      name,
		OwnerID:   1,
		ParentID:  nil,
		ProjectID: 1,
	}, nil
}

func (api *API) deleteFolder(id string) error {
	entries, err := api.getFilesystemEntries()
	if err != nil {
		return err
	}
	var target *filesystemEntry
	for _, entry := range entries {
		if entry.Type == "folder" && entry.ID == id {
			target = &entry
			break
		}
	}
	if target == nil {
		return errFolderNotFound
	}
	root, err := api.snippetsDir()
	if err != nil {
		return err
	}
	folderPath := filepath.Join(root, target.Name)
	return os.RemoveAll(folderPath)
}

func (api *API) getSnippet(id string) (snippet, error) {
	entries, err := api.getFilesystemEntries()
	if err != nil {
		return snippet{}, err
	}
	for _, entry := range entries {
		if entry.Type == "file" && entry.ID == id {
			return buildSnippet(entry.Name, entry.Content, entry.FolderID, entry.CreatedAt), nil
		}
	}
	return snippet{}, errSnippetNotFound
}

func sanitizeName(name string) string {
	base := filepath.Base(name)
	if base != name || strings.Contains(name, "\x00") {
		return ""
	}
	return base
}

func matchesFolder(fileFolder, targetFolder *string) bool {
	if fileFolder == nil && targetFolder == nil {
		return true
	}
	if fileFolder != nil && targetFolder != nil && *fileFolder == *targetFolder {
		return true
	}
	return false
}

func deterministicUUID(inputs []string) string {
	var cleaned []string
	for _, input := range inputs {
		if input != "" {
			cleaned = append(cleaned, input)
		}
	}
	input := strings.Join(cleaned, "_")
	if input == "" {
		return uuid.NewString()
	}
	hash := simpleHash(input)
	bytes := make([]byte, 16)
	seed := uint32(hash)
	for i := 0; i < 16; i++ {
		seed = (seed*1103515245 + 12345) & 0x7fffffff
		bytes[i] = byte(seed >> 16)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	id, _ := uuid.FromBytes(bytes)
	return id.String()
}

func simpleHash(input string) int32 {
	var hash int32
	for _, r := range input {
		hash = (hash << 5) - hash + int32(r)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}
