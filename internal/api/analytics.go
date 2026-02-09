package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

func (api *API) retrieveAnalyticsData(r *http.Request, name, projectRef string, params map[string]string) (map[string]any, error) {
	if api.cfg.LogflareURL == "" {
		return nil, errors.New("LOGFLARE_URL is required")
	}
	if api.cfg.LogflareToken == "" {
		return nil, errors.New("LOGFLARE_PRIVATE_ACCESS_TOKEN is required")
	}

	base := api.cfg.LogflareURL
	endpoint, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	endpoint.Path = "/api/endpoints/query/" + name
	query := endpoint.Query()
	query.Set("project", projectRef)
	for k, v := range params {
		if v != "" {
			query.Set(k, v)
		}
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", api.cfg.LogflareToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, errors.New("failed to retrieve analytics data")
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}
