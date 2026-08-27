package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/types"
)

type ProviderService struct {
	httpClient      *http.Client
	rawResponseLogs rawProviderResponseLogConfig
}

const upstreamProviderFailureMessage = "Upstream provider request failed"

func NewProviderService() *ProviderService {
	return &ProviderService{
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
		rawResponseLogs: loadRawProviderResponseLogConfig(),
	}
}

type rawProviderResponseLogConfig struct {
	enabled bool
	filters []string
}

func loadRawProviderResponseLogConfig() rawProviderResponseLogConfig {
	filters := make([]string, 0)
	for _, filter := range strings.Split(os.Getenv("LOG_RAW_PROVIDER_RESPONSE_FILTERS"), ",") {
		filter = strings.ToLower(strings.TrimSpace(filter))
		if filter != "" {
			filters = append(filters, filter)
		}
	}

	return rawProviderResponseLogConfig{
		enabled: envFlagEnabled("LOG_RAW_PROVIDER_RESPONSES"),
		filters: filters,
	}
}

func envFlagEnabled(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func requestTimeoutError(ctx context.Context) error {
	switch ctx.Err() {
	case context.DeadlineExceeded:
		return errors.NewTimeoutError("Request timeout", "request")
	case context.Canceled:
		return errors.NewTimeoutError("Request canceled", "request")
	default:
		return nil
	}
}

func requestTimeoutGatewayError(ctx context.Context) *types.GatewayError {
	if err := requestTimeoutError(ctx); err != nil {
		if timeoutErr, ok := err.(*errors.TimeoutError); ok {
			return &types.GatewayError{Type: "timeout_error", Code: "TIMEOUT", Message: timeoutErr.Message}
		}
	}
	return nil
}

func (s *ProviderService) CallProvider(
	baseURL, apiKey, model string,
	request types.ChatCompletionRequest,
	timeoutMs int,
	ctx context.Context,
	providerType string,
	auth types.ProviderAuth,
	requestID string,
) (*types.ChatCompletionResponse, error) {
	if providerType == "ollama" {
		return s.callOllamaProvider(baseURL, apiKey, model, request, ctx, auth, requestID)
	}
	if providerType == cloudflareProviderType {
		return s.callCloudflareProvider(baseURL, apiKey, model, request, ctx, auth)
	}
	if providerType == cohereProviderType {
		return s.callCohereProvider(baseURL, apiKey, model, request, ctx, auth, requestID)
	}

	reqBody, err := s.prepareRequest(request, model, baseURL, providerType, auth)
	if err != nil {
		return nil, err
	}

	// Build URL - standard OpenAI-compatible endpoint
	url := fmt.Sprintf("%s/chat/completions", baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if err := s.setAuth(ctx, req, apiKey, auth); err != nil {
		return nil, err
	}

	// Make request with timeout
	resp, err := s.httpClient.Do(req)
	if err != nil {
		if timeoutErr := requestTimeoutError(ctx); timeoutErr != nil {
			return nil, timeoutErr
		}
		return nil, wrapNetworkError(err, detectProvider(baseURL, providerType, auth), baseURL)
	}
	defer resp.Body.Close()

	return s.handleResponse(resp, baseURL, providerType, auth, request, model)
}

// StreamProviderChannel makes a streaming request to a provider using channels
func (s *ProviderService) StreamProviderChannel(
	baseURL, apiKey, model string,
	request types.ChatCompletionRequest,
	timeoutMs int,
	ctx context.Context,
	providerType string,
	auth types.ProviderAuth,
	requestID string,
) types.StreamResult {
	if providerType == "ollama" {
		return s.callOllamaStreamProvider(baseURL, apiKey, model, request, ctx, auth, requestID)
	}
	if providerType == cloudflareProviderType {
		chunks := make(chan *types.SSEChunk)
		errChan := make(chan *types.GatewayError, 1)
		close(chunks)
		errChan <- &types.GatewayError{Type: "provider_error", Code: "STREAMING_NOT_SUPPORTED", Message: "Cloudflare Workers AI native endpoint does not support streaming in this gateway"}
		close(errChan)
		return types.StreamResult{Chunks: chunks, Err: errChan}
	}
	if providerType == cohereProviderType {
		return s.callCohereStreamProvider(baseURL, apiKey, model, request, ctx, auth, requestID)
	}

	chunks := make(chan *types.SSEChunk)
	errChan := make(chan *types.GatewayError, 1)

	go func() {
		defer close(chunks)

		reqBody, err := s.prepareRequest(request, model, baseURL, providerType, auth)
		if err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "REQUEST_PREP_FAILED", Message: err.Error()}
			return
		}

		url := fmt.Sprintf("%s/chat/completions", baseURL)

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
		if err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "REQUEST_CREATE_FAILED", Message: err.Error()}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

		if err := s.setAuth(ctx, req, apiKey, auth); err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "AUTH_FAILED", Message: err.Error()}
			return
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			if timeoutErr := requestTimeoutGatewayError(ctx); timeoutErr != nil {
				errChan <- timeoutErr
			} else {
				wrappedErr := wrapNetworkError(err, detectProvider(baseURL, providerType, auth), baseURL)
				errChan <- &types.GatewayError{
					Type:    "network_error",
					Code:    "NETWORK_ERROR",
					Message: wrappedErr.Error(),
					Details: map[string]any{
						"network_type": classifyNetworkError(err),
						"provider":     detectProvider(baseURL, providerType, auth),
					},
				}
			}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errChan <- s.convertToGatewayError(s.handleErrorResponse(resp, baseURL, providerType, auth))
			return
		}

		provider := detectProvider(baseURL, providerType, auth)
		if err := s.parseSSEStreamChannel(ctx, resp.Body, chunks, provider, model, requestID); err != nil {
			if timeoutErr, ok := err.(*errors.TimeoutError); ok {
				errChan <- &types.GatewayError{
					Type:    "timeout_error",
					Code:    "TIMEOUT",
					Message: timeoutErr.Message,
					Details: map[string]any{"timeout_type": timeoutErr.TimeoutType},
				}
			} else if err == context.DeadlineExceeded || err == context.Canceled {
				errChan <- requestTimeoutGatewayError(ctx)
			} else {
				errChan <- &types.GatewayError{Type: "provider_error", Code: "STREAM_PARSE_FAILED", Message: err.Error()}
			}
		} else {
			errChan <- nil
		}
	}()

	return types.StreamResult{
		Chunks: chunks,
		Err:    errChan,
	}
}

