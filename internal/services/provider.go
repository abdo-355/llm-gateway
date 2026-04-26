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

func (s *ProviderService) CallProvider(
	baseURL, apiKey, model string,
	request types.ChatCompletionRequest,
	timeoutMs int,
	ctx context.Context,
	providerType string,
	auth types.ProviderAuth,
) (*types.ChatCompletionResponse, error) {
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
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.NewTimeoutError("Request timeout", "request")
		}
		return nil, err
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
) types.StreamResult {
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
			if ctx.Err() == context.DeadlineExceeded {
				errChan <- &types.GatewayError{Type: "provider_error", Code: "TIMEOUT", Message: "Request timeout"}
			} else {
				errChan <- &types.GatewayError{Type: "provider_error", Code: "REQUEST_FAILED", Message: err.Error()}
			}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errChan <- s.convertToGatewayError(s.handleErrorResponse(resp, baseURL, providerType, auth))
			return
		}

		provider := detectProvider(baseURL, providerType, auth)
		if err := s.parseSSEStreamChannel(ctx, resp.Body, chunks, provider, model); err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "STREAM_PARSE_FAILED", Message: err.Error()}
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
		return &types.GatewayError{
			Type:    "rate_limit_error",
			Code:    "RATE_LIMITED",
			Message: e.Message,
			Details: map[string]any{
				"retry_after": e.RetryAfter,
				"limit_type":  e.LimitType,
				"headers":     e.Headers,
			},
		}
	case *errors.PaymentRequiredError:
		return &types.GatewayError{Type: "payment_error", Code: "PAYMENT_REQUIRED", Message: e.Message}
	case *errors.ValidationError:
		return &types.GatewayError{Type: "validation_error", Code: "VALIDATION_ERROR", Message: e.Message}
	case *errors.TimeoutError:
		return &types.GatewayError{Type: "timeout_error", Code: "TIMEOUT", Message: e.Message}
	case *errors.ProviderError:
		return &types.GatewayError{
			Type:    "provider_error",
			Code:    "PROVIDER_ERROR",
			Message: e.Message,
			Details: map[string]any{"headers": e.Headers, "status_code": e.StatusCode},
		}
	default:
		return &types.GatewayError{Type: "provider_error", Code: "UNKNOWN", Message: err.Error()}
	}
}

func (s *ProviderService) prepareRequest(request types.ChatCompletionRequest, model, baseURL, providerType string, auth types.ProviderAuth) ([]byte, error) {
	request.Router = nil

	request.Model = model
	provider := detectProvider(baseURL, providerType, auth)
	request = normalizeRequestForProvider(request, provider)
	if err := validateRequestForProvider(request, provider); err != nil {
		return nil, err
	}

	if request.ResponseFormat != nil && request.ResponseFormat.Type == "json_object" {
		request.Messages = ensureJSONKeyword(request.Messages)
	}

	return json.Marshal(request)
}

func validateRequestForProvider(request types.ChatCompletionRequest, provider string) error {
	if request.ResponseFormat == nil {
		return nil
	}

	if provider == "cerebras" && request.ResponseFormat.Type == "json_object" && request.Stream != nil && *request.Stream {
		return fmt.Errorf("cerebras does not support json_object with streaming")
	}

	if provider == "cerebras" && request.ResponseFormat.Type == "json_schema" && request.ResponseFormat.JSONSchema != nil {
		if !schemaObjectsDisallowAdditionalProperties(request.ResponseFormat.JSONSchema.Schema) {
			return fmt.Errorf("cerebras strict json_schema requires additionalProperties=false on every object")
		}
	}

	if provider == "vertex" && request.ResponseFormat.Type == "json_schema" && request.ResponseFormat.JSONSchema != nil {
		if schemaContainsRecursiveRef(request.ResponseFormat.JSONSchema.Schema) {
			return fmt.Errorf("vertex does not support recursive json_schema references")
		}
	}

	return nil
}

