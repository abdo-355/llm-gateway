package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/types"
)

type ProviderService struct {
	httpClient *http.Client
}

func NewProviderService() *ProviderService {
	return &ProviderService{
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
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
	reqBody, err := s.prepareRequest(request, model, providerType)
	if err != nil {
		return nil, err
	}

	// Build URL
	url := fmt.Sprintf("%s/chat/completions", baseURL)
	if providerType == "vertex" {
		url = s.buildVertexURL(baseURL, model, false)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if auth.Type == "bearer" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	} else if auth.Type == "header" && auth.HeaderName != "" {
		req.Header.Set(auth.HeaderName, apiKey)
	}

	// Make request with timeout
	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.NewTimeoutError("Request timeout", "request")
		}
		return nil, err
	}
	defer resp.Body.Close()

	return s.handleResponse(resp, providerType, request, model)
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

		reqBody, err := s.prepareRequest(request, model, providerType)
		if err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "REQUEST_PREP_FAILED", Message: err.Error()}
			return
		}

		url := fmt.Sprintf("%s/chat/completions", baseURL)
		if providerType == "vertex" {
			url = s.buildVertexURL(baseURL, model, true)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
		if err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "REQUEST_CREATE_FAILED", Message: err.Error()}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

		if auth.Type == "bearer" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
		} else if auth.Type == "header" && auth.HeaderName != "" {
			req.Header.Set(auth.HeaderName, apiKey)
		}

		client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
		resp, err := client.Do(req)
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
			errChan <- s.convertToGatewayError(s.handleErrorResponse(resp))
			return
		}

		if err := s.parseSSEStreamChannel(ctx, resp.Body, chunks); err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "STREAM_PARSE_FAILED", Message: err.Error()}
		}
	}()

	return types.StreamResult{
		Chunks: chunks,
		Err:    errChan,
	}
}

func (s *ProviderService) convertToGatewayError(err error) *types.GatewayError {
	if ge, ok := err.(*types.GatewayError); ok {
		return ge
	}
	return &types.GatewayError{Type: "provider_error", Code: "UNKNOWN", Message: err.Error()}
}

func (s *ProviderService) prepareRequest(request types.ChatCompletionRequest, model, providerType string) ([]byte, error) {
	// Remove internal router field
	request.Router = nil

	request.Model = model

	if providerType == "vertex" {
		vertexReq := s.transformToVertexRequest(request)
		return json.Marshal(vertexReq)
	}

	return json.Marshal(request)
}

// transformToVertexRequest converts OpenAI format to Vertex AI format
// Maps message roles and extracts text content from various content types
func (s *ProviderService) transformToVertexRequest(request types.ChatCompletionRequest) map[string]any {
	contents := []map[string]any{}

	for _, msg := range request.Messages {
		// Extract text from content (handle string or array of content parts)
		text := ""
		switch content := msg.Content.(type) {
		case string:
			text = content
		case []any:
			// Handle array of content parts (text, image, etc.)
			for _, part := range content {
				if partMap, ok := part.(map[string]any); ok {
					if partType, ok := partMap["type"].(string); ok && partType == "text" {
						if textValue, ok := partMap["text"].(string); ok {
							text += textValue
						}
					}
				}
			}
		}

		// Vertex AI doesn't support 'system' role, map to 'user'
		// 'assistant' role maps to 'model' in Vertex
		role := msg.Role
		if role == "system" {
			role = "user"
		} else if role == "assistant" {
			role = "model"
		}

		contents = append(contents, map[string]any{
			"role":  role,
			"parts": []map[string]string{{"text": text}},
		})
	}

	// Build generation config from request parameters
	generationConfig := map[string]any{}
	if request.Temperature != nil {
		generationConfig["temperature"] = *request.Temperature
	}
	if request.MaxTokens != nil {
		generationConfig["maxOutputTokens"] = *request.MaxTokens
	}
	if request.TopP != nil {
		generationConfig["topP"] = *request.TopP
	}

	req := map[string]any{
		"contents": contents,
	}
	if len(generationConfig) > 0 {
		req["generationConfig"] = generationConfig
	}

	return req
}

func (s *ProviderService) buildVertexURL(baseURL, model string, streaming bool) string {
	action := "generateContent"
	if streaming {
		action = "streamGenerateContent"
	}

	// Ensure baseUrl ends with /v1 for global endpoint
	cleanBaseURL := baseURL
	if !strings.HasSuffix(baseURL, "/v1") {
		cleanBaseURL = baseURL + "/v1"
	}
	return fmt.Sprintf("%s/publishers/google/models/%s:%s", cleanBaseURL, model, action)
}

