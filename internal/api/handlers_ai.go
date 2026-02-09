package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type aiPolicyRequest struct {
	TableName string   `json:"tableName"`
	Schema    string   `json:"schema"`
	Columns   []string `json:"columns"`
	Message   string   `json:"message"`
	Model     string   `json:"model"`
}

type aiPolicyItem struct {
	SQL        string   `json:"sql"`
	Name       string   `json:"name"`
	Command    string   `json:"command"`
	Definition string   `json:"definition,omitempty"`
	Check      string   `json:"check,omitempty"`
	Action     string   `json:"action"`
	Roles      []string `json:"roles"`
	Table      string   `json:"table"`
	Schema     string   `json:"schema"`
}

type aiCronRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

type aiTitleRequest struct {
	SQL   string `json:"sql"`
	Model string `json:"model"`
}

type aiCodeCompleteRequest struct {
	Model              string `json:"model"`
	Language           string `json:"language"`
	CompletionMetadata struct {
		TextBeforeCursor string `json:"textBeforeCursor"`
		TextAfterCursor  string `json:"textAfterCursor"`
		Prompt           string `json:"prompt"`
		Selection        string `json:"selection"`
	} `json:"completionMetadata"`
}

type aiFeedbackRateRequest struct {
	Rating   string `json:"rating"`
	Reason   string `json:"reason"`
	Messages []any  `json:"messages"`
}

type aiFeedbackClassifyRequest struct {
	Prompt string `json:"prompt"`
}

type aiDocsRequest struct {
	Messages []openAIChatMessage `json:"messages"`
	Model    string              `json:"model"`
}

type aiOnboardingRequest struct {
	Messages []aiUIMessage `json:"messages"`
	Model    string        `json:"model"`
}