func normalizeRequestForProvider(request types.ChatCompletionRequest, provider string) types.ChatCompletionRequest {
	switch provider {
	case "groq", "cerebras":
		if request.MaxCompletionTokens == nil && request.MaxTokens != nil {
			request.MaxCompletionTokens = request.MaxTokens
		}
		request.MaxTokens = nil
	case "mistral":
		if request.MaxTokens == nil && request.MaxCompletionTokens != nil {
			request.MaxTokens = request.MaxCompletionTokens
		}
		request.MaxCompletionTokens = nil
		if request.RandomSeed == nil && request.Seed != nil {
			request.RandomSeed = request.Seed
		}
		request.Seed = nil
		request.User = ""
	case "gemini", "vertex":
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
	case "cerebras":
		request.Metadata = nil
	case "gemini":
		request.Metadata = nil
		request.Seed = nil
		request.RandomSeed = nil
		request.User = ""
		request.FrequencyPenalty = nil
		request.PresencePenalty = nil
	case "vertex":
		request.Metadata = nil
		request.User = ""
	}

	return request
}

func detectProvider(baseURL, providerType string, auth types.ProviderAuth) string {
	switch auth.Env {
	case "GROQ_API_KEY":
		return "groq"
	case "CEREBRAS_API_KEY":
		return "cerebras"
	case "MISTRAL_API_KEY":
		return "mistral"
	case "GEMINI_API_KEY":
		return "gemini"
	case "NIM_API_KEY":
		return "nim"
	case "OLLAMA_API_KEY":
		return "ollama"
	case "KILO_API_KEY":
		return "kilo"
	case "GOOGLE_VERTEX_API_KEY":
		return "vertex"
	}

	if providerType == "vertex" {
		return "vertex"
	}

	switch {
	case strings.Contains(baseURL, "api.groq.com"):
		return "groq"
	case strings.Contains(baseURL, "api.cerebras.ai"):
		return "cerebras"
	case strings.Contains(baseURL, "api.mistral.ai"):
		return "mistral"
	case strings.Contains(baseURL, "generativelanguage.googleapis.com"):
		return "gemini"
	case strings.Contains(baseURL, "aiplatform.googleapis.com"):
		return "vertex"
	case strings.Contains(baseURL, "integrate.api.nvidia.com"):
		return "nim"
	case strings.Contains(baseURL, "api.kilo.ai"):
		return "kilo"
	case strings.Contains(baseURL, "ollama.com"):
		return "ollama"
	default:
		return ""
	}
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

func schemaObjectsDisallowAdditionalProperties(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}

	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return false
	}

	return walkSchemaAdditionalProperties(schema)
}