func (s *ProviderService) setAuth(ctx context.Context, req *http.Request, apiKey string, auth types.ProviderAuth) error {
	switch auth.Type {
	case "bearer":
		if apiKey != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
		}
	case "header":
		if auth.HeaderName != "" && apiKey != "" {
			req.Header.Set(auth.HeaderName, apiKey)
		}
	}
	return nil
}

func (s *ProviderService) convertToGatewayError(err error) *types.GatewayError {
	if ge, ok := err.(*types.GatewayError); ok {
		return ge
	}

	switch e := err.(type) {
	case *errors.RateLimitError:
		code := "RATE_LIMITED"
		if e.LimitSubtype == "quota_exhausted" {
			code = "QUOTA_EXHAUSTED"
		} else if e.LimitSubtype == "overload" {
			code = "PROVIDER_OVERLOADED"
		}
		return &types.GatewayError{
			Type:    "rate_limit_error",
			Code:    code,
			Message: e.Message,
			Details: map[string]any{
				"retry_after":   e.RetryAfter,
				"limit_type":    e.LimitType,
				"limit_subtype": e.LimitSubtype,
				"headers":       e.Headers,
			},
		}
	case *errors.PaymentRequiredError:
		return &types.GatewayError{Type: "payment_error", Code: "PAYMENT_REQUIRED", Message: e.Message}
	case *errors.ValidationError:
		return &types.GatewayError{Type: "validation_error", Code: "VALIDATION_ERROR", Message: e.Message}
	case *errors.TimeoutError:
		return &types.GatewayError{Type: "timeout_error", Code: "TIMEOUT", Message: e.Message}
	case *errors.NetworkError:
		return &types.GatewayError{
			Type:    "network_error",
			Code:    "NETWORK_ERROR",
			Message: e.Message,
			Details: map[string]any{
				"network_type": e.NetworkType,
				"provider":     e.ProviderID,
				"base_url":     e.BaseURL,
			},
		}
	case *errors.ParseError:
		return &types.GatewayError{
			Type:    "parse_error",
			Code:    "PARSE_ERROR",
			Message: e.Message,
			Details: map[string]any{
				"parse_type":  e.ParseType,
				"provider":    e.ProviderID,
				"model":       e.Model,
				"raw_content": truncateString(e.RawContent, 200),
			},
		}
	case *errors.EmptyResponseError:
		return &types.GatewayError{
			Type:    "empty_response_error",
			Code:    "EMPTY_RESPONSE",
			Message: e.Message,
			Details: map[string]any{
				"provider":    e.ProviderID,
				"model":       e.Model,
				"status_code": e.StatusCode,
			},
		}
	case *errors.ProviderError:
		return &types.GatewayError{
			Type:    "provider_error",
			Code:    "PROVIDER_ERROR",
			Message: upstreamProviderFailureMessage,
			Details: map[string]any{"status_code": e.StatusCode},
		}
	default:
		return &types.GatewayError{Type: "provider_error", Code: "UNKNOWN", Message: upstreamProviderFailureMessage}
	}
}

func (s *ProviderService) prepareRequest(request types.ChatCompletionRequest, model, baseURL, providerType string, auth types.ProviderAuth) ([]byte, error) {
	request.Router = nil

	request.Model = model
	provider := detectProvider(baseURL, providerType, auth)
	request = normalizeRequestForProvider(request, provider, model)

	// thinking_blocks are a gateway extension; no upstream dialect accepts
	// them on requests, so strip them from replayed history. The copy keeps
	// the caller's message slice untouched.
	messages := make([]types.OpenAIMessage, len(request.Messages))
	copy(messages, request.Messages)
	for i := range messages {
		messages[i].ThinkingBlocks = nil
	}
	request.Messages = messages

	if request.ResponseFormat != nil && request.ResponseFormat.Type == "json_object" {
		request.Messages = ensureJSONKeyword(request.Messages)
	}

	return json.Marshal(request)
}

func isStrictJSONSchema(format *types.ResponseFormat) bool {
	return format != nil &&
		format.Type == "json_schema" &&
		format.JSONSchema != nil &&
		format.JSONSchema.Strict != nil &&
		*format.JSONSchema.Strict
}

func isResponsesBackedOpenCodeModel(model string) bool {
	switch model {
	case "muse-spark-1.2-contributor-free":
		return true
	default:
		return false
	}
}

func normalizeRequestForProvider(request types.ChatCompletionRequest, provider, model string) types.ChatCompletionRequest {
	request = normalizeStructuredOutputForProvider(request, provider, model)
	applyReasoningForProvider(&request, provider, request.ProviderCapabilities)

	switch provider {
	case "groq":
		if request.MaxCompletionTokens == nil && request.MaxTokens != nil {
			request.MaxCompletionTokens = request.MaxTokens
		}
		request.MaxTokens = nil
	case "opencode":
		// Muse Spark rides Zen's Responses API bridge: it rejects legacy
		// max_tokens with a 400 and only honors max_completion_tokens.
		if isResponsesBackedOpenCodeModel(model) {
			if request.MaxCompletionTokens == nil && request.MaxTokens != nil {
				request.MaxCompletionTokens = request.MaxTokens
			}
			request.MaxTokens = nil
			break
		}
		if request.MaxTokens == nil && request.MaxCompletionTokens != nil {
			request.MaxTokens = request.MaxCompletionTokens
		}
		request.MaxCompletionTokens = nil
	}

	switch provider {
	case "groq":
		request.Metadata = nil
		request.FrequencyPenalty = nil
		request.PresencePenalty = nil
	case "oci":
		if request.Temperature != nil && *request.Temperature == 0 {
			request.Temperature = nil
		}
	}

	return request
}