type aiFilterProperty struct {
	Label     string `json:"label"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Operators []any  `json:"operators"`
	Options   []any  `json:"options"`
}

type aiFilterRequest struct {
	Prompt           string             `json:"prompt"`
	FilterProperties []aiFilterProperty `json:"filterProperties"`
	Model            string             `json:"model"`
}

func (api *API) handleAISQLPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload aiPolicyRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeAIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	payload.TableName = strings.TrimSpace(payload.TableName)
	if payload.TableName == "" {
		writeAIError(w, http.StatusBadRequest, "tableName is required")
		return
	}
	if strings.TrimSpace(payload.Schema) == "" {
		payload.Schema = "public"
	}

	prompt := fmt.Sprintf(
		"Generate Postgres RLS policies for table %q.%q.\nColumns: %v\nUser request: %s\n"+
			"Return STRICT JSON only. Either an array or an object {\"policies\":[...]}.\n"+
			"Each policy item fields: name, sql, command (SELECT|INSERT|UPDATE|DELETE|ALL), action (PERMISSIVE|RESTRICTIVE), roles (string[]), definition (optional), check (optional).\n"+
			"No markdown, no explanation.",
		payload.Schema,
		payload.TableName,
		payload.Columns,
		payload.Message,
	)

	answer, _, status, errMsg := api.generateOpenAIText(r.Context(), payload.Model, []openAIChatMessage{
		{Role: "system", Content: "You are a Postgres RLS expert. Output valid JSON only."},
		{Role: "user", Content: prompt},
	})
	if errMsg != "" {
		writeAIError(w, status, errMsg)
		return
	}

	policies := parsePolicies(answer)
	if len(policies) == 0 {
		policies = []aiPolicyItem{buildFallbackPolicy(payload)}
	}

	for i := range policies {
		policies[i] = sanitizePolicy(policies[i], payload)
	}

	writeJSON(w, http.StatusOK, policies)
}

func (api *API) handleAISQLCronV2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload aiCronRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeAIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	payload.Prompt = strings.TrimSpace(payload.Prompt)
	if payload.Prompt == "" {
		writeAIError(w, http.StatusBadRequest, "Prompt is required")
		return
	}

	answer, _, status, errMsg := api.generateOpenAIText(r.Context(), payload.Model, []openAIChatMessage{
		{
			Role:    "system",
			Content: "Convert natural language to pg_cron expression. Output only expression text.",
		},
		{Role: "user", Content: payload.Prompt},
	})
	if errMsg != "" {
		writeAIError(w, status, errMsg)
		return
	}

	cronExpr := normalizeCronExpression(answer)
	if cronExpr == "" {
		cronExpr = "* * * * *"
	}
	writeJSON(w, http.StatusOK, cronExpr)
}

func (api *API) handleAISQLTitleV2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload aiTitleRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeAIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	payload.SQL = strings.TrimSpace(payload.SQL)
	if payload.SQL == "" {
		writeAIError(w, http.StatusBadRequest, "SQL query is required")
		return
	}

	answer, _, status, errMsg := api.generateOpenAIText(r.Context(), payload.Model, []openAIChatMessage{
		{
			Role:    "system",
			Content: "Generate concise SQL snippet metadata. Output STRICT JSON only: {\"title\":\"...\",\"description\":\"...\"}",
		},
		{Role: "user", Content: payload.SQL},
	})
	if errMsg != "" {
		writeAIError(w, status, errMsg)
		return
	}

	var result struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := parseJSONFromModelOutput(answer, &result); err != nil {
		result.Title = fallbackTitleFromSQL(payload.SQL)
		result.Description = fallbackDescriptionFromSQL(payload.SQL)
	}
	result.Title = strings.TrimSpace(result.Title)
	result.Description = strings.TrimSpace(result.Description)
	if result.Title == "" {
		result.Title = fallbackTitleFromSQL(payload.SQL)
	}
	if result.Description == "" {
		result.Description = fallbackDescriptionFromSQL(payload.SQL)
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"title":       result.Title,
		"description": result.Description,
	})
}

func (api *API) handleAISQLFilterV1(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload aiFilterRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeAIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	payload.Prompt = strings.TrimSpace(payload.Prompt)
	if payload.Prompt == "" {
		writeAIError(w, http.StatusBadRequest, "Prompt is required")
		return
	}
	if len(payload.FilterProperties) == 0 {
		writeAIError(w, http.StatusBadRequest, "At least one filter property is required")
		return
	}

	filterPrompt := fmt.Sprintf(
		"Convert this user request into JSON filter group only.\nRequest: %q\nAllowed properties: %s\n"+
			"Return format: {\"logicalOperator\":\"AND|OR\",\"conditions\":[{\"propertyName\":\"...\",\"operator\":\"...\",\"value\":...}]}\n"+
			"No markdown.",
		payload.Prompt,
		mustJSON(payload.FilterProperties),
	)

	answer, _, status, errMsg := api.generateOpenAIText(r.Context(), payload.Model, []openAIChatMessage{
		{
			Role:    "system",
			Content: "You build structured SQL filters. Output strict JSON only.",
		},
		{Role: "user", Content: filterPrompt},
	})
	if errMsg != "" {
		writeAIError(w, status, errMsg)
		return
	}

	propertiesByName := make(map[string]aiFilterProperty, len(payload.FilterProperties))
	for _, property := range payload.FilterProperties {
		if name := strings.TrimSpace(property.Name); name != "" {
			propertiesByName[name] = property
		}
	}

	var raw any
	if err := parseJSONFromModelOutput(answer, &raw); err == nil {
		if sanitized, ok := sanitizeFilterGroup(raw, propertiesByName); ok {
			writeJSON(w, http.StatusOK, sanitized)
			return
		}
	}

	writeJSON(w, http.StatusOK, buildFallbackFilterGroup(payload.Prompt, payload.FilterProperties))
}

func (api *API) handleAICodeComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload aiCodeCompleteRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeAIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	meta := payload.CompletionMetadata
	prompt := strings.TrimSpace(meta.Prompt)
	selection := meta.Selection
	if prompt == "" {
		writeJSON(w, http.StatusOK, selection)
		return
	}

	userPrompt := fmt.Sprintf(
		"Language: %s\nInstruction: %s\nBefore:\n%s\nSelection:\n%s\nAfter:\n%s\n"+
			"Return ONLY the replacement text for Selection, no markdown.",
		strings.TrimSpace(payload.Language),
		prompt,
		meta.TextBeforeCursor,
		selection,
		meta.TextAfterCursor,
	)

	answer, _, status, errMsg := api.generateOpenAIText(r.Context(), payload.Model, []openAIChatMessage{
		{Role: "system", Content: "You are a code completion assistant. Return replacement text only."},
		{Role: "user", Content: userPrompt},
	})
	if errMsg != "" {
		writeAIError(w, status, errMsg)
		return
	}

	completion := cleanModelTextOutput(answer)
	if completion == "" {
		completion = selection
	}
	writeJSON(w, http.StatusOK, completion)
}

func (api *API) handleAIFeedbackRate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload aiFeedbackRateRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeAIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	contextText := strings.ToLower(mustJSON(payload.Messages) + "\n" + payload.Reason + "\n" + payload.Rating)
	category := classifyFeedbackConversation(contextText)
	writeJSON(w, http.StatusOK, map[string]string{
		"category": category,
	})
}

func (api *API) handleAIFeedbackClassify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload aiFeedbackClassifyRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeAIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	prompt := strings.TrimSpace(payload.Prompt)
	if prompt == "" {
		writeJSON(w, http.StatusOK, map[string]string{"feedback_category": "unknown"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"feedback_category": classifyFeedbackPrompt(prompt),
	})
}

func (api *API) handleAIDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload aiDocsRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeAIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(payload.Messages) == 0 {
		writeAIError(w, http.StatusBadRequest, "messages are required")
		return
	}

	answer, model, status, errMsg := api.generateOpenAIText(r.Context(), payload.Model, payload.Messages)
	if errMsg != "" {
		writeAIError(w, status, errMsg)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAIError(w, http.StatusInternalServerError, "Streaming is not supported by this server")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	chunks := splitTextChunks(answer, 220)
	now := time.Now().Unix()
	for idx, part := range chunks {
		delta := map[string]any{"content": part}
		if idx == 0 {
			delta["role"] = "assistant"
		}
		_ = writeSSEChunk(w, flusher, map[string]any{
			"id":      "chatcmpl-supabase-studio-go",
			"object":  "chat.completion.chunk",
			"created": now,
			"model":   model,
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         delta,
					"finish_reason": nil,
				},
			},
		})
	}

	_ = writeSSEChunk(w, flusher, map[string]any{
		"id":      "chatcmpl-supabase-studio-go",
		"object":  "chat.completion.chunk",
		"created": now,
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			},
		},
	})
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func (api *API) handleAIOnboardingDesign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, "POST")
		return
	}

	var payload aiOnboardingRequest
	if err := decodeJSON(r, &payload); err != nil {
		writeAIError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(payload.Messages) == 0 {
		writeAIError(w, http.StatusBadRequest, "messages are required")
		return
	}

	prompt := extractLatestUserPrompt(payload.Messages)
	if prompt == "" {
		prompt = "Create an initial database schema."
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAIError(w, http.StatusInternalServerError, "Streaming is not supported by this server")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Vercel-AI-UI-Message-Stream", "v1")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	_ = writeSSEChunk(w, flusher, map[string]any{"type": "start"})

	if isResetRequest(prompt) {
		_ = writeSSEChunk(w, flusher, map[string]any{
			"type":       "tool-input-start",
			"toolCallId": "tool-reset-1",
			"toolName":   "reset",
		})
		_ = writeSSEChunk(w, flusher, map[string]any{
			"type":       "tool-input-available",
			"toolCallId": "tool-reset-1",
			"toolName":   "reset",
			"input":      map[string]any{},
		})
		_ = writeSSEChunk(w, flusher, map[string]any{"type": "text-start", "id": "text-1"})
		_ = writeSSEChunk(w, flusher, map[string]any{
			"type":  "text-delta",
			"id":    "text-1",
			"delta": "Reset requested. I cleared the generated setup and you can describe a fresh project now.",
		})
		_ = writeSSEChunk(w, flusher, map[string]any{"type": "text-end", "id": "text-1"})
		_ = writeSSEChunk(w, flusher, map[string]any{"type": "finish"})
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
		return
	}

	sql := api.generateOnboardingSQL(r.Context(), payload.Model, prompt)
	services := inferServicesFromPrompt(prompt)
	title := inferProjectTitle(prompt)
	summary := "Generated an initial schema, selected recommended Supabase services, and set a project title."

	_ = writeSSEChunk(w, flusher, map[string]any{
		"type":       "tool-input-start",
		"toolCallId": "tool-sql-1",
		"toolName":   "executeSql",
	})
	_ = writeSSEChunk(w, flusher, map[string]any{
		"type":       "tool-input-available",
		"toolCallId": "tool-sql-1",
		"toolName":   "executeSql",
		"input": map[string]any{
			"sql": sql,
		},
	})

	_ = writeSSEChunk(w, flusher, map[string]any{
		"type":       "tool-input-start",
		"toolCallId": "tool-services-1",
		"toolName":   "setServices",
	})
	_ = writeSSEChunk(w, flusher, map[string]any{
		"type":       "tool-input-available",
		"toolCallId": "tool-services-1",
		"toolName":   "setServices",
		"input": map[string]any{
			"services": services,
		},
	})

	_ = writeSSEChunk(w, flusher, map[string]any{
		"type":       "tool-input-start",
		"toolCallId": "tool-title-1",
		"toolName":   "setTitle",
	})
	_ = writeSSEChunk(w, flusher, map[string]any{
		"type":       "tool-input-available",
		"toolCallId": "tool-title-1",
		"toolName":   "setTitle",
		"input": map[string]any{
			"title": title,
		},
	})

	_ = writeSSEChunk(w, flusher, map[string]any{"type": "text-start", "id": "text-1"})
	_ = writeSSEChunk(w, flusher, map[string]any{"type": "text-delta", "id": "text-1", "delta": summary})
	_ = writeSSEChunk(w, flusher, map[string]any{"type": "text-end", "id": "text-1"})
	_ = writeSSEChunk(w, flusher, map[string]any{"type": "finish"})
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func (api *API) generateOpenAIText(ctx context.Context, requestedModel string, messages []openAIChatMessage) (string, string, int, string) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return "", "", http.StatusBadRequest, "OPENAI_API_KEY is not configured"
	}

	model := pickAIModel(requestedModel, parseOpenAIModelsEnv())
	if model == "" {
		return "", "", http.StatusBadRequest, "No AI model configured. Set OPENAI_MODELS or OPENAI_MODEL."
	}
	if len(messages) == 0 {
		return "", model, http.StatusBadRequest, "At least one message is required"
	}

	requestBody := openAIChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	bodyBytes, _ := json.Marshal(requestBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resolveOpenAIChatCompletionsURL(), bytes.NewReader(bodyBytes))
	if err != nil {
		return "", model, http.StatusInternalServerError, "Failed to create upstream request"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := api.client.Do(req)
	if err != nil {
		return "", model, http.StatusBadGateway, fmt.Sprintf("Upstream AI request failed: %v", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", model, resp.StatusCode, parseUpstreamAIError(respBytes)
	}

	var completion openAIChatResponse
	if err := json.Unmarshal(respBytes, &completion); err != nil {
		return "", model, http.StatusBadGateway, "Failed to parse upstream AI response"
	}
	if len(completion.Choices) == 0 {
		return "", model, http.StatusBadGateway, "Upstream AI response did not contain any choices"
	}

	return strings.TrimSpace(extractOpenAIContentText(completion.Choices[0].Message.Content)), model, 0, ""
}

func parseUpstreamAIError(respBytes []byte) string {
	var upstreamErr openAIChatResponse
	if err := json.Unmarshal(respBytes, &upstreamErr); err == nil && upstreamErr.Error != nil {
		if message := strings.TrimSpace(upstreamErr.Error.Message); message != "" {
			return message
		}
	}

	message := strings.TrimSpace(string(respBytes))
	if message == "" {
		return "Upstream AI request failed"
	}
	return message
}

func writeAIError(w http.ResponseWriter, status int, message string) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, map[string]any{
		"message": message,
		"error":   message,
	})
}

func parseJSONFromModelOutput(text string, target any) error {
	cleaned := cleanModelTextOutput(text)
	if cleaned == "" {
		return fmt.Errorf("empty output")
	}
	if err := json.Unmarshal([]byte(cleaned), target); err == nil {
		return nil
	}

	start := strings.IndexAny(cleaned, "{[")
	end := strings.LastIndexAny(cleaned, "}]")
	if start >= 0 && end > start {
		return json.Unmarshal([]byte(cleaned[start:end+1]), target)
	}

	return fmt.Errorf("could not parse model output as json")
}

func cleanModelTextOutput(text string) string {
	text = strings.TrimSpace(text)
	if fenceStart := strings.Index(text, "```"); fenceStart >= 0 {
		fenced := text[fenceStart+3:]
		if newline := strings.Index(fenced, "\n"); newline >= 0 {
			fenced = fenced[newline+1:]
		}
		if fenceEnd := strings.Index(fenced, "```"); fenceEnd >= 0 {
			text = fenced[:fenceEnd]
		}
	}
	if strings.HasPrefix(strings.TrimSpace(text), "```") {
		lines := strings.Split(strings.TrimSpace(text), "\n")
		if len(lines) > 0 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		text = strings.Join(lines, "\n")
	}
	return strings.TrimSpace(strings.Trim(text, "`"))
}