func walkSchemaAdditionalProperties(node any) bool {
	switch typed := node.(type) {
	case map[string]any:
		if schemaType, ok := typed["type"].(string); ok && schemaType == "object" {
			flag, ok := typed["additionalProperties"].(bool)
			if !ok || flag {
				return false
			}
		}
		for _, value := range typed {
			if !walkSchemaAdditionalProperties(value) {
				return false
			}
		}
	case []any:
		for _, value := range typed {
			if !walkSchemaAdditionalProperties(value) {
				return false
			}
		}
	}
	return true
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
		body, _ := io.ReadAll(resp.Body)
		retryAfter, limitType := parseRateLimitDetails(provider, resp.Header, body)
		return nil, &errors.RateLimitError{
			ProviderError: errors.ProviderError{Message: normalizeProviderErrorMessage(provider, resp.StatusCode, body), StatusCode: 429, IsRetryable: true, Headers: headers},
			RetryAfter:    retryAfter,
			LimitType:     limitType,
		}
	}

	if resp.StatusCode == http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		return nil, &errors.PaymentRequiredError{ProviderError: errors.ProviderError{Message: normalizeProviderErrorMessage(provider, resp.StatusCode, body), StatusCode: 402, IsRetryable: false, Headers: headers}}
	}

	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		return nil, &errors.ProviderError{
			Message:     normalizeProviderErrorMessage(provider, resp.StatusCode, body),
			StatusCode:  resp.StatusCode,
			IsRetryable: true,
			Headers:     headers,
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		message := normalizeProviderErrorMessage(provider, resp.StatusCode, body)
		if isValidationStatus(resp.StatusCode) {
			validationErr := errors.NewValidationError(message, nil)
			validationErr.StatusCode = resp.StatusCode
			return nil, validationErr
		}
		return nil, &errors.ProviderError{
			Message:     message,
			StatusCode:  resp.StatusCode,
			IsRetryable: resp.StatusCode >= 500,
			Headers:     headers,
		}
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	s.logRawProviderResponseBody(provider, model, resp.StatusCode, body)

	var response types.ChatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (s *ProviderService) handleErrorResponse(resp *http.Response, baseURL, providerType string, auth types.ProviderAuth) error {
	body, _ := io.ReadAll(resp.Body)
	provider := detectProvider(baseURL, providerType, auth)
	headers := flattenHeaders(resp.Header)

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		retryAfter, limitType := parseRateLimitDetails(provider, resp.Header, body)
		return &errors.RateLimitError{
			ProviderError: errors.ProviderError{Message: normalizeProviderErrorMessage(provider, resp.StatusCode, body), StatusCode: 429, IsRetryable: true, Headers: headers},
			RetryAfter:    retryAfter,
			LimitType:     limitType,
		}
	case http.StatusPaymentRequired:
		return &errors.PaymentRequiredError{ProviderError: errors.ProviderError{Message: normalizeProviderErrorMessage(provider, resp.StatusCode, body), StatusCode: 402, IsRetryable: false, Headers: headers}}
	default:
		message := normalizeProviderErrorMessage(provider, resp.StatusCode, body)
		if isValidationStatus(resp.StatusCode) {
			validationErr := errors.NewValidationError(message, nil)
			validationErr.StatusCode = resp.StatusCode
			return validationErr
		}
		return &errors.ProviderError{
			Message:     message,
			StatusCode:  resp.StatusCode,
			IsRetryable: resp.StatusCode >= 500,
			Headers:     headers,
		}
	}
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

func parseRateLimitDetails(provider string, headers http.Header, body []byte) (int, string) {
	retryAfter := 60
	if value := headers.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
			retryAfter = seconds
		}
	}

	switch provider {
	case "groq":
		if limit := headers.Get("X-RateLimit-Limit-Requests"); limit != "" {
			return retryAfter, "rpd"
		}
		if limit := headers.Get("X-RateLimit-Limit-Tokens"); limit != "" {
			return retryAfter, "tpm"
		}
	case "cerebras":
		if headers.Get("X-RateLimit-Limit-Requests-Minute") != "" {
			return retryAfter, "rpm"
		}
		if headers.Get("X-RateLimit-Limit-Tokens-Minute") != "" {
			return retryAfter, "tpm"
		}
		if headers.Get("X-RateLimit-Limit-Requests-Day") != "" {
			return retryAfter, "rpd"
		}
	case "vertex", "gemini":
		if strings.Contains(strings.ToUpper(string(body)), "RESOURCE_EXHAUSTED") {
			return retryAfter, "resource_exhausted"
		}
	}

	return retryAfter, "rpm"
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
		Type    string `json:"type"`
		Code    any    `json:"code"`
		Object  string `json:"object"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err == nil {
		if wrapped.Error.Message != "" {
			return wrapped.Error.Message
		}
		switch msg := wrapped.Message.(type) {
		case string:
			return msg
		case map[string]any:
			if detail, ok := msg["detail"]; ok {
				return fmt.Sprintf("%v", detail)
			}
		}
	}

	var googleArray []map[string]any
	if err := json.Unmarshal(trimmed, &googleArray); err == nil && len(googleArray) > 0 {
		if errObj, ok := googleArray[0]["error"].(map[string]any); ok {
			if message, ok := errObj["message"].(string); ok {
				return message
			}
		}
	}

	return strings.TrimSpace(string(trimmed))
}

func isValidationStatus(statusCode int) bool {
	return statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity
}

func (s *ProviderService) parseSSEStreamChannel(ctx context.Context, body io.ReadCloser, chunks chan<- *types.SSEChunk, provider, model string) error {
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineCh := make(chan string)
	scanErrCh := make(chan error, 1)

	go func() {
		defer close(lineCh)
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
		scanErrCh <- scanner.Err()
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
			body.Close()
			return ctx.Err()

		case <-timer.C:
			body.Close()
			return errors.NewTimeoutError("Inactivity timeout", "inactivity")

		case line, ok := <-lineCh:
			if !ok {
				return <-scanErrCh
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

			var chunk types.SSEChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				logger.Error().
					Str("type", "http").
					Str("event", "sse.parse_failed").
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
				body.Close()
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
	}

	return true
}
