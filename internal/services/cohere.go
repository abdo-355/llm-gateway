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
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/google/uuid"
)

const cohereProviderType = "cohere"

type cohereChatRequest struct {
	Model       string  `json:"model"`
	Message     string  `json:"message"`
	Temperature float64 `json:"temperature,omitempty"`
	Stream      bool    `json:"stream,omitempty"`
	MaxTokens   *int    `json:"max_tokens,omitempty"`
	P           *float64 `json:"p,omitempty"`
}

type cohereChatResponse struct {
	Text         string          `json:"text"`
	GenerationID string          `json:"generation_id"`
	FinishReason *string         `json:"finish_reason,omitempty"`
	Meta         *cohereChatMeta `json:"meta,omitempty"`
}

type cohereChatMeta struct {
	Tokens *cohereTokens `json:"tokens,omitempty"`
}

type cohereTokens struct {
	InputTokens  float64 `json:"input_tokens"`
	OutputTokens float64 `json:"output_tokens"`
}

type cohereStreamEvent struct {
	EventType string                 `json:"event_type"`
	Place     map[string]any         `json:"place,omitempty"`
	Text      string                 `json:"text,omitempty"`
	Meta      *cohereChatMeta        `json:"meta,omitempty"`
	Raw       map[string]interface{} `json:"-"`
}

func (s *ProviderService) callCohereProvider(
	baseURL, apiKey, model string,
	request types.ChatCompletionRequest,
	ctx context.Context,
	auth types.ProviderAuth,
) (*types.ChatCompletionResponse, error) {
	msg := buildCohereMessage(request.Messages)
	cohereReq := cohereChatRequest{
		Model:   model,
		Message: msg,
		Stream:  false,
	}

	if request.Temperature != nil {
		cohereReq.Temperature = *request.Temperature
	}
	if request.MaxTokens != nil {
		cohereReq.MaxTokens = request.MaxTokens
	} else if request.MaxCompletionTokens != nil {
		cohereReq.MaxTokens = request.MaxCompletionTokens
	}
	if request.TopP != nil {
		cohereReq.P = request.TopP
	}

	reqBody, err := json.Marshal(cohereReq)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/chat", baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if err := s.setAuth(ctx, req, apiKey, auth); err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if timeoutErr := requestTimeoutError(ctx); timeoutErr != nil {
			return nil, timeoutErr
		}
		return nil, wrapNetworkError(err, "cohere", baseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, s.handleErrorResponse(resp, baseURL, "cohere", auth)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.NewParseError("Failed to read Cohere response", "json", "cohere", model, "", err)
	}

	var cohereResp cohereChatResponse
	if err := json.Unmarshal(body, &cohereResp); err != nil {
		return nil, errors.NewParseError("Failed to parse Cohere response", "json", "cohere", model, string(body), err)
	}

	openAIResp := cohereToOpenAIResponse(cohereResp, model)
	return &openAIResp, nil
}