func parsePolicies(answer string) []aiPolicyItem {
	var policies []aiPolicyItem
	if err := parseJSONFromModelOutput(answer, &policies); err == nil && len(policies) > 0 {
		return policies
	}

	var wrapped struct {
		Policies []aiPolicyItem `json:"policies"`
	}
	if err := parseJSONFromModelOutput(answer, &wrapped); err == nil && len(wrapped.Policies) > 0 {
		return wrapped.Policies
	}

	return nil
}

func sanitizePolicy(policy aiPolicyItem, req aiPolicyRequest) aiPolicyItem {
	policy.Table = req.TableName
	policy.Schema = req.Schema

	policy.Name = strings.TrimSpace(policy.Name)
	if policy.Name == "" {
		policy.Name = "Allow access"
	}

	policy.Command = strings.ToUpper(strings.TrimSpace(policy.Command))
	switch policy.Command {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "ALL":
	default:
		policy.Command = "SELECT"
	}

	policy.Action = strings.ToUpper(strings.TrimSpace(policy.Action))
	switch policy.Action {
	case "PERMISSIVE", "RESTRICTIVE":
	default:
		policy.Action = "PERMISSIVE"
	}

	if len(policy.Roles) == 0 {
		policy.Roles = []string{"public"}
	}
	for i := range policy.Roles {
		role := strings.TrimSpace(policy.Roles[i])
		if role == "" {
			role = "public"
		}
		policy.Roles[i] = role
	}

	policy.Definition = strings.TrimSpace(policy.Definition)
	policy.Check = strings.TrimSpace(policy.Check)

	fallbackExpr := policyFallbackExpression(req.Columns)
	if looksNaturalLanguageExpression(policy.Definition) {
		policy.Definition = fallbackExpr
	}
	if looksNaturalLanguageExpression(policy.Check) {
		policy.Check = fallbackExpr
	}

	policy.SQL = strings.TrimSpace(policy.SQL)
	if policy.SQL != "" && !strings.Contains(strings.ToLower(policy.SQL), "create policy") {
		if policy.Definition == "" {
			policy.Definition = policy.SQL
		}
		if (policy.Command == "INSERT" || policy.Command == "UPDATE" || policy.Command == "ALL") && policy.Check == "" {
			policy.Check = policy.SQL
		}
		policy.SQL = ""
	}
	if policy.SQL == "" {
		policy.SQL = buildPolicySQL(policy)
	}

	return policy
}