func normalizeStructuredOutputForProvider(request types.ChatCompletionRequest, provider, model string) types.ChatCompletionRequest {
	if request.ResponseFormat == nil {
		return request
	}
	structuredOutput := request.ProviderCapabilities.StructuredOutputs

	if request.ResponseFormat.Type == "json_schema" {
		if structuredOutput == "json_object" || (structuredOutput == "" && providerUsesNativeJSONObject(provider, model)) {
			request.Messages = prependStructuredOutputSchemaInstruction(request.Messages, request.ResponseFormat.JSONSchema)
			request.ResponseFormat = &types.ResponseFormat{Type: "json_object"}
			return request
		}

		format := request.ResponseFormat
		if providerRequiresStrictResponseSchemaDialect(provider, model) {
			format = strictDialectJSONSchemaFormat(format)
		}
		if structuredOutput != "json_schema_strict" && isStrictJSONSchema(format) {
			format = nonStrictJSONSchemaFormat(format)
		}
		request.ResponseFormat = format
		return request
	}

	if request.ResponseFormat.Type != "json_object" {
		return request
	}

	if structuredOutput == "json_object" || providerUsesNativeJSONObject(provider, model) {
		return request
	}
	if structuredOutput == "json_schema" || structuredOutput == "json_schema_strict" {
		// Continue below and express json_object through a permissive schema.
	} else if !providerUsesJSONSchemaForJSONObject(provider, model) {
		return request
	}

	request.ResponseFormat = &types.ResponseFormat{
		Type: "json_schema",
		JSONSchema: &types.JSONSchema{
			Name:        "response",
			Description: "JSON object response",
			Schema:      json.RawMessage(`{"type":"object","additionalProperties":true}`),
		},
	}
	return request
}

func prependStructuredOutputSchemaInstruction(messages []types.OpenAIMessage, schema *types.JSONSchema) []types.OpenAIMessage {
	if schema == nil || len(schema.Schema) == 0 {
		return messages
	}

	instruction := "Return only a valid JSON object that matches this JSON Schema. Do not include markdown fences or prose."
	if schema.Name != "" {
		instruction += " Schema name: " + schema.Name + "."
	}
	if schema.Description != "" {
		instruction += " Description: " + schema.Description + "."
	}
	instruction += "\nSchema:\n" + string(schema.Schema)

	withInstruction := make([]types.OpenAIMessage, 0, len(messages)+1)
	withInstruction = append(withInstruction, types.OpenAIMessage{Role: "system", Content: instruction})
	withInstruction = append(withInstruction, messages...)
	return withInstruction
}

func nonStrictJSONSchemaFormat(format *types.ResponseFormat) *types.ResponseFormat {
	if format == nil || format.JSONSchema == nil {
		return format
	}
	schema := *format.JSONSchema
	schema.Strict = nil
	return &types.ResponseFormat{Type: format.Type, JSONSchema: &schema}
}

func strictDialectJSONSchemaFormat(format *types.ResponseFormat) *types.ResponseFormat {
	if format == nil || format.JSONSchema == nil || len(format.JSONSchema.Schema) == 0 {
		return format
	}

	schema := *format.JSONSchema
	schema.Schema = normalizeJSONSchemaForStrictDialect(schema.Schema)
	return &types.ResponseFormat{Type: format.Type, JSONSchema: &schema}
}

func normalizeJSONSchemaForStrictDialect(raw json.RawMessage) json.RawMessage {
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return raw
	}

	normalized := normalizeStrictDialectSchemaNode(schema)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return raw
	}
	return encoded
}

func normalizeStrictDialectSchemaNode(node any) any {
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			typed[key] = normalizeStrictDialectSchemaNode(value)
		}

		props, ok := typed["properties"].(map[string]any)
		if !ok {
			return typed
		}

		typed["additionalProperties"] = false
		if len(props) == 0 {
			return typed
		}

		required := make(map[string]struct{}, len(props))
		if existing, ok := typed["required"].([]any); ok {
			for _, item := range existing {
				if value, ok := item.(string); ok {
					required[value] = struct{}{}
				}
			}
		}
		for name := range props {
			required[name] = struct{}{}
		}

		names := make([]string, 0, len(required))
		for name := range required {
			names = append(names, name)
		}
		sort.Strings(names)
		requiredList := make([]any, 0, len(names))
		for _, name := range names {
			requiredList = append(requiredList, name)
		}
		typed["required"] = requiredList
		return typed
	case []any:
		for i, value := range typed {
			typed[i] = normalizeStrictDialectSchemaNode(value)
		}
		return typed
	default:
		return typed
	}
}

func providerRequiresStrictResponseSchemaDialect(provider, model string) bool {
	switch provider {
	case "groq":
		return true
	case "oci":
		return strings.HasPrefix(model, "openai.")
	default:
		return false
	}
}

func providerUsesNativeJSONObject(provider, model string) bool {
	if provider == "gemini" {
		return true
	}
	if provider == "oci" && strings.HasPrefix(model, "google.gemini-") {
		return true
	}
	return false
}

func providerUsesJSONSchemaForJSONObject(provider, model string) bool {
	switch provider {
	case "groq", "nim", "zai", "kilo":
		return true
	case "oci":
		return !strings.HasPrefix(model, "google.gemini-")
	default:
		return false
	}
}

