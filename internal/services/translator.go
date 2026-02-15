package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/google/uuid"
)

func ResponseRequestToChatCompletion(req *types.ResponseRequest) (*types.ChatCompletionRequest, error) {
	chatReq := &types.ChatCompletionRequest{
		Model:               req.Model,
		Temperature:         req.Temperature,
		MaxTokens:           req.MaxTokens,
		MaxCompletionTokens: req.MaxCompletionTokens,
		TopP:                req.TopP,
		Stream:              req.Stream,
		Tools:               req.Tools,
		ToolChoice:          req.ToolChoice,
		Router:              req.Router,
	}

	if req.Text != nil && req.Text.Format != nil {
		chatReq.ResponseFormat = req.Text.Format
	}

	if req.Instructions != nil {
		chatReq.Messages = append(chatReq.Messages, types.OpenAIMessage{
			Role:    "system",
			Content: *req.Instructions,
		})
	}

	inputItems, err := types.ParseResponseInput(req.Input)
	if err != nil {
		return nil, err
	}

	messages, err := InputItemsToMessages(inputItems)
	if err != nil {
		return nil, err
	}
	chatReq.Messages = append(chatReq.Messages, messages...)

	return chatReq, nil
}

func InputItemsToMessages(items []types.ResponseInputItem) ([]types.OpenAIMessage, error) {
	var messages []types.OpenAIMessage
	var pendingToolCalls []types.ToolCall
	var pendingRole string

	for _, item := range items {
		switch {
		case item.IsMessage():
			if len(pendingToolCalls) > 0 {
				messages = append(messages, types.OpenAIMessage{
					Role:      "assistant",
					ToolCalls: pendingToolCalls,
				})
				pendingToolCalls = nil
			}

			content := item.Content
			if str, ok := content.(string); ok {
				content = str
			}

			messages = append(messages, types.OpenAIMessage{
				Role:    item.Role,
				Content: content,
			})

		case item.IsFunctionCall():
			if pendingRole == "" {
				pendingRole = "assistant"
			}

			callID := item.CallID
			if callID == "" {
				callID = fmt.Sprintf("call_%s", uuid.New().String()[:24])
			}

			pendingToolCalls = append(pendingToolCalls, types.ToolCall{
				ID:   callID,
				Type: "function",
				Function: types.FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})

		case item.IsFunctionCallOutput():
			if len(pendingToolCalls) > 0 {
				messages = append(messages, types.OpenAIMessage{
					Role:      "assistant",
					ToolCalls: pendingToolCalls,
				})
				pendingToolCalls = nil
			}

			messages = append(messages, types.OpenAIMessage{
				Role:       "tool",
				ToolCallID: item.CallID,
				Content:    item.Output,
			})

		default:
			return nil, &types.GatewayError{
				Type:    "validation_error",
				Code:    "UNKNOWN_ITEM_TYPE",
				Message: fmt.Sprintf("unknown item type: %s", item.Type),
			}
		}
	}

	if len(pendingToolCalls) > 0 {
		messages = append(messages, types.OpenAIMessage{
			Role:      "assistant",
			ToolCalls: pendingToolCalls,
		})
	}

	return messages, nil
}

func ChatCompletionToResponse(chatResp *types.ChatCompletionResponse) *types.Response {
	output := make([]types.ResponseItem, 0)
	var outputTexts []string

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]

		if choice.Message.Content != nil && *choice.Message.Content != "" {
			text := *choice.Message.Content
			outputTexts = append(outputTexts, text)

			output = append(output, types.ResponseItem{
				ID:     fmt.Sprintf("msg_%s", chatResp.ID),
				Type:   "message",
				Role:   "assistant",
				Status: "completed",
				Content: []types.ContentOutput{{
					Type: "output_text",
					Text: text,
				}},
			})
		}

		for _, tc := range choice.Message.ToolCalls {
			output = append(output, types.ResponseItem{
				ID:        fmt.Sprintf("fc_%s", strings.TrimPrefix(tc.ID, "call_")),
				Type:      "function_call",
				CallID:    tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
				Status:    "completed",
			})
		}
	}

	status := "completed"
	if len(chatResp.Choices) > 0 && chatResp.Choices[0].FinishReason == "length" {
		status = "incomplete"
	}

	return &types.Response{
		ID:         fmt.Sprintf("resp_%s", chatResp.ID),
		Object:     "response",
		CreatedAt:  chatResp.Created,
		Model:      chatResp.Model,
		Output:     output,
		OutputText: strings.Join(outputTexts, "\n"),
		Usage:      chatResp.Usage,
		Status:     status,
	}
}

func ComputeOutputText(output []types.ResponseItem) string {
	var texts []string
	for _, item := range output {
		if item.Type == "message" {
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					texts = append(texts, content.Text)
				}
			}
		}
	}
	return strings.Join(texts, "\n")
}