func (s *ProviderService) handleResponse(resp *http.Response, providerType string, _ types.ChatCompletionRequest, model string) (*types.ChatCompletionResponse, error) {
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.NewRateLimitError("Rate limited", 60, "rpm")
	}

	if resp.StatusCode == http.StatusPaymentRequired {
		return nil, errors.NewPaymentRequiredError("Payment required")
	}

	if resp.StatusCode >= 500 {
		return nil, &errors.ProviderError{
			Message:     fmt.Sprintf("Server error: %d", resp.StatusCode),
			StatusCode:  resp.StatusCode,
			IsRetryable: true,
		}
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &errors.ProviderError{
			Message:     fmt.Sprintf("HTTP error %d: %s", resp.StatusCode, string(body)),
			StatusCode:  resp.StatusCode,
			IsRetryable: resp.StatusCode >= 500,
		}
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if providerType == "vertex" {
		return s.transformVertexResponse(body, model)
	}

	var response types.ChatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (s *ProviderService) transformVertexResponse(body []byte, model string) (*types.ChatCompletionResponse, error) {
	var vertexResp map[string]any
	if err := json.Unmarshal(body, &vertexResp); err != nil {
		return nil, err
	}

	// Extract candidate with early return pattern to reduce nesting
	candidates, ok := vertexResp["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return s.createEmptyVertexResponse(model), nil
	}

	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		return s.createEmptyVertexResponse(model), nil
	}

	content := s.extractVertexText(candidate)
	finishReason := s.mapVertexFinishReason(candidate["finishReason"])

	// Extract usage metadata
	usage := s.extractVertexUsage(vertexResp)

	return &types.ChatCompletionResponse{
		ID:      "vertex-" + time.Now().Format("20060102150405"),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ResponseMessage{
				Role:    "assistant",
				Content: &content,
			},
			FinishReason: finishReason,
		}},
		Usage: usage,
	}, nil
}

func (s *ProviderService) createEmptyVertexResponse(model string) *types.ChatCompletionResponse {
	emptyContent := ""
	return &types.ChatCompletionResponse{
		ID:      "vertex-" + time.Now().Format("20060102150405"),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ResponseMessage{
				Role:    "assistant",
				Content: &emptyContent,
			},
			FinishReason: "stop",
		}},
		Usage: &types.Usage{},
	}
}

func (s *ProviderService) extractVertexText(candidate map[string]any) string {
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		return ""
	}

	parts, ok := content["parts"].([]any)
	if !ok || len(parts) == 0 {
		return ""
	}

	part, ok := parts[0].(map[string]any)
	if !ok {
		return ""
	}

	text, _ := part["text"].(string)
	return text
}

func (s *ProviderService) extractVertexUsage(vertexResp map[string]any) *types.Usage {
	usage := &types.Usage{}
	if usageMeta, ok := vertexResp["usageMetadata"].(map[string]any); ok {
		if promptTokens, ok := usageMeta["promptTokenCount"].(float64); ok {
			usage.PromptTokens = int(promptTokens)
		}
		if completionTokens, ok := usageMeta["candidatesTokenCount"].(float64); ok {
			usage.CompletionTokens = int(completionTokens)
		}
		if totalTokens, ok := usageMeta["totalTokenCount"].(float64); ok {
			usage.TotalTokens = int(totalTokens)
		}
	}
	return usage
}

func (s *ProviderService) mapVertexFinishReason(reason any) string {
	reasonStr, _ := reason.(string)
	switch reasonStr {
	case "STOP":
		return "stop"
	case "MAX_TOKENS", "LENGTH":
		return "length"
	case "SAFETY", "RECITATION":
		return "content_filter"
	default:
		return "stop"
	}
}

func (s *ProviderService) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return errors.NewRateLimitError("Rate limited", 60, "rpm")
	case http.StatusPaymentRequired:
		return errors.NewPaymentRequiredError("Payment required")
	default:
		return &errors.ProviderError{
			Message:     fmt.Sprintf("HTTP error %d: %s", resp.StatusCode, string(body)),
			StatusCode:  resp.StatusCode,
			IsRetryable: resp.StatusCode >= 500,
		}
	}
}

func (s *ProviderService) parseSSEStreamChannel(ctx context.Context, body io.ReadCloser, chunks chan<- *types.SSEChunk) error {
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

	inactivity := 60 * time.Second
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

			select {
			case chunks <- &chunk:
			case <-ctx.Done():
				body.Close()
				return ctx.Err()
			}
		}
	}
}