func detectProvider(baseURL, providerType string, auth types.ProviderAuth) string {
	if providerType == cloudflareProviderType {
		return cloudflareProviderID
	}

	switch auth.Env {
	case "GROQ_API_KEY":
		return "groq"
	case "NIM_API_KEY":
		return "nim"
	case "OLLAMA_API_KEY":
		return "ollama"
	case "KILO_API_KEY":
		return "kilo"
	case "OPENCODE_ZEN_API_KEY":
		return "opencode"
	case cloudflareAPITokenEnv:
		return cloudflareProviderID
	case "OCI_API_KEY":
		return "oci"
	case "BAI_API_KEY":
		return "bai"
	case "INFERX_API_KEY":
		return "inferx"
	case "GMI_API_KEY":
		return "gmi"
	case "ORCAROUTER_API_KEY":
		return "orca"
	case "AI_GATEWAY_API_KEY":
		return "vercel"
	case "EMPERO_API_KEY":
		return "empero"
	}

	switch {
	case strings.Contains(baseURL, "api.groq.com"):
		return "groq"
	case strings.Contains(baseURL, "integrate.api.nvidia.com"):
		return "nim"
	case strings.Contains(baseURL, "api.kilo.ai"):
		return "kilo"
	case strings.Contains(baseURL, "opencode.ai"):
		return "opencode"
	case strings.Contains(baseURL, "api.cloudflare.com"):
		return cloudflareProviderID
	case strings.Contains(baseURL, "ollama.com"):
		return "ollama"
	case strings.Contains(baseURL, "api.b.ai"):
		return "bai"
	case strings.Contains(baseURL, "inferx.net"):
		return "inferx"
	case strings.Contains(baseURL, "gmi-serving.com"):
		return "gmi"
	case strings.Contains(baseURL, "orcarouter.ai"):
		return "orca"
	case strings.Contains(baseURL, "ai-gateway.vercel.sh"):
		return "vercel"
	case strings.Contains(baseURL, "free.empero.org"):
		return "empero"
	default:
		return ""
	}
}

// classifyNetworkError determines the type of network error
func classifyNetworkError(err error) string {
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "timeout") {
		return "timeout"
	}
	if strings.Contains(errStr, "connection") {
		return "connection"
	}
	if strings.Contains(errStr, "dns") {
		return "dns"
	}
	if strings.Contains(errStr, "tls") {
		return "tls"
	}
	return "unknown"
}

// wrapNetworkError wraps a network error with provider context
func wrapNetworkError(err error, providerID, baseURL string) error {
	if err == nil {
		return nil
	}

	// Don't wrap if already a network error
	if _, ok := err.(*errors.NetworkError); ok {
		return err
	}

	networkType := classifyNetworkError(err)
	return errors.NewNetworkError(
		fmt.Sprintf("Network error calling %s", baseURL),
		networkType,
		providerID,
		baseURL,
		err,
	)
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 0 {
		return ""
	}
	return s[:maxLen] + "..."
}

func (s *ProviderService) shouldLogRawProviderResponse(provider, model string) bool {
	if !s.rawResponseLogs.enabled {
		return false
	}

	if len(s.rawResponseLogs.filters) == 0 {
		return true
	}

	candidates := []string{strings.ToLower(provider), strings.ToLower(model)}
	if provider != "" && model != "" {
		candidates = append(candidates, strings.ToLower(provider)+"/"+strings.ToLower(model))
	}

	for _, filter := range s.rawResponseLogs.filters {
		if matchesRawProviderResponseFilter(filter, candidates) {
			return true
		}
	}

	return false
}

func matchesRawProviderResponseFilter(filter string, candidates []string) bool {
	if filter == "" {
		return false
	}

	prefix := filter
	wildcard := strings.HasSuffix(filter, "*")
	if wildcard {
		prefix = strings.TrimSuffix(filter, "*")
	}

	for _, candidate := range candidates {
		if wildcard {
			if strings.HasPrefix(candidate, prefix) {
				return true
			}
			continue
		}

		if candidate == filter {
			return true
		}
	}

	return false
}

func (s *ProviderService) logRawProviderResponseBody(provider, model string, statusCode int, body []byte) {
	if !s.shouldLogRawProviderResponse(provider, model) {
		return
	}

	logger.Info().
		Str("type", "http").
		Str("event", "provider.response_body_raw").
		Str("provider", provider).
		Str("model", model).
		Int("status_code", statusCode).
		Str("body", string(body)).
		Msg("Logged raw upstream response body")
}

func (s *ProviderService) logRawProviderSSEData(provider, model, data string) {
	if !s.shouldLogRawProviderResponse(provider, model) {
		return
	}

	logger.Info().
		Str("type", "http").
		Str("event", "provider.sse_data_raw").
		Str("provider", provider).
		Str("model", model).
		Str("data", data).
		Msg("Logged raw upstream SSE payload")
}

func schemaContainsRecursiveRef(raw json.RawMessage) bool {
	return strings.Contains(string(raw), `"$ref":"#`) || strings.Contains(string(raw), `"$ref": "#`)
}

func ensureJSONKeyword(messages []types.OpenAIMessage) []types.OpenAIMessage {
	for _, msg := range messages {
		if msg.Role == "system" || msg.Role == "user" || msg.Role == "assistant" {
			content := extractStringContent(msg.Content)
			if strings.Contains(strings.ToLower(content), "json") {
				return messages
			}
		}
	}

	jsonHint := types.OpenAIMessage{
		Role:    "system",
		Content: "Respond in valid JSON format.",
	}
	return append([]types.OpenAIMessage{jsonHint}, messages...)
}

func extractStringContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var result string
		for _, part := range v {
			if m, ok := part.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					result += text + " "
				}
			}
		}
		return result
	default:
		return fmt.Sprintf("%v", content)
	}
}