func policyFallbackExpression(columns []string) string {
	for _, column := range columns {
		switch strings.ToLower(strings.TrimSpace(column)) {
		case "user_id":
			return "auth.uid() = user_id"
		case "owner_id":
			return "auth.uid() = owner_id"
		}
	}
	return "true"
}

func looksNaturalLanguageExpression(expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}

	lower := strings.ToLower(expr)
	if containsAny(lower, "allow ", "users ", "should ", "must ", "only ", "their own") {
		return true
	}

	sqlSignal := regexp.MustCompile(`(?i)(=|<>|!=|<=|>=|\bauth\.uid\(\)|\bcurrent_user|\bexists\b|\bin\b|\btrue\b|\bfalse\b|\(|\)|::)`)
	return !sqlSignal.MatchString(expr)
}

func buildFallbackPolicy(req aiPolicyRequest) aiPolicyItem {
	definition := "true"
	check := ""
	for _, column := range req.Columns {
		switch strings.ToLower(strings.TrimSpace(column)) {
		case "user_id", "owner_id":
			definition = "auth.uid() = " + strings.TrimSpace(column)
			check = definition
		}
	}

	policy := aiPolicyItem{
		Name:       "Allow read access",
		Command:    "SELECT",
		Definition: definition,
		Check:      check,
		Action:     "PERMISSIVE",
		Roles:      []string{"public"},
		Table:      req.TableName,
		Schema:     req.Schema,
	}
	policy.SQL = buildPolicySQL(policy)
	return policy
}