func (s *ProviderService) callCohereStreamProvider(
	baseURL, apiKey, model string,
	request types.ChatCompletionRequest,
	ctx context.Context,
	auth types.ProviderAuth,
) types.StreamResult {
	chunks := make(chan *types.SSEChunk)
	errChan := make(chan *types.GatewayError, 1)

	go func() {
		defer close(chunks)
		defer close(errChan)

		msg := buildCohereMessage(request.Messages)
		cohereReq := cohereChatRequest{
			Model:   model,
			Message: msg,
			Stream:  true,
		}

		if request.Temperature != nil {
			cohereReq.Temperature = *request.Temperature
		}
		if request.MaxTokens != nil {
			cohereReq.MaxTokens = request.MaxTokens
		} else if request.MaxCompletionTokens != nil {
			cohereReq.MaxTokens = request.MaxCompletionTokens
		}
		if request.TopP != nil {
			cohereReq.P = request.TopP
		}

		reqBody, err := json.Marshal(cohereReq)
		if err != nil {
			errChan <- &types.GatewayError{Type: "provider_error", Code: "REQUEST_PREP_FAILED", Message: err.Error()}
			return
		}

		url := fmt.Sprintf("%s/chat", baseURL)
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
				wrappedErr := wrapNetworkError(err, "cohere", baseURL)
				errChan <- &types.GatewayError{
					Type:    "network_error",
					Code:    "NETWORK_ERROR",
					Message: wrappedErr.Error(),
				}
			}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			err := s.handleErrorResponse(resp, baseURL, "cohere", auth)
			errChan <- s.convertToGatewayError(err)
			return
		}

		generationID := uuid.New().String()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}

			payload := strings.TrimPrefix(line, "data: ")

			if payload == "[DONE]" {
				chunks <- &types.SSEChunk{
					Object: "chat.completion.chunk",
					Choices: []types.DeltaChoice{{
						Index: 0,
						Delta: types.DeltaMessage{},
						FinishReason: func() *string { s := "stop"; return &s }(),
					}},
				}
				break
			}

			var event cohereStreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				continue
			}

			switch event.EventType {
			case "text-generation":
				chunk := &types.SSEChunk{
					ID:     generationID,
					Object: "chat.completion.chunk",
					Model:  model,
					Created: time.Now().Unix(),
					Choices: []types.DeltaChoice{{
						Index:        0,
						Delta:        types.DeltaMessage{Content: &event.Text},
						FinishReason: nil,
					}},
				}
				chunks <- chunk

			case "stream-end":
				finishReason := "stop"
				chunks <- &types.SSEChunk{
					ID:     generationID,
					Object: "chat.completion.chunk",
					Model:  model,
					Created: time.Now().Unix(),
					Choices: []types.DeltaChoice{{
						Index:        0,
						Delta:        types.DeltaMessage{},
						FinishReason: &finishReason,
					}},
				}
				if event.Meta != nil && event.Meta.Tokens != nil {
					promptTokens := int(event.Meta.Tokens.InputTokens)
					completionTokens := int(event.Meta.Tokens.OutputTokens)
					totalTokens := promptTokens + completionTokens
					chunks <- &types.SSEChunk{
						ID:     generationID,
						Object: "chat.completion.chunk",
						Model:  model,
						Created: time.Now().Unix(),
						Choices: []types.DeltaChoice{{
							Index: 0,
							Delta: types.DeltaMessage{},
						}},
						Usage: &types.Usage{
							PromptTokens:     promptTokens,
							CompletionTokens: completionTokens,
							TotalTokens:      totalTokens,
						},
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- &types.GatewayError{
				Type:    "parse_error",
				Code:    "SSE_SCAN_ERROR",
				Message: fmt.Sprintf("Failed to read Cohere SSE stream: %v", err),
			}
			return
		}
	}()

	return types.StreamResult{Chunks: chunks, Err: errChan}
}

func buildCohereMessage(messages []types.OpenAIMessage) string {
	var parts []string
	for _, msg := range messages {
		content := ""
		switch c := msg.Content.(type) {
		case string:
			content = c
		case []any:
			for _, item := range c {
				if part, ok := item.(map[string]any); ok {
					if text, ok := part["text"].(string); ok {
						content += text
					}
				}
			}
		}
		if content != "" {
			if msg.Role == "system" {
				parts = append(parts, content)
			} else if msg.Role == "user" {
				parts = append(parts, content)
			} else if msg.Role == "assistant" {
				parts = append(parts, content)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func cohereToOpenAIResponse(cohereResp cohereChatResponse, model string) types.ChatCompletionResponse {
	promptTokens := 0
	completionTokens := 0
	if cohereResp.Meta != nil && cohereResp.Meta.Tokens != nil {
		promptTokens = int(cohereResp.Meta.Tokens.InputTokens)
		completionTokens = int(cohereResp.Meta.Tokens.OutputTokens)
	}

	finishReason := "stop"
	if cohereResp.FinishReason != nil {
		finishReason = *cohereResp.FinishReason
	}

	return types.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8]),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ResponseMessage{
					Role:    "assistant",
					Content: &cohereResp.Text,
				},
				FinishReason: finishReason,
			},
		},
		Usage: &types.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}
}