func (s *ProviderService) handleResponse(resp *http.Response, baseURL, providerType string, auth types.ProviderAuth, _ types.ChatCompletionRequest, model string) (*types.ChatCompletionResponse, error) {
	provider := detectProvider(baseURL, providerType, auth)
	headers := flattenHeaders(resp.Header)
	if resp.StatusCode == http.StatusTooManyRequests {
		body := readProviderErrorBody(resp.Body)
		details := parseRateLimitDetails(provider, resp.Header, body)
		err := errors.NewRateLimitErrorWithSubtype(
			normalizeProviderErrorMessage(provider, resp.StatusCode, body),
			details.RetryAfter, details.LimitType, details.LimitSubtype, headers,
		)
		err.RetryAfterProvided = details.RetryAfterProvided
		err.ResetAtUnixMs = details.ResetAtUnixMs
		applyProviderQuotaSync(err, resp.Header, body)
		return nil, err
	}

	if resp.StatusCode == http.StatusPaymentRequired {
		body := readProviderErrorBody(resp.Body)
		return nil, &errors.PaymentRequiredError{ProviderError: errors.ProviderError{Message: normalizeProviderErrorMessage(provider, resp.StatusCode, body), StatusCode: 402, IsRetryable: false, Headers: headers}}
	}

	if resp.StatusCode >= 500 {
		body := readProviderErrorBody(resp.Body)
		return nil, &errors.ProviderError{
			Message:     normalizeProviderErrorMessage(provider, resp.StatusCode, body),
			StatusCode:  resp.StatusCode,
			IsRetryable: true,
			Headers:     headers,
		}
	}

	if resp.StatusCode != http.StatusOK {
		body := readProviderErrorBody(resp.Body)
		return nil, classifyProviderHTTPError(provider, resp.StatusCode, headers, body)
	}

	// Parse response
	body, err := readBoundedResponseBody(resp.Body, maxProviderResponseBodyBytes)
	if err != nil {
		return nil, err
	}
	s.logRawProviderResponseBody(provider, model, resp.StatusCode, body)

	// Check for empty response body on successful requests
	if resp.StatusCode == http.StatusOK && len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.NewEmptyResponseError(provider, model, resp.StatusCode)
	}

	var response types.ChatCompletionResponse
	if provider == cloudflareProviderID {
		return parseCloudflareRunResponse(body, model)
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.NewParseError(
			fmt.Sprintf("Failed to parse response from %s/%s", provider, model),
			"json",
			provider,
			model,
			truncateString(string(body), 500),
			err,
		)
	}

	return &response, nil
}

func (s *ProviderService) handleErrorResponse(resp *http.Response, baseURL, providerType string, auth types.ProviderAuth) error {
	body := readProviderErrorBody(resp.Body)
	provider := detectProvider(baseURL, providerType, auth)
	headers := flattenHeaders(resp.Header)

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		details := parseRateLimitDetails(provider, resp.Header, body)
		err := errors.NewRateLimitErrorWithSubtype(
			normalizeProviderErrorMessage(provider, resp.StatusCode, body),
			details.RetryAfter, details.LimitType, details.LimitSubtype, headers,
		)
		err.RetryAfterProvided = details.RetryAfterProvided
		err.ResetAtUnixMs = details.ResetAtUnixMs
		applyProviderQuotaSync(err, resp.Header, body)
		return err
	case http.StatusPaymentRequired:
		return &errors.PaymentRequiredError{ProviderError: errors.ProviderError{Message: normalizeProviderErrorMessage(provider, resp.StatusCode, body), StatusCode: 402, IsRetryable: false, Headers: headers}}
	default:
		return classifyProviderHTTPError(provider, resp.StatusCode, headers, body)
	}
}

func classifyProviderHTTPError(provider string, statusCode int, headers map[string]string, body []byte) error {
	message := normalizeProviderErrorMessage(provider, statusCode, body)
	if isProviderFailoverHTTPError(statusCode, message) {
		return &errors.ProviderError{
			Message:     message,
			StatusCode:  statusCode,
			IsRetryable: false,
			Headers:     headers,
		}
	}
	if isValidationStatus(statusCode) {
		validationErr := errors.NewValidationError(message, nil)
		validationErr.StatusCode = statusCode
		return validationErr
	}
	return &errors.ProviderError{
		Message:     message,
		StatusCode:  statusCode,
		IsRetryable: statusCode >= 500,
		Headers:     headers,
	}
}

func isProviderFailoverHTTPError(statusCode int, message string) bool {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "degraded function cannot be invoked") {
		return true
	}
	if strings.Contains(lower, "was retired") {
		return true
	}
	return isProviderSchemaDialectRejection(statusCode, lower) || isProviderUnsupportedResponseFormat(statusCode, lower)
}

func isProviderUnsupportedResponseFormat(statusCode int, lowerMessage string) bool {
	if !isValidationStatus(statusCode) {
		return false
	}
	if !strings.Contains(lowerMessage, "response format") && !strings.Contains(lowerMessage, "response_format") {
		return false
	}
	if strings.Contains(lowerMessage, "unavailable") ||
		strings.Contains(lowerMessage, "not available") ||
		strings.Contains(lowerMessage, "disabled") ||
		strings.Contains(lowerMessage, "not supported") ||
		strings.Contains(lowerMessage, "unsupported") ||
		strings.Contains(lowerMessage, "does not support") ||
		strings.Contains(lowerMessage, "not support") {
		return true
	}
	return false
}