func buildPolicySQL(policy aiPolicyItem) string {
	roles := strings.Join(policy.Roles, ", ")
	if roles == "" {
		roles = "public"
	}
	definition := strings.TrimSpace(policy.Definition)
	check := strings.TrimSpace(policy.Check)

	var sql strings.Builder
	sql.WriteString(`create policy "`)
	sql.WriteString(strings.ReplaceAll(policy.Name, `"`, `\"`))
	sql.WriteString(`" on "`)
	sql.WriteString(strings.ReplaceAll(policy.Schema, `"`, `\"`))
	sql.WriteString(`"."`)
	sql.WriteString(strings.ReplaceAll(policy.Table, `"`, `\"`))
	sql.WriteString(`" as `)
	sql.WriteString(policy.Action)
	sql.WriteString(` for `)
	sql.WriteString(policy.Command)
	sql.WriteString(` to `)
	sql.WriteString(roles)

	switch policy.Command {
	case "SELECT", "DELETE":
		if definition == "" {
			definition = "true"
		}
		sql.WriteString(" using (")
		sql.WriteString(definition)
		sql.WriteString(")")
	case "INSERT":
		if check == "" {
			check = "true"
		}
		sql.WriteString(" with check (")
		sql.WriteString(check)
		sql.WriteString(")")
	case "UPDATE":
		if definition == "" {
			definition = "true"
		}
		if check == "" {
			check = definition
		}
		sql.WriteString(" using (")
		sql.WriteString(definition)
		sql.WriteString(")")
		sql.WriteString(" with check (")
		sql.WriteString(check)
		sql.WriteString(")")
	case "ALL":
		if definition != "" {
			sql.WriteString(" using (")
			sql.WriteString(definition)
			sql.WriteString(")")
		}
		if check != "" {
			sql.WriteString(" with check (")
			sql.WriteString(check)
			sql.WriteString(")")
		}
	}

	sql.WriteString(";")
	return sql.String()
}

