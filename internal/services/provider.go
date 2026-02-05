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
	"github.com/abdo-355/llm-gateway/internal/lib"
	"github.com/abdo-355/llm-gateway/internal/types"
)

// ProviderService handles HTTP calls to LLM providers
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

// StreamProvider makes a streaming request to a provider
func (s *ProviderService) StreamProvider(
	baseURL, apiKey, model string,
	request types.ChatCompletionRequest,
	timeoutMs int,
	onChunk func(*types.SSEChunk),
	ctx context.Context,
	providerType string,
	auth types.ProviderAuth,
) error {
	reqBody, err := s.prepareRequest(request, model, providerType)
	if err != nil {
		return err
	}

	// Build URL
	url := fmt.Sprintf("%s/chat/completions", baseURL)
	if providerType == "vertex" {
		url = s.buildVertexURL(baseURL, model, true)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	if auth.Type == "bearer" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	} else if auth.Type == "header" && auth.HeaderName != "" {
		req.Header.Set(auth.HeaderName, apiKey)
	}

	// Make request
	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return errors.NewTimeoutError("Request timeout", "request")
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return s.handleErrorResponse(resp)
	}

	return s.parseSSEStream(resp.Body, onChunk, providerType)
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

func (s *ProviderService) transformToVertexRequest(request types.ChatCompletionRequest) map[string]any {
	// Simple transformation - in real implementation, this would be more comprehensive
	contents := []map[string]any{}

	for _, msg := range request.Messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}

		var parts []map[string]string
		switch content := msg.Content.(type) {
		case string:
			parts = append(parts, map[string]string{"text": content})
		}

		contents = append(contents, map[string]any{
			"role":  role,
			"parts": parts,
		})
	}

	req := map[string]any{
		"contents": contents,
	}

	if request.Temperature != nil {
		req["generationConfig"] = map[string]any{
			"temperature": *request.Temperature,
		}
	}

	if request.MaxTokens != nil {
		if config, ok := req["generationConfig"].(map[string]any); ok {
			config["maxOutputTokens"] = *request.MaxTokens
		}
	}

	return req
}

func (s *ProviderService) buildVertexURL(baseURL, model string, streaming bool) string {
	// Simplified URL building - actual implementation would need proper project/location
	suffix := ":generateContent"
	if streaming {
		suffix = ":streamGenerateContent"
	}
	return fmt.Sprintf("%s/models/%s%s", baseURL, model, suffix)
}

func (s *ProviderService) handleResponse(resp *http.Response, providerType string, originalReq types.ChatCompletionRequest, model string) (*types.ChatCompletionResponse, error) {
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
	// Simplified transformation
	var vertexResp map[string]any
	if err := json.Unmarshal(body, &vertexResp); err != nil {
		return nil, err
	}

	response := types.ChatCompletionResponse{
		ID:      "vertex-" + time.Now().Format("20060102150405"),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
	}

	// Extract content from candidates
	if candidates, ok := vertexResp["candidates"].([]any); ok && len(candidates) > 0 {
		if candidate, ok := candidates[0].(map[string]any); ok {
			if content, ok := candidate["content"].(map[string]any); ok {
				if parts, ok := content["parts"].([]any); ok && len(parts) > 0 {
					if part, ok := parts[0].(map[string]any); ok {
						if text, ok := part["text"].(string); ok {
							response.Choices = append(response.Choices, types.Choice{
								Index: 0,
								Message: types.ResponseMessage{
									Role:    "assistant",
									Content: &text,
								},
								FinishReason: "stop",
							})
						}
					}
				}
			}
		}
	}

	return &response, nil
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

func (s *ProviderService) parseSSEStream(reader io.Reader, onChunk func(*types.SSEChunk), providerType string) error {
	scanner := bufio.NewScanner(reader)
	inactivityTimeout := 60 * time.Second
	lastActivity := time.Now()

	for scanner.Scan() {
		line := scanner.Text()
		lastActivity = time.Now()

		// Check for inactivity timeout
		if time.Since(lastActivity) > inactivityTimeout {
			return errors.NewTimeoutError("Inactivity timeout", "inactivity")
		}

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse SSE data
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			return nil
		}

		// Parse chunk
		var chunk types.SSEChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			lib.Error("Failed to parse SSE chunk", "error", err, "data", data)
			continue
		}

		onChunk(&chunk)
	}

	return scanner.Err()
}

var providerService = NewProviderService()

func GetProviderService() *ProviderService {
	return providerService
}