func isProviderSchemaDialectRejection(statusCode int, lowerMessage string) bool {
	if !isValidationStatus(statusCode) {
		return false
	}
	if strings.Contains(lowerMessage, "unsupported json schema feature for gemini") {
		return true
	}
	if strings.Contains(lowerMessage, "failed_generation") && strings.Contains(lowerMessage, "failed to validate json") {
		return true
	}
	if !strings.Contains(lowerMessage, "response_format") {
		return false
	}
	if !strings.Contains(lowerMessage, "json schema") && !strings.Contains(lowerMessage, "json_schema") {
		return false
	}
	return strings.Contains(lowerMessage, "`required` is required") ||
		strings.Contains(lowerMessage, "must be listed in `required`") ||
		strings.Contains(lowerMessage, "unsupported json schema feature") ||
		strings.Contains(lowerMessage, "unsupported schema feature") ||
		strings.Contains(lowerMessage, "anyof") ||
		strings.Contains(lowerMessage, "oneof") ||
		strings.Contains(lowerMessage, "allof") ||
		strings.Contains(lowerMessage, "enum") ||
		strings.Contains(lowerMessage, "additionalproperties") ||
		strings.Contains(lowerMessage, "additional properties")
}

func flattenHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	flat := make(map[string]string, len(headers))
	for key, values := range headers {
		flat[key] = strings.Join(values, ",")
	}
	return flat
}

type rateLimitParseResult struct {
	RetryAfter         int
	RetryAfterProvided bool
	LimitType          string
	LimitSubtype       string
	ResetAtUnixMs      int64
}

func parseRateLimitDetails(provider string, headers http.Header, body []byte) rateLimitParseResult {
	retryAfter := 60
	retryAfterProvided := false
	if value := headers.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			retryAfter = seconds
			retryAfterProvided = true
		} else if retryAt, err := http.ParseTime(value); err == nil {
			seconds := int(time.Until(retryAt).Seconds())
			if seconds > 0 {
				retryAfter = seconds
				retryAfterProvided = true
			}
		}
	}
	if !retryAfterProvided {
		if delay := parseBodyRetryDelay(body); delay > 0 {
			retryAfter = delay
			retryAfterProvided = true
		}
	}

	bodyUpper := strings.ToUpper(string(body))
	limitSubtype := "rate_limit"
	if quota, ok := parseProviderQuotaDetails(body); ok {
		return rateLimitParseResult{
			RetryAfter:         retryAfter,
			RetryAfterProvided: retryAfterProvided,
			LimitType:          quota.LimitType,
			LimitSubtype:       "quota_exhausted",
			ResetAtUnixMs:      parseResetAtUnixMs(headers, body),
		}
	}

	switch provider {
	case "orca":
		if strings.Contains(bodyUpper, "FREE_RATE_LIMITED") || strings.Contains(bodyUpper, "PROMPT") || !retryAfterProvided {
			if !retryAfterProvided {
				return rateLimitParseResult{
					RetryAfter:         0,
					RetryAfterProvided: false,
					LimitType:          "prompt_cap",
					LimitSubtype:       "prompt_too_large",
					ResetAtUnixMs:      0,
				}
			}
		}
		return rateLimitParseResult{
			RetryAfter:         retryAfter,
			RetryAfterProvided: true,
			LimitType:          "rpm",
			LimitSubtype:       "rate_limit",
			ResetAtUnixMs:      parseResetAtUnixMs(headers, body),
		}
	case "groq":
		if limit := headers.Get("X-RateLimit-Limit-Requests"); limit != "" {
			return rateLimitParseResult{retryAfter, retryAfterProvided, "rpd", limitSubtype, parseResetAtUnixMs(headers, body)}
		}
		if limit := headers.Get("X-RateLimit-Limit-Tokens"); limit != "" {
			return rateLimitParseResult{retryAfter, retryAfterProvided, "tpm", limitSubtype, parseResetAtUnixMs(headers, body)}
		}
	case cloudflareProviderID:
		if strings.Contains(bodyUpper, "USED UP YOUR DAILY FREE ALLOCATION") ||
			strings.Contains(bodyUpper, "DAILY FREE ALLOCATION OF 10,000 NEURONS") ||
			(strings.Contains(bodyUpper, "10,000 NEURONS") && strings.Contains(bodyUpper, "FREE ALLOCATION")) {
			return rateLimitParseResult{retryAfter, retryAfterProvided, "daily_neurons", "quota_exhausted", parseResetAtUnixMs(headers, body)}
		}
	}

	// Day-scale request limits: benching for seconds is wrong when the window
	// resets daily (e.g. kilo "limit_rpd/… Daily limit reached", OpenRouter
	// "free-models-per-day-stealth"). Google PerDay quotas are handled above
	// via structured quotaId details.
	if isDayScaleRequestBody(bodyUpper) {
		subtype := "rate_limit"
		if strings.Contains(bodyUpper, "LIMIT REACHED") || strings.Contains(bodyUpper, "QUOTA") {
			subtype = "quota_exhausted"
		}
		return rateLimitParseResult{
			RetryAfter:         retryAfter,
			RetryAfterProvided: retryAfterProvided,
			LimitType:          "rpd",
			LimitSubtype:       subtype,
			ResetAtUnixMs:      parseResetAtUnixMs(headers, body),
		}
	}

	// Parse body for quota/billing exhaustion signals across all providers
	if strings.Contains(bodyUpper, "QUOTA_EXCEEDED") ||
		strings.Contains(bodyUpper, "RESOURCE_EXHAUSTED") ||
		strings.Contains(bodyUpper, "MONTHLY") ||
		strings.Contains(bodyUpper, "BILLING") ||
		strings.Contains(bodyUpper, "INSUFFICIENT_QUOTA") ||
		strings.Contains(bodyUpper, "CREDITS") {
		return rateLimitParseResult{retryAfter, retryAfterProvided, "quota", "quota_exhausted", parseResetAtUnixMs(headers, body)}
	}

	// Detect overload signals
	if strings.Contains(bodyUpper, "OVERLOAD") ||
		strings.Contains(bodyUpper, "SLOW_DOWN") ||
		strings.Contains(bodyUpper, "CAPACITY") {
		return rateLimitParseResult{retryAfter, retryAfterProvided, "rpm", "overload", 0}
	}

	return rateLimitParseResult{retryAfter, retryAfterProvided, "rpm", limitSubtype, 0}
}