func normalizeCronExpression(raw string) string {
	trimmed := cleanModelTextOutput(raw)
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.Trim(line, `"'`))
		if line == "" {
			continue
		}

		secRegex := regexp.MustCompile(`(?i)\b(\d+)\s*seconds?\b`)
		if match := secRegex.FindStringSubmatch(line); len(match) == 2 {
			return match[1] + " seconds"
		}

		cronRegex := regexp.MustCompile(`([*0-9\/,\-]+\s+){4}[*0-9\/,\-]+`)
		if match := cronRegex.FindString(line); strings.TrimSpace(match) != "" {
			return strings.TrimSpace(match)
		}

		return line
	}

	return ""
}

func fallbackTitleFromSQL(sql string) string {
	sql = strings.TrimSpace(strings.ToLower(sql))
	switch {
	case strings.HasPrefix(sql, "select"):
		return "Select Records"
	case strings.HasPrefix(sql, "insert"):
		return "Insert Records"
	case strings.HasPrefix(sql, "update"):
		return "Update Records"
	case strings.HasPrefix(sql, "delete"):
		return "Delete Records"
	case strings.HasPrefix(sql, "create table"):
		return "Create Table"
	default:
		return "SQL Snippet"
	}
}

func fallbackDescriptionFromSQL(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "Generated SQL snippet."
	}
	if len(sql) > 120 {
		sql = sql[:120] + "..."
	}
	return "SQL snippet: " + sql
}

func mustJSON(value any) string {
	bytes, _ := json.Marshal(value)
	return string(bytes)
}

func sanitizeFilterGroup(raw any, properties map[string]aiFilterProperty) (map[string]any, bool) {
	group, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}

	logicalOperator := strings.ToUpper(strings.TrimSpace(stringFromAny(group["logicalOperator"])))
	if logicalOperator != "OR" {
		logicalOperator = "AND"
	}

	rawConditions, _ := group["conditions"].([]any)
	conditions := make([]any, 0, len(rawConditions))
	for _, rawCondition := range rawConditions {
		conditionMap, isMap := rawCondition.(map[string]any)
		if !isMap {
			continue
		}

		if _, hasNested := conditionMap["conditions"]; hasNested {
			nested, ok := sanitizeFilterGroup(conditionMap, properties)
			if ok {
				conditions = append(conditions, nested)
			}
			continue
		}

		propertyName := strings.TrimSpace(stringFromAny(conditionMap["propertyName"]))
		if propertyName == "" {
			continue
		}
		property, exists := properties[propertyName]
		if !exists {
			continue
		}

		operator := strings.TrimSpace(stringFromAny(conditionMap["operator"]))
		allowedOperators := normalizeOperators(property.Operators)
		if operator == "" {
			operator = firstOrDefault(allowedOperators, "=")
		}
		if len(allowedOperators) > 0 && !containsString(allowedOperators, operator) {
			operator = allowedOperators[0]
		}

		value := coerceFilterValue(conditionMap["value"], property)
		conditions = append(conditions, map[string]any{
			"propertyName": propertyName,
			"operator":     operator,
			"value":        value,
		})
	}

	return map[string]any{
		"logicalOperator": logicalOperator,
		"conditions":      conditions,
	}, true
}