func GenerateResponseID() string {
	return fmt.Sprintf("resp_%d_%s", time.Now().Unix(), uuid.New().String()[:8])
}

func GenerateCallID() string {
	return fmt.Sprintf("call_%s", uuid.New().String()[:24])
}

func ValidateStatelessRequest(req *types.ResponseRequest) error {
	if req.Store != nil && *req.Store {
		return &types.GatewayError{
			Type:    "validation_error",
			Code:    "STATELESS_VIOLATION",
			Message: "store=true is not supported - gateway is stateless",
		}
	}

	if req.PreviousResponseID != nil && *req.PreviousResponseID != "" {
		return &types.GatewayError{
			Type:    "validation_error",
			Code:    "STATELESS_VIOLATION",
			Message: "previous_response_id is not supported - gateway is stateless",
		}
	}

	return nil
}

func ExtractProviderFromResponseID(responseID string) (string, error) {
	return "", nil
}

func MessagesToInputItems(messages []types.OpenAIMessage) []types.ResponseInputItem {
	items := make([]types.ResponseInputItem, 0, len(messages))

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			continue

		case "user", "assistant":
			items = append(items, types.ResponseInputItem{
				Type:    "message",
				Role:    msg.Role,
				Content: msg.Content,
			})

			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					items = append(items, types.ResponseInputItem{
						Type:      "function_call",
						CallID:    tc.ID,
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					})
				}
			}

		case "tool":
			outputStr := ""
			switch v := msg.Content.(type) {
			case string:
				outputStr = v
			default:
				bytes, _ := json.Marshal(v)
				outputStr = string(bytes)
			}
			items = append(items, types.ResponseInputItem{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: outputStr,
			})
		}
	}

	return items
}

func ResponseToChatCompletionRequest(resp *types.Response) (*types.ChatCompletionRequest, error) {
	chatReq := &types.ChatCompletionRequest{
		Model: resp.Model,
	}

	messages := make([]types.OpenAIMessage, 0)
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			content := ""
			if len(item.Content) > 0 && item.Content[0].Text != "" {
				content = item.Content[0].Text
			}
			messages = append(messages, types.OpenAIMessage{
				Role:    item.Role,
				Content: content,
			})

		case "function_call":
			messages = append(messages, types.OpenAIMessage{
				Role: "assistant",
				ToolCalls: []types.ToolCall{{
					ID:   item.CallID,
					Type: "function",
					Function: types.FunctionCall{
						Name:      item.Name,
						Arguments: item.Arguments,
					},
				}},
			})

		case "function_call_output":
			outputStr := ""
			switch v := item.Output.(type) {
			case string:
				outputStr = v
			default:
				bytes, _ := json.Marshal(v)
				outputStr = string(bytes)
			}
			messages = append(messages, types.OpenAIMessage{
				Role:       "tool",
				ToolCallID: item.CallID,
				Content:    outputStr,
			})
		}
	}

	chatReq.Messages = messages
	return chatReq, nil
}

func ParseStringContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		for _, part := range v {
			if m, ok := part.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					return text
				}
			}
		}
	}
	return fmt.Sprintf("%v", content)
}

func EstimateResponseTokens(resp *types.Response) int {
	tokens := 0
	for _, item := range resp.Output {
		if item.Type == "message" {
			for _, content := range item.Content {
				tokens += len(content.Text) / 4
			}
		}
		if item.Type == "function_call" {
			tokens += len(item.Name) / 4
			tokens += len(item.Arguments) / 4
		}
	}
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

func FormatToolOutput(ok bool, errorType, message string, data map[string]any) string {
	result := map[string]any{
		"ok": ok,
	}
	if !ok {
		result["error"] = errorType
		result["message"] = message
	}
	for k, v := range data {
		result[k] = v
	}
	bytes, _ := json.Marshal(result)
	return string(bytes)
}

func ParseToolOutput(output string) (bool, string, string, map[string]any) {
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return false, "parse_error", "Failed to parse tool output", nil
	}

	ok, _ := result["ok"].(bool)
	errorType, _ := result["error"].(string)
	message, _ := result["message"].(string)

	data := make(map[string]any)
	for k, v := range result {
		if k != "ok" && k != "error" && k != "message" {
			data[k] = v
		}
	}

	return ok, errorType, message, data
}

func GetIntFromMap(m map[string]any, key string) *int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return &n
		case int64:
			i := int(n)
			return &i
		case float64:
			i := int(n)
			return &i
		case string:
			if i, err := strconv.Atoi(n); err == nil {
				return &i
			}
		}
	}
	return nil
}

func GetFloatFromMap(m map[string]any, key string) *float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return &n
		case float32:
			f := float64(n)
			return &f
		case int:
			f := float64(n)
			return &f
		case string:
			if f, err := strconv.ParseFloat(n, 64); err == nil {
				return &f
			}
		}
	}
	return nil
}