// isDayScaleRequestBody detects free-text markers of daily request windows.
func isDayScaleRequestBody(bodyUpper string) bool {
	return strings.Contains(bodyUpper, "PER-DAY") ||
		strings.Contains(bodyUpper, "PER DAY") ||
		strings.Contains(bodyUpper, "PERDAY") ||
		strings.Contains(bodyUpper, "LIMIT_RPD") ||
		strings.Contains(bodyUpper, "FREE-MODELS-PER-DAY") ||
		strings.Contains(bodyUpper, "DAILY LIMIT") ||
		strings.Contains(bodyUpper, "DAILY QUOTA")
}

// parseResetAtUnixMs extracts an absolute window-reset timestamp when the
// provider supplies one. OpenRouter sends X-RateLimit-Reset as epoch
// milliseconds both as a response header and echoed in error.metadata.headers.
func parseResetAtUnixMs(headers http.Header, body []byte) int64 {
	if value := headers.Get("X-RateLimit-Reset"); value != "" {
		if ms, ok := normalizeResetEpochMs(value); ok {
			return ms
		}
	}
	return parseBodyMetadataResetMs(body)
}

// normalizeResetEpochMs accepts epoch milliseconds (OpenRouter), epoch seconds,
// or HTTP-date reset values and clamps them to a plausible future horizon.
func normalizeResetEpochMs(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if ms < 1_000_000_000_000 { // magnitudes below ~2001 in ms are epoch seconds
			ms *= 1000
		}
		return clampResetEpochMs(ms)
	}
	if t, err := http.ParseTime(raw); err == nil {
		return clampResetEpochMs(t.UnixMilli())
	}
	return 0, false
}

func clampResetEpochMs(ms int64) (int64, bool) {
	now := time.Now().UnixMilli()
	maxHorizon := now + int64(48*time.Hour/time.Millisecond)
	if ms <= now || ms > maxHorizon {
		return 0, false
	}
	return ms, true
}

func parseBodyMetadataResetMs(body []byte) int64 {
	if value, ok := errorMetadataHeader(body, "X-RateLimit-Reset"); ok {
		if ms, ok := normalizeResetEpochMs(value); ok {
			return ms
		}
	}
	return 0
}