func buildFallbackFilterGroup(prompt string, properties []aiFilterProperty) map[string]any {
	group := map[string]any{
		"logicalOperator": "AND",
		"conditions":      []any{},
	}
	if len(properties) == 0 {
		return group
	}

	lowerPrompt := strings.ToLower(prompt)
	selected := properties[0]
	for _, property := range properties {
		name := strings.ToLower(property.Name)
		label := strings.ToLower(property.Label)
		if name != "" && strings.Contains(lowerPrompt, name) {
			selected = property
			break
		}
		if label != "" && strings.Contains(lowerPrompt, label) {
			selected = property
			break
		}
	}

	operator := firstOrDefault(normalizeOperators(selected.Operators), "=")
	value := prompt
	switch selected.Type {
	case "number":
		numberRegex := regexp.MustCompile(`-?\d+(\.\d+)?`)
		if match := numberRegex.FindString(prompt); match != "" {
			if parsed, err := strconv.ParseFloat(match, 64); err == nil {
				value = fmt.Sprintf("%v", parsed)
			}
		} else {
			value = "0"
		}
	case "boolean":
		if strings.Contains(lowerPrompt, "false") || strings.Contains(lowerPrompt, "not") {
			value = "false"
		} else {
			value = "true"
		}
	}

	group["conditions"] = []any{
		map[string]any{
			"propertyName": selected.Name,
			"operator":     operator,
			"value":        coerceFilterValue(value, selected),
		},
	}
	return group
}

func normalizeOperators(operators []any) []string {
	result := make([]string, 0, len(operators))
	for _, operator := range operators {
		switch value := operator.(type) {
		case string:
			value = strings.TrimSpace(value)
			if value != "" {
				result = append(result, value)
			}
		case map[string]any:
			candidate := strings.TrimSpace(stringFromAny(value["value"]))
			if candidate == "" {
				candidate = strings.TrimSpace(stringFromAny(value["label"]))
			}
			if candidate != "" {
				result = append(result, candidate)
			}
		}
	}
	if len(result) == 0 {
		result = append(result, "=")
	}
	return result
}

func normalizeOptions(options []any) []string {
	result := make([]string, 0, len(options))
	for _, option := range options {
		switch value := option.(type) {
		case string:
			value = strings.TrimSpace(value)
			if value != "" {
				result = append(result, value)
			}
		case map[string]any:
			candidate := strings.TrimSpace(stringFromAny(value["value"]))
			if candidate == "" {
				candidate = strings.TrimSpace(stringFromAny(value["label"]))
			}
			if candidate != "" {
				result = append(result, candidate)
			}
		}
	}
	return result
}

func coerceFilterValue(value any, property aiFilterProperty) any {
	options := normalizeOptions(property.Options)
	switch property.Type {
	case "number":
		switch typed := value.(type) {
		case float64:
			return typed
		case int:
			return typed
		case string:
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
				return parsed
			}
		}
		return 0
	case "boolean":
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			typed = strings.TrimSpace(strings.ToLower(typed))
			if typed == "true" || typed == "1" || typed == "yes" {
				return true
			}
			if typed == "false" || typed == "0" || typed == "no" {
				return false
			}
		}
		return false
	default:
		text := strings.TrimSpace(stringFromAny(value))
		if text == "" {
			text = firstOrDefault(options, "")
		}
		if len(options) > 0 && text != "" && !containsString(options, text) {
			// keep a valid option when available
			text = options[0]
		}
		return text
	}
}

