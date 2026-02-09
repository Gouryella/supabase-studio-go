package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (api *API) handleGetIPAddress(w http.ResponseWriter, r *http.Request) {
	ipAddress := extractIPAddress(r)
	writeJSON(w, http.StatusOK, map[string]any{"ipAddress": ipAddress})
}

func extractIPAddress(r *http.Request) string {
	// Cloudflare connecting IP takes precedence
	if cf := r.Header.Get("cf-connecting-ip"); cf != "" {
		return strings.Split(cf, ",")[0]
	}
	// Try X-Real-IP header
	if realIP := r.Header.Get("x-real-ip"); realIP != "" {
		return realIP
	}
	// Try X-Forwarded-For header
	if forwardedFor := r.Header.Get("x-forwarded-for"); forwardedFor != "" {
		return strings.Split(forwardedFor, ",")[0]
	}
	// Fall back to remote address
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return ""
}

func (api *API) handleGetUTCTime(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"utcTime": time.Now().UTC().Format(time.RFC3339)})
}

func (api *API) handleDeploymentCommit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "s-maxage=600")

	commitSha := os.Getenv("VERCEL_GIT_COMMIT_SHA")
	if commitSha == "" {
		commitSha = "development"
	}

	commitTime := "unknown"
	if commitSha != "development" {
		if timeStr, err := fetchCommitTime(r.Context(), commitSha); err == nil {
			commitTime = timeStr
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"commitSha":  commitSha,
		"commitTime": commitTime,
	})
}

func fetchCommitTime(ctx context.Context, commitSha string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://github.com/supabase/supabase/commit/"+commitSha+".json", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", errors.New("failed to fetch commit details")
	}
	var payload struct {
		Payload struct {
			Commit struct {
				CommittedDate string `json:"committedDate"`
			} `json:"commit"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.Payload.Commit.CommittedDate, nil
}

func (api *API) handleCLIReleaseVersion(w http.ResponseWriter, r *http.Request) {
	type release struct {
		TagName     string `json:"tag_name"`
		PublishedAt string `json:"published_at"`
	}

	current := ""
	if val := os.Getenv("CURRENT_CLI_VERSION"); val != "" {
		current = "v" + val
	}

	latest := ""
	publishedAt := ""
	beta := ""

	if resp, err := http.Get("https://api.github.com/repos/supabase/cli/releases/latest"); err == nil {
		defer resp.Body.Close()
		var rls release
		if err := json.NewDecoder(resp.Body).Decode(&rls); err == nil {
			latest = rls.TagName
			publishedAt = rls.PublishedAt
		}
	}

	if resp, err := http.Get("https://api.github.com/repos/supabase/cli/releases?per_page=1"); err == nil {
		defer resp.Body.Close()
		var rls []release
		if err := json.NewDecoder(resp.Body).Decode(&rls); err == nil && len(rls) > 0 {
			beta = rls[0].TagName
		}
	}

	payload := map[string]any{
		"current": current,
	}
	if latest != "" {
		payload["latest"] = latest
		payload["beta"] = beta
		payload["published_at"] = publishedAt
	}

	writeJSON(w, http.StatusOK, payload)
}

func (api *API) handleCheckCNAME(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "domain is required"})
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://cloudflare-dns.com/dns-query?name="+domain+"&type=CNAME", nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
		return
	}
	req.Header.Set("Accept", "application/dns-json")

	resp, err := api.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
		return
	}
	defer resp.Body.Close()

	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, payload)
}

func (api *API) handleConnectContent(w http.ResponseWriter, r *http.Request) {
	root := os.Getenv("STUDIO_CONNECT_CONTENT_DIR")
	if root == "" {
		root = filepath.Join("components", "interfaces", "Home", "Connect", "content")
	}
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			files = append(files, rel)
		}
		return nil
	})

	writeJSON(w, http.StatusOK, files)
}