// errorMetadataHeader reads a header echo out of an error envelope such as
// {"error":{"metadata":{"headers":{"X-RateLimit-Reset":"1787443200000"}}}}.
func errorMetadataHeader(body []byte, wanted string) (string, bool) {
	var payload struct {
		Error struct {
			Metadata struct {
				Headers map[string]string `json:"headers"`
			} `json:"metadata"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	for name, value := range payload.Error.Metadata.Headers {
		if strings.EqualFold(name, wanted) {
			return value, true
		}
	}
	return "", false
}

// applyProviderQuotaSync mirrors numeric provider-side limits onto the error so
// handleRateLimitFailure can sync local tracking windows. Structured Google
// quota violations win; otherwise plain X-RateLimit-Limit headers feed rpd syncs.
func applyProviderQuotaSync(err *errors.RateLimitError, headers http.Header, body []byte) {
	if quota, ok := parseProviderQuotaDetails(body); ok {
		err.LimitType = quota.LimitType
		err.LimitSubtype = "quota_exhausted"
		err.IsRetryable = false
		err.ProviderQuotaLimit = quota.Limit
		err.ProviderQuotaID = quota.ID
		return
	}
	if err.LimitType != "rpd" && err.LimitType != "tpd" {
		return
	}
	limit := parseHeaderInt(headers, "X-RateLimit-Limit")
	if limit <= 0 {
		if value, ok := errorMetadataHeader(body, "X-RateLimit-Limit"); ok {
			limit = parseHeaderInt(http.Header{"X-RateLimit-Limit": []string{value}}, "X-RateLimit-Limit")
		}
	}
	if limit > 0 {
		err.ProviderQuotaLimit = limit
	}
}

type providerQuotaDetails struct {
	LimitType string
	Limit     int
	ID        string
}

type errorWithDetailsPayload struct {
	Error struct {
		Details []json.RawMessage `json:"details"`
	} `json:"error"`
}

func extractErrorDetails(body []byte) []json.RawMessage {
	var single errorWithDetailsPayload
	if err := json.Unmarshal(body, &single); err == nil && len(single.Error.Details) > 0 {
		return single.Error.Details
	}
	var arr []errorWithDetailsPayload
	if err := json.Unmarshal(body, &arr); err == nil {
		var all []json.RawMessage
		for _, item := range arr {
			all = append(all, item.Error.Details...)
		}
		return all
	}
	return nil
}

// parseBodyRetryDelay extracts a provider-supplied retry delay from an error
// response body, e.g. Google's google.rpc.RetryInfo detail:
// {"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"37s"}]}}
// or array formatted errors:
// [{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"13s"}]}}]
func parseBodyRetryDelay(body []byte) int {
	details := extractErrorDetails(body)
	if len(details) == 0 {
		return 0
	}

	for _, rawDetail := range details {
		var detail struct {
			RetryDelay string `json:"retryDelay"`
		}
		if err := json.Unmarshal(rawDetail, &detail); err != nil || detail.RetryDelay == "" {
			continue
		}
		if seconds := parseDurationSeconds(detail.RetryDelay); seconds > 0 {
			return seconds
		}
	}
	return 0
}

// parseDurationSeconds parses strings like "37s", "90s", or "1.5s" into whole seconds.
func parseDurationSeconds(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}
	seconds := int(d.Seconds())
	if d > 0 && seconds < 1 {
		return 1
	}
	return seconds
}

func parseProviderQuotaDetails(body []byte) (providerQuotaDetails, bool) {
	details := extractErrorDetails(body)
	if len(details) == 0 {
		return providerQuotaDetails{}, false
	}

	for _, rawDetail := range details {
		var detail map[string]any
		if err := json.Unmarshal(rawDetail, &detail); err != nil {
			continue
		}
		violations, ok := detail["violations"].([]any)
		if !ok {
			continue
		}
		for _, rawViolation := range violations {
			violation, ok := rawViolation.(map[string]any)
			if !ok {
				continue
			}
			quotaID, _ := violation["quotaId"].(string)
			quotaMetric, _ := violation["quotaMetric"].(string)
			limitType := quotaLimitType(quotaID, quotaMetric)
			limit := quotaValueInt(violation["quotaValue"])
			if limitType == "" || limit <= 0 {
				continue
			}
			return providerQuotaDetails{LimitType: limitType, Limit: limit, ID: quotaID}, true
		}
	}

	return providerQuotaDetails{}, false
}

func quotaLimitType(quotaID, quotaMetric string) string {
	combined := strings.ToLower(quotaID + " " + quotaMetric)
	switch {
	case strings.Contains(combined, "perday") || strings.Contains(combined, "per_day") || strings.Contains(combined, "requestsperday"):
		return "rpd"
	case strings.Contains(combined, "perminute") || strings.Contains(combined, "per_minute") || strings.Contains(combined, "requestsperminute"):
		if strings.Contains(combined, "token") {
			return "tpm"
		}
		return "rpm"
	case strings.Contains(combined, "token") && strings.Contains(combined, "day"):
		return "tpd"
	case strings.Contains(combined, "request") && strings.Contains(combined, "day"):
		return "rpd"
	case strings.Contains(combined, "request") && strings.Contains(combined, "minute"):
		return "rpm"
	default:
		return ""
	}
}

func quotaValueInt(value any) int {
	switch typed := value.(type) {
	case string:
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return 0
		}
		return parsed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func normalizeProviderErrorMessage(provider string, statusCode int, body []byte) string {
	message := extractProviderErrorMessage(provider, body)
	if message == "" {
		trimmed := strings.TrimSpace(string(body))
		if trimmed == "" {
			return fmt.Sprintf("HTTP error %d", statusCode)
		}
		message = trimmed
	}
	return fmt.Sprintf("HTTP error %d: %s", statusCode, message)
}

func extractProviderErrorMessage(provider string, body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}

	var wrapped struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
		Message any    `json:"message"`
		Detail  any    `json:"detail"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
		Object  string `json:"object"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err == nil {
		if wrapped.Error.Message != "" {
			return wrapped.Error.Message
		}
		if len(wrapped.Errors) > 0 && wrapped.Errors[0].Message != "" {
			return wrapped.Errors[0].Message
		}
		switch msg := wrapped.Message.(type) {
		case string:
			return msg
		case map[string]any:
			if detail, ok := msg["detail"]; ok {
				return fmt.Sprintf("%v", detail)
			}
		}
		switch detail := wrapped.Detail.(type) {
		case string:
			return detail
		case map[string]any:
			return fmt.Sprintf("%v", detail)
		case []any:
			return fmt.Sprintf("%v", detail)
		}
	}

	return strings.TrimSpace(string(trimmed))
}

func isValidationStatus(statusCode int) bool {
	return statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity
}

func (s *ProviderService) parseSSEStreamChannel(ctx context.Context, body io.ReadCloser, chunks chan<- *types.SSEChunk, provider, model, requestID string) error {
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineCh := make(chan string)
	scanCtx, cancelScan := context.WithCancel(ctx)
	scanDone := make(chan struct{})
	var scanErr error

	go func() {
		defer close(scanDone)
		defer close(lineCh)
		for scanner.Scan() {
			select {
			case lineCh <- scanner.Text():
			case <-scanCtx.Done():
				return
			}
		}
		scanErr = scanner.Err()
	}()
	defer func() {
		cancelScan()
		_ = body.Close()
		<-scanDone
	}()

	inactivity := defaultStreamInactivityTimeout
	timer := time.NewTimer(inactivity)
	defer timer.Stop()

	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(inactivity)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-timer.C:
			return errors.NewTimeoutError("Inactivity timeout", "inactivity")

		case line, ok := <-lineCh:
			if !ok {
				<-scanDone
				if err := ctx.Err(); err != nil {
					return err
				}
				return scanErr
			}

			resetTimer()

			if line == "" {
				continue
			}

			if !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			s.logRawProviderSSEData(provider, model, data)

			if data == "[DONE]" {
				return nil
			}

			if streamErr := parseProviderSSEError(data); streamErr != nil {
				return streamErr
			}

			var chunk types.SSEChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				logger.Error().
					Str("type", "http").
					Str("event", "sse.parse_failed").
					Str("request_id", requestID).
					Str("provider", provider).
					Str("model", model).
					Err(err).
					Str("data", data).
					Msg("Failed to parse SSE chunk")
				continue
			}

			if shouldSkipChunk(chunk) {
				continue
			}

			select {
			case chunks <- &chunk:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func shouldSkipChunk(chunk types.SSEChunk) bool {
	if chunk.Usage != nil {
		return false
	}

	for _, choice := range chunk.Choices {
		if choice.Delta.Role != "" || choice.Delta.Content != nil || choice.Delta.Refusal != nil || len(choice.Delta.ToolCalls) > 0 || choice.FinishReason != nil {
			return false
		}
		// Reasoning-only deltas are client-visible payload; never skip them.
		if choice.Delta.ReasoningContent != nil || len(choice.Delta.ThinkingBlocks) > 0 {
			return false
		}
	}

	return true
}

func parseProviderSSEError(data string) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil || payload.Error.Message == "" {
		return nil
	}

	return &errors.ProviderError{
		Message:     payload.Error.Message,
		StatusCode:  500,
		IsRetryable: true,
	}
}