func firstOrDefault(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return values[0]
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func classifyFeedbackConversation(text string) string {
	switch {
	case containsAny(text, "rls", "policy", "row level security"):
		return "rls_policies"
	case containsAny(text, "edge function", "serverless", "function", "deno"):
		return "edge_functions"
	case containsAny(text, "index", "slow", "performance", "optimiz"):
		return "database_optimization"
	case containsAny(text, "error", "failed", "exception", "bug", "not working"):
		return "debugging"
	case containsAny(text, "schema", "table", "column", "relationship", "migration"):
		return "schema_design"
	case containsAny(text, "sql", "query", "select", "insert", "update", "delete"):
		return "sql_generation"
	case containsAny(text, "how", "what", "help"):
		return "general_help"
	default:
		return "other"
	}
}

func classifyFeedbackPrompt(prompt string) string {
	text := strings.ToLower(prompt)
	if containsAny(text,
		"bug", "error", "failed", "not working", "cannot", "can't", "billing", "charged", "recover",
		"broken", "help", "issue", "unable",
	) {
		return "support"
	}
	if containsAny(text,
		"feature", "request", "suggest", "please add", "could you", "would be nice", "improve",
	) {
		return "feedback"
	}
	return "unknown"
}

func containsAny(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func splitTextChunks(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = 200
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return []string{""}
	}
	chunks := make([]string, 0, (len(runes)/maxRunes)+1)
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func extractLatestUserPrompt(messages []aiUIMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "user") {
			if text := extractUIMessageText(messages[i]); strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	return ""
}

func isResetRequest(prompt string) bool {
	prompt = strings.ToLower(prompt)
	return containsAny(prompt, "reset", "start over", "clear all", "wipe")
}

func (api *API) generateOnboardingSQL(ctx context.Context, model string, prompt string) string {
	answer, _, _, errMsg := api.generateOpenAIText(ctx, model, []openAIChatMessage{
		{
			Role: "system",
			Content: "You are a Postgres schema designer. Return SQL only. " +
				"Use id bigint primary key generated always as identity. " +
				"Use text columns by default. Keep output concise and runnable.",
		},
		{
			Role:    "user",
			Content: prompt,
		},
	})
	if errMsg != "" {
		return fallbackOnboardingSQL()
	}

	sql := cleanModelTextOutput(answer)
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return fallbackOnboardingSQL()
	}
	return sql
}

func fallbackOnboardingSQL() string {
	return strings.TrimSpace(`
create table if not exists public.profiles (
  id uuid primary key references auth.users(id) on delete cascade,
  full_name text,
  created_at timestamp with time zone default now()
);

create table if not exists public.todos (
  id bigint primary key generated always as identity,
  user_id uuid not null references auth.users(id) on delete cascade,
  title text not null,
  is_completed boolean not null default false,
  created_at timestamp with time zone default now()
);

alter table public.profiles enable row level security;
alter table public.todos enable row level security;
`)
}

func inferServicesFromPrompt(prompt string) []map[string]string {
	type service struct {
		Name   string
		Reason string
	}

	lower := strings.ToLower(prompt)
	selected := []service{
		{Name: "Database", Reason: "Store your application's relational data."},
	}

	if containsAny(lower, "auth", "login", "user", "account", "sign in", "signup") {
		selected = append(selected, service{Name: "Auth", Reason: "Manage users and authentication flows."})
	}
	if containsAny(lower, "storage", "upload", "file", "image", "video", "bucket") {
		selected = append(selected, service{Name: "Storage", Reason: "Store and serve files from buckets."})
	}
	if containsAny(lower, "edge function", "function", "serverless", "webhook", "api") {
		selected = append(selected, service{Name: "Edge Function", Reason: "Run backend logic close to your data."})
	}
	if containsAny(lower, "cron", "schedule", "scheduled", "daily", "hourly", "weekly") {
		selected = append(selected, service{Name: "Cron", Reason: "Run scheduled jobs."})
	}
	if containsAny(lower, "queue", "job", "worker", "background") {
		selected = append(selected, service{Name: "Queues", Reason: "Process background jobs reliably."})
	}
	if containsAny(lower, "embedding", "vector", "semantic", "rag", "similarity", "search") {
		selected = append(selected, service{Name: "Vector", Reason: "Power semantic search and AI retrieval."})
	}

	unique := make([]map[string]string, 0, len(selected))
	seen := map[string]struct{}{}
	for _, service := range selected {
		if _, ok := seen[service.Name]; ok {
			continue
		}
		seen[service.Name] = struct{}{}
		unique = append(unique, map[string]string{
			"name":   service.Name,
			"reason": service.Reason,
		})
	}

	return unique
}

func inferProjectTitle(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "New Project"
	}
	prompt = strings.ReplaceAll(prompt, "\n", " ")
	words := strings.Fields(prompt)
	if len(words) == 0 {
		return "New Project"
	}
	if len(words) > 6 {
		words = words[:6]
	}
	for i := range words {
		word := strings.ToLower(words[i])
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}
