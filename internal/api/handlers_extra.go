package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func (api *API) handleIncidentStatus(w http.ResponseWriter, r *http.Request) {
	if !api.cfg.IsPlatform {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	const cacheControl = "public, max-age=300, s-maxage=300, stale-while-revalidate=60"
	if r.Method == http.MethodHead {
		w.Header().Set("Cache-Control", cacheControl)
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET, HEAD")
		return
	}

	pageID := os.Getenv("STATUSPAGE_PAGE_ID")
	apiKey := os.Getenv("STATUSPAGE_API_KEY")
	if pageID == "" || apiKey == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "StatusPage not configured"})
		return
	}

	endpoint := "https://api.statuspage.io/v1/pages/" + pageID + "/incidents/unresolved"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	req.Header.Set("Authorization", "OAuth "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := api.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Unable to fetch incidents at this time"})
		return
	}

	var payload []struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Status       string  `json:"status"`
		CreatedAt    string  `json:"created_at"`
		ScheduledFor *string `json:"scheduled_for"`
		Impact       string  `json:"impact"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Unable to parse incidents"})
		return
	}

	now := time.Now()
	var incidents []map[string]any
	for _, incident := range payload {
		activeSince := incident.CreatedAt
		if incident.ScheduledFor != nil && *incident.ScheduledFor != "" {
			if parsed, err := time.Parse(time.RFC3339, *incident.ScheduledFor); err == nil {
				if parsed.After(now) {
					continue
				}
				activeSince = parsed.Format(time.RFC3339)
			}
		}
		incidents = append(incidents, map[string]any{
			"id":           incident.ID,
			"name":         incident.Name,
			"status":       incident.Status,
			"impact":       incident.Impact,
			"active_since": activeSince,
		})
	}

	w.Header().Set("Cache-Control", cacheControl)
	writeJSON(w, http.StatusOK, incidents)
}

func (api *API) handleEdgeFunctionTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Body    any               `json:"body"`
		Headers map[string]string `json:"headers"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "Invalid request body"}})
		return
	}

	if !isValidEdgeFunctionURL(payload.URL) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"status": 400,
			"error":  map[string]any{"message": "Provided URL is not a valid Supabase edge function URL"},
		})
		return
	}

	headers := map[string]string{"Content-Type": "application/json"}
	for k, v := range payload.Headers {
		if v != "" {
			headers[k] = v
		}
	}
	if auth, ok := headers["x-test-authorization"]; ok {
		headers["Authorization"] = auth
		delete(headers, "x-test-authorization")
	}

	method := strings.ToUpper(payload.Method)
	if method == "" {
		method = http.MethodPost
	}

	var body io.Reader
	if method != http.MethodGet && method != http.MethodHead {
		if headers["Content-Type"] == "application/json" {
			bodyBytes, _ := json.Marshal(payload.Body)
			body = bytes.NewReader(bodyBytes)
		} else if payload.Body != nil {
			if s, ok := payload.Body.(string); ok {
				body = strings.NewReader(s)
			}
		}
	}

	req, err := http.NewRequestWithContext(r.Context(), method, payload.URL, body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": 500, "error": map[string]any{"message": err.Error()}})
		return
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := api.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": 500, "error": map[string]any{"message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	respBodyBytes, _ := io.ReadAll(resp.Body)
	contentType := resp.Header.Get("content-type")
	responseBody := string(respBodyBytes)

	if strings.Contains(contentType, "application/json") {
		var jsonBody any
		if err := json.Unmarshal(respBodyBytes, &jsonBody); err == nil {
			serialized, _ := json.Marshal(jsonBody)
			responseBody = string(serialized)
		}
	}

	if resp.StatusCode >= 400 {
		var parsed map[string]any
		if err := json.Unmarshal(respBodyBytes, &parsed); err == nil {
			if msg, ok := parsed["error"].(string); ok {
				writeJSON(w, resp.StatusCode, map[string]any{"status": resp.StatusCode, "error": map[string]any{"message": msg}})
				return
			}
		}
		writeJSON(w, resp.StatusCode, map[string]any{"status": resp.StatusCode, "error": map[string]any{"message": responseBody}})
		return
	}

	headersOut := map[string]string{}
	for key, values := range resp.Header {
		if len(values) > 0 {
			headersOut[key] = values[0]
		}
	}

	writeJSON(w, resp.StatusCode, map[string]any{
		"status":  resp.StatusCode,
		"headers": headersOut,
		"body":    responseBody,
	})
}

func isValidEdgeFunctionURL(urlStr string) bool {
	custom := os.Getenv("NIMBUS_PROD_PROJECTS_URL")
	if custom != "" {
		apex := strings.ReplaceAll(strings.TrimPrefix(custom, "https://*."), ".", "\\.")
		re := regexp.MustCompile("^https://[a-z]*\\." + apex + "/functions/v[0-9]{1}/.*$")
		return re.MatchString(urlStr)
	}
	re := regexp.MustCompile(`^https://[a-z]*\.supabase\.(red|co)/functions/v[0-9]{1}/.*$`)
	return re.MatchString(urlStr)
}

func (api *API) handleGenerateAttachmentURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	token := bearerToken(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"message": "Unauthorized"}})
		return
	}

	var payload struct {
		Filenames []string `json:"filenames"`
		Bucket    string   `json:"bucket"`
	}
	if err := decodeJSON(r, &payload); err != nil || len(payload.Filenames) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "Invalid request body"}})
		return
	}
	if payload.Bucket == "" {
		payload.Bucket = "support-attachments"
	}

	sub, err := extractJWTSubject(token, api.cfg.AuthJWTSecret)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"message": "Unauthorized"}})
		return
	}

	requestedPrefixes := map[string]struct{}{}
	for _, filename := range payload.Filenames {
		parts := strings.Split(filename, "/")
		if len(parts) > 0 {
			requestedPrefixes[parts[0]] = struct{}{}
		}
	}
	for prefix := range requestedPrefixes {
		if prefix != sub {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": map[string]any{"message": "Forbidden: Users can only access their own resources"}})
			return
		}
	}

	if api.cfg.SupportAPIURL == "" || api.cfg.SupportAPIKey == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "Support API is not configured"}})
		return
	}

	urlStr := strings.TrimSuffix(api.cfg.SupportAPIURL, "/") + "/storage/v1/object/sign/" + payload.Bucket
	body, _ := json.Marshal(map[string]any{
		"paths":     payload.Filenames,
		"expiresIn": 10 * 365 * 24 * 60 * 60,
	})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, urlStr, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", api.cfg.SupportAPIKey)
	req.Header.Set("Authorization", "Bearer "+api.cfg.SupportAPIKey)

	resp, err := api.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "Failed to sign URLs for attachments"}})
		return
	}

	var response struct {
		SignedUrls []struct {
			SignedURL string `json:"signedURL"`
		} `json:"signedUrls"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "Failed to sign URLs for attachments"}})
		return
	}

	var urls []string
	for _, item := range response.SignedUrls {
		urls = append(urls, item.SignedURL)
	}

	writeJSON(w, http.StatusOK, urls)
}

func extractJWTSubject(token, secret string) (string, error) {
	parsed, err := jwt.Parse(token, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		return "", errors.New("invalid token")
	}
	if claims, ok := parsed.Claims.(jwt.MapClaims); ok {
		if sub, ok := claims["sub"].(string); ok {
			return sub, nil
		}
	}
	return "", errors.New("invalid token")
}

func (api *API) handleMCP(w http.ResponseWriter, r *http.Request) {
	respondNotImplemented(w, "MCP endpoint is not available in the Go runtime")
}

func parseOpenAIModelsEnv() []string {
	raw := strings.TrimSpace(os.Getenv("OPENAI_MODELS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	}
	if raw == "" {
		return nil
	}

	var models []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &models); err == nil {
			return normalizeModelList(models)
		}
		if strings.HasSuffix(raw, "]") {
			raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]"))
		}
	}

	parts := strings.Split(raw, ",")
	for _, part := range parts {
		model := strings.TrimSpace(part)
		model = strings.Trim(model, "\"'")
		if model != "" {
			models = append(models, model)
		}
	}

	return normalizeModelList(models)
}

func normalizeModelList(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	normalized := make([]string, 0, len(models))

	for _, model := range models {
		value := strings.TrimSpace(model)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	return normalized
}

func (api *API) handleCheckAPIKey(w http.ResponseWriter, r *http.Request) {
	models := parseOpenAIModelsEnv()
	defaultModel := ""
	if len(models) > 0 {
		defaultModel = models[0]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"hasKey":       os.Getenv("OPENAI_API_KEY") != "",
		"models":       models,
		"defaultModel": defaultModel,
	})
}

type aiGenerateV4Request struct {
	Messages []aiUIMessage `json:"messages"`
	Model    string        `json:"model"`
}

type aiUIMessage struct {
	Role    string     `json:"role"`
	Content any        `json:"content"`
	Parts   []aiUIPart `json:"parts"`
}

type aiUIPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

type openAIChatStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content any `json:"content"`
		} `json:"delta"`
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (api *API) handleAISQLGenerateV4(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "OPENAI_API_KEY is not configured",
		})
		return
	}

	var payload aiGenerateV4Request
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "Invalid request body",
		})
		return
	}

	models := parseOpenAIModelsEnv()
	model := pickAIModel(payload.Model, models)
	if model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "No AI model configured. Set OPENAI_MODELS or OPENAI_MODEL.",
		})
		return
	}

	openAIMessages := buildOpenAIMessages(payload.Messages)
	if len(openAIMessages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "At least one text message is required",
		})
		return
	}

	requestBody := openAIChatRequest{
		Model:    model,
		Messages: openAIMessages,
		Stream:   true,
	}
	bodyBytes, _ := json.Marshal(requestBody)

	urlStr := resolveOpenAIChatCompletionsURL()
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, urlStr, bytes.NewReader(bodyBytes))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "Failed to create upstream request",
		})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := api.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": fmt.Sprintf("Upstream AI request failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(respBytes))
		var upstreamErr openAIChatResponse
		if err := json.Unmarshal(respBytes, &upstreamErr); err == nil && upstreamErr.Error != nil && upstreamErr.Error.Message != "" {
			msg = upstreamErr.Error.Message
		}
		if msg == "" {
			msg = "Upstream AI request failed"
		}
		writeJSON(w, resp.StatusCode, map[string]any{
			"error": msg,
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "Streaming is not supported by this server",
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Vercel-AI-UI-Message-Stream", "v1")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	const textID = "text-1"
	_ = writeSSEChunk(w, flusher, map[string]any{"type": "start"})
	_ = writeSSEChunk(w, flusher, map[string]any{"type": "text-start", "id": textID})

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	wroteDelta := false

	if strings.Contains(contentType, "text/event-stream") {
		_ = streamOpenAIResponse(resp.Body, func(delta string) error {
			if delta == "" {
				return nil
			}
			for _, piece := range splitStreamingText(delta) {
				wroteDelta = true
				if err := writeSSEChunk(w, flusher, map[string]any{"type": "text-delta", "id": textID, "delta": piece}); err != nil {
					return err
				}
			}
			return nil
		})
	} else {
		respBytes, _ := io.ReadAll(resp.Body)
		var completion openAIChatResponse
		if err := json.Unmarshal(respBytes, &completion); err == nil && len(completion.Choices) > 0 {
			answer := extractOpenAIContentText(completion.Choices[0].Message.Content)
			if answer != "" {
				wroteDelta = true
				_ = writeSSEChunk(w, flusher, map[string]any{"type": "text-delta", "id": textID, "delta": answer})
			}
		}
	}

	if !wroteDelta {
		_ = writeSSEChunk(w, flusher, map[string]any{
			"type":  "text-delta",
			"id":    textID,
			"delta": "No content returned from upstream model.",
		})
	}

	_ = writeSSEChunk(w, flusher, map[string]any{"type": "text-end", "id": textID})
	_ = writeSSEChunk(w, flusher, map[string]any{"type": "finish"})
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func streamOpenAIResponse(body io.Reader, onDelta func(string) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			return nil
		}

		var chunk openAIChatStreamResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			delta := extractOpenAIContentText(choice.Delta.Content)
			if delta == "" {
				delta = extractOpenAIContentText(choice.Message.Content)
			}
			if delta == "" || strings.EqualFold(strings.TrimSpace(delta), "null") {
				continue
			}
			if err := onDelta(delta); err != nil {
				return err
			}
		}
	}

	return scanner.Err()
}

func splitStreamingText(text string) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	out := make([]string, 0, len(runes))
	for _, r := range runes {
		out = append(out, string(r))
	}
	return out
}

func resolveOpenAIChatCompletionsURL() string {
	raw := strings.TrimSpace(os.Getenv("OPENAI_API_URL"))
	if raw == "" {
		return "https://api.openai.com/v1/chat/completions"
	}
	trimmed := strings.TrimRight(raw, "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	return trimmed + "/chat/completions"
}

func pickAIModel(requested string, configured []string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		if len(configured) == 0 || containsString(configured, requested) {
			return requested
		}
	}

	if len(configured) > 0 {
		return configured[0]
	}

	if fallback := strings.TrimSpace(os.Getenv("OPENAI_MODEL")); fallback != "" {
		return fallback
	}

	return ""
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func buildOpenAIMessages(messages []aiUIMessage) []openAIChatMessage {
	result := make([]openAIChatMessage, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		switch role {
		case "system", "user", "assistant":
		default:
			continue
		}

		text := extractUIMessageText(message)
		if strings.TrimSpace(text) == "" {
			continue
		}

		result = append(result, openAIChatMessage{
			Role:    role,
			Content: text,
		})
	}
	return result
}

func extractUIMessageText(message aiUIMessage) string {
	if len(message.Parts) > 0 {
		parts := make([]string, 0, len(message.Parts))
		for _, part := range message.Parts {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}

	switch content := message.Content.(type) {
	case string:
		return content
	case []any:
		parts := make([]string, 0, len(content))
		for _, item := range content {
			if part, ok := item.(map[string]any); ok {
				if text, ok := part["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func extractOpenAIContentText(content any) string {
	if content == nil {
		return ""
	}

	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if part, ok := item.(map[string]any); ok {
				if text, ok := part["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text, ok := value["text"].(string); ok {
			return text
		}
		bytes, _ := json.Marshal(value)
		return string(bytes)
	default:
		bytes, _ := json.Marshal(value)
		text := string(bytes)
		if strings.EqualFold(strings.TrimSpace(text), "null") {
			return ""
		}
		return text
	}
}

func writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, chunk any) error {
	payload, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (api *API) handleStripeSync(w http.ResponseWriter, r *http.Request) {
	respondNotImplemented(w, "Stripe Sync integration is not available in the Go runtime")
}
