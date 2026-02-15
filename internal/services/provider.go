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
		httpClient: &http.Client{},
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
			errChan <- s.convertToGatewayError(s.handleErrorResponse(resp))
			return
		}

		if err := s.parseSSEStreamChannel(ctx, resp.Body, chunks); err != nil {
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
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	case "header":
		if auth.HeaderName != "" {
			req.Header.Set(auth.HeaderName, apiKey)
		}
	case "adc":
		token, err := GetVertexToken(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	return nil
}

func (s *ProviderService) convertToGatewayError(err error) *types.GatewayError {
	if ge, ok := err.(*types.GatewayError); ok {
		return ge
	}
	return &types.GatewayError{Type: "provider_error", Code: "UNKNOWN", Message: err.Error()}
}

func (s *ProviderService) prepareRequest(request types.ChatCompletionRequest, model, providerType string) ([]byte, error) {
	request.Router = nil

	request.Model = model

	if request.ResponseFormat != nil && request.ResponseFormat.Type == "json_object" {
		request.Messages = ensureJSONKeyword(request.Messages)
	}

	return json.Marshal(request)
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

	var response types.ChatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
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
