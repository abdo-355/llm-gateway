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
	"github.com/google/uuid"
)

// ollamaChatRequest is the native Ollama /api/chat request body.
type ollamaChatRequest struct {
	Model    string                `json:"model"`
	Messages []types.OpenAIMessage `json:"messages"`
	Stream   bool                  `json:"stream,omitempty"`
	Format   any                   `json:"format,omitempty"`
	Options  *ollamaOptions        `json:"options,omitempty"`
	Tools    []types.OpenAITool    `json:"tools,omitempty"`
}

type ollamaOptions struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	NumPredict       *int     `json:"num_predict,omitempty"`
	Seed             *int     `json:"seed,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	Stop             any      `json:"stop,omitempty"`
}

// ollamaChatResponse is the native Ollama /api/chat response body.
type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

// prepareOllamaRequest converts an OpenAI ChatCompletionRequest into an Ollama native request.
func (s *ProviderService) prepareOllamaRequest(request types.ChatCompletionRequest, model string) ([]byte, error) {
	ollamaReq := ollamaChatRequest{
		Model:    model,
		Messages: request.Messages,
		Stream:   false,
	}

	if request.Stream != nil {
		ollamaReq.Stream = *request.Stream
	}

	ollamaReq.Options = &ollamaOptions{}

	if request.Temperature != nil {
		ollamaReq.Options.Temperature = request.Temperature
	}

	if request.MaxTokens != nil {
		ollamaReq.Options.NumPredict = request.MaxTokens
	} else if request.MaxCompletionTokens != nil {
		ollamaReq.Options.NumPredict = request.MaxCompletionTokens
	}

	if request.Seed != nil {
		ollamaReq.Options.Seed = request.Seed
	}

	if request.TopP != nil {
		ollamaReq.Options.TopP = request.TopP
	}

	if request.FrequencyPenalty != nil {
		ollamaReq.Options.FrequencyPenalty = request.FrequencyPenalty
	}

	if request.PresencePenalty != nil {
		ollamaReq.Options.PresencePenalty = request.PresencePenalty
	}

	if request.Stop != nil {
		ollamaReq.Options.Stop = request.Stop
	}

	// Map response_format to native format field
	if request.ResponseFormat != nil {
		switch request.ResponseFormat.Type {
		case "json_object":
			ollamaReq.Format = "json"
		case "json_schema":
			if request.ResponseFormat.JSONSchema != nil {
				ollamaReq.Format = request.ResponseFormat.JSONSchema.Schema
			} else {
				ollamaReq.Format = "json"
			}
		}
	}

	// Tools passthrough
	if len(request.Tools) > 0 {
		ollamaReq.Tools = request.Tools
	}

	return json.Marshal(ollamaReq)
}

// handleOllamaResponse parses an Ollama native API response and converts to OpenAI format.
func (s *ProviderService) handleOllamaResponse(resp *http.Response, model string) (*types.ChatCompletionResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	provider := "ollama"

	if resp.StatusCode == http.StatusTooManyRequests {
		headers := flattenHeaders(resp.Header)
		retryAfter := 60
		if value := resp.Header.Get("Retry-After"); value != "" {
			if seconds, convErr := strconvParseInt(value); convErr == nil && seconds > 0 {
				retryAfter = seconds
			}
		}
		return nil, errors.NewRateLimitErrorWithSubtype(
			normalizeProviderErrorMessage(provider, resp.StatusCode, body),
			retryAfter, "rpm", "rate_limit", headers,
		)
	}

	if resp.StatusCode == http.StatusPaymentRequired {
		return nil, &errors.PaymentRequiredError{
			ProviderError: errors.ProviderError{
				Message:     normalizeProviderErrorMessage(provider, resp.StatusCode, body),
				StatusCode:  402,
				IsRetryable: false,
				Headers:     flattenHeaders(resp.Header),
			},
		}
	}

	if resp.StatusCode >= 500 {
		return nil, &errors.ProviderError{
			Message:     normalizeProviderErrorMessage(provider, resp.StatusCode, body),
			StatusCode:  resp.StatusCode,
			IsRetryable: true,
			Headers:     flattenHeaders(resp.Header),
		}
	}

	if resp.StatusCode != http.StatusOK {
		message := normalizeProviderErrorMessage(provider, resp.StatusCode, body)
		return nil, &errors.ProviderError{
			Message:     message,
			StatusCode:  resp.StatusCode,
			IsRetryable: resp.StatusCode >= 500,
			Headers:     flattenHeaders(resp.Header),
		}
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.NewEmptyResponseError(provider, model, resp.StatusCode)
	}

	s.logRawProviderResponseBody(provider, model, resp.StatusCode, body)

	var ollamaResp ollamaChatResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		// Try parsing as NDJSON (single line) - Ollama native API always returns this format
		if firstLine, _, cut := bytes.Cut(body, []byte("\n")); cut {
			if err2 := json.Unmarshal(firstLine, &ollamaResp); err2 != nil {
				return nil, errors.NewParseError(
					fmt.Sprintf("Failed to parse Ollama response for model %s", model),
					"json",
					provider,
					model,
					truncateString(string(body), 500),
					err,
				)
			}
		} else {
			return nil, errors.NewParseError(
				fmt.Sprintf("Failed to parse Ollama response for model %s", model),
				"json",
				provider,
				model,
				truncateString(string(body), 500),
				err,
			)
		}
	}

	if ollamaResp.Message.Content == "" && len(ollamaResp.Message.ToolCalls) == 0 {
		return nil, errors.NewEmptyResponseError(provider, model, resp.StatusCode)
	}

	content := ollamaResp.Message.Content
	var refusal *string
	if content == "" {
		content = ""
	}

	toolCalls := make([]types.ToolCall, 0, len(ollamaResp.Message.ToolCalls))
	for _, tc := range ollamaResp.Message.ToolCalls {
		argsStr := ""
		switch args := tc.Function.Arguments.(type) {
		case string:
			argsStr = args
		default:
			if argsBytes, err := json.Marshal(args); err == nil {
				argsStr = string(argsBytes)
			}
		}
		toolCalls = append(toolCalls, types.ToolCall{
			ID:   "call_" + uuid.NewString()[:12],
			Type: "function",
			Function: types.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: argsStr,
			},
		})
	}

	promptTokens := ollamaResp.PromptEvalCount
	completionTokens := ollamaResp.EvalCount

	return &types.ChatCompletionResponse{
		ID:      "chatcmpl-" + uuid.NewString()[:29],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   ollamaResp.Model,
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ResponseMessage{
				Role:      ollamaResp.Message.Role,
				Content:   &content,
				ToolCalls: toolCalls,
				Refusal:   refusal,
			},
			FinishReason: "stop",
		}},
		Usage: &types.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}, nil
}

// parseOllamaSSEStream parses the Ollama native API SSE stream and converts to standard SSEChunk objects.
func (s *ProviderService) parseOllamaSSEStream(ctx context.Context, body io.ReadCloser, chunks chan<- *types.SSEChunk, model, requestID string) error {
	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	provider := "ollama"

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

			data := line

			s.logRawProviderSSEData(provider, model, data)

			var ollamaResp ollamaChatResponse
			if err := json.Unmarshal([]byte(data), &ollamaResp); err != nil {
				logger.Error().
					Str("type", "http").
					Str("event", "ollama.sse_parse_failed").
					Str("request_id", requestID).
					Str("provider", provider).
					Str("model", model).
					Err(err).
					Str("data", data).
					Msg("Failed to parse Ollama SSE line")
				continue
			}

			content := ollamaResp.Message.Content
			toolCalls := make([]types.DeltaToolCall, 0, len(ollamaResp.Message.ToolCalls))
			for _, tc := range ollamaResp.Message.ToolCalls {
				argsStr := ""
				switch args := tc.Function.Arguments.(type) {
				case string:
					argsStr = args
				default:
					if argsBytes, err := json.Marshal(args); err == nil {
						argsStr = string(argsBytes)
					}
				}
				toolCalls = append(toolCalls, types.DeltaToolCall{
					Index: 0,
					ID:    "call_" + uuid.NewString()[:12],
					Type:  "function",
					Function: &types.DeltaFunction{
						Name:      &tc.Function.Name,
						Arguments: &argsStr,
					},
				})
			}

			chunk := &types.SSEChunk{
				ID:      "chatcmpl-" + uuid.NewString()[:29],
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   ollamaResp.Model,
				Choices: []types.DeltaChoice{{
					Index: 0,
					Delta: types.DeltaMessage{
						Role:      ollamaResp.Message.Role,
						Content:   ollamaStrPtr(content),
						ToolCalls: toolCalls,
					},
				}},
			}

			if ollamaResp.Done {
				chunk.Usage = &types.Usage{
					PromptTokens:     ollamaResp.PromptEvalCount,
					CompletionTokens: ollamaResp.EvalCount,
					TotalTokens:      ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
				}
			}

			select {
			case chunks <- chunk:
			case <-ctx.Done():
				body.Close()
				return ctx.Err()
			}

			if ollamaResp.Done {
				return nil
			}
		}
	}
}

func strconvParseInt(s string) (int, error) {
	var val int
	_, err := fmt.Sscan(s, &val)
	return val, err
}

func ollamaStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// callOllamaProvider makes a non-streaming call to the Ollama native /api/chat endpoint.
// Uses stream:true internally for reliability, then collects chunks into a single response.
func (s *ProviderService) callOllamaProvider(
	baseURL, apiKey, model string,
	request types.ChatCompletionRequest,
	ctx context.Context,
	auth types.ProviderAuth,
	requestID string,
) (*types.ChatCompletionResponse, error) {
	stream := true
	requestCopy := request
	requestCopy.Stream = &stream

	reqBody, err := s.prepareOllamaRequest(requestCopy, model)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(baseURL, "/") + "/api/chat"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")

	if err := s.setAuth(ctx, req, apiKey, auth); err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if timeoutErr := requestTimeoutError(ctx); timeoutErr != nil {
			return nil, timeoutErr
		}
		return nil, wrapNetworkError(err, "ollama", baseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return s.handleOllamaResponse(resp, model)
	}

	return s.collectOllamaStreamResult(resp.Body, model, baseURL, requestID)
}

// collectOllamaStreamResult reads an NDJSON stream from Ollama and builds a single ChatCompletionResponse.
func (s *ProviderService) collectOllamaStreamResult(body io.ReadCloser, model, baseURL, requestID string) (*types.ChatCompletionResponse, error) {
	provider := "ollama"

	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var contentBuilder strings.Builder
	var role string
	var finalModel string
	promptTokens := 0
	completionTokens := 0
	var toolCalls []types.ToolCall

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		s.logRawProviderSSEData(provider, model, line)

		var ollamaResp ollamaChatResponse
		if err := json.Unmarshal([]byte(line), &ollamaResp); err != nil {
			logger.Error().
				Str("type", "http").
				Str("event", "ollama.stream_line_invalid").
				Str("request_id", requestID).
				Str("provider", provider).
				Str("model", model).
				Err(err).
				Str("data", truncateString(line, 200)).
				Msg("Failed to parse Ollama stream line")
			continue
		}

		if ollamaResp.Message.Role != "" {
			role = ollamaResp.Message.Role
		}

		contentBuilder.WriteString(ollamaResp.Message.Content)

		for _, tc := range ollamaResp.Message.ToolCalls {
			argsStr := ""
			switch args := tc.Function.Arguments.(type) {
			case string:
				argsStr = args
			default:
				if argsBytes, err := json.Marshal(args); err == nil {
					argsStr = string(argsBytes)
				}
			}
			toolCalls = append(toolCalls, types.ToolCall{
				ID:   "call_" + uuid.NewString()[:12],
				Type: "function",
				Function: types.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: argsStr,
				},
			})
		}

		finalModel = ollamaResp.Model

		if ollamaResp.Done {
			promptTokens = ollamaResp.PromptEvalCount
			completionTokens = ollamaResp.EvalCount
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, wrapNetworkError(err, provider, baseURL)
	}

	fullContent := contentBuilder.String()
	if fullContent == "" && len(toolCalls) == 0 {
		return nil, errors.NewEmptyResponseError(provider, model, 200)
	}

	return &types.ChatCompletionResponse{
		ID:      "chatcmpl-" + uuid.NewString()[:29],
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   finalModel,
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ResponseMessage{
				Role:      role,
				Content:   ollamaStrPtr(fullContent),
				ToolCalls: toolCalls,
			},
			FinishReason: "stop",
		}},
		Usage: &types.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}, nil
}

// callOllamaStreamProvider makes a streaming call to the Ollama native /api/chat endpoint.
func (s *ProviderService) callOllamaStreamProvider(
	baseURL, apiKey, model string,
	request types.ChatCompletionRequest,
	ctx context.Context,
	auth types.ProviderAuth,
	requestID string,
) types.StreamResult {
	chunks := make(chan *types.SSEChunk)
	errChan := make(chan *types.GatewayError, 1)

	go func() {
		defer close(chunks)

		stream := true
		requestCopy := request
		requestCopy.Stream = &stream
		reqBody, err := s.prepareOllamaRequest(requestCopy, model)
		if err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "REQUEST_PREP_FAILED", Message: err.Error()}
			return
		}

		url := strings.TrimRight(baseURL, "/") + "/api/chat"

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
		if err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "REQUEST_CREATE_FAILED", Message: err.Error()}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/x-ndjson")

		if err := s.setAuth(ctx, req, apiKey, auth); err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "AUTH_FAILED", Message: err.Error()}
			return
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			if timeoutErr := requestTimeoutGatewayError(ctx); timeoutErr != nil {
				errChan <- timeoutErr
			} else {
				wrappedErr := wrapNetworkError(err, "ollama", baseURL)
				errChan <- &types.GatewayError{
					Type:    "network_error",
					Code:    "NETWORK_ERROR",
					Message: wrappedErr.Error(),
					Details: map[string]any{
						"network_type": classifyNetworkError(err),
						"provider":     "ollama",
					},
				}
			}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			_, err := s.handleOllamaResponse(resp, model)
			errChan <- s.convertToGatewayError(err)
			return
		}

		if err := s.parseOllamaSSEStream(ctx, resp.Body, chunks, model, requestID); err != nil {
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
