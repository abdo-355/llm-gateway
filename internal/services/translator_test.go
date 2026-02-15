package services

import (
	"encoding/json"
	"testing"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseRequestToChatCompletion(t *testing.T) {
	tests := []struct {
		name           string
		request        *types.ResponseRequest
		expectedMsgs   int
		expectedModel  string
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "string input",
			request: &types.ResponseRequest{
				Model: "gpt-4",
				Input: "Hello, world!",
			},
			expectedMsgs:  1,
			expectedModel: "gpt-4",
			expectError:   false,
		},
		{
			name: "with instructions",
			request: &types.ResponseRequest{
				Model:        "gpt-4",
				Input:        "Hello",
				Instructions: strPtr("You are a helpful assistant."),
			},
			expectedMsgs:  2,
			expectedModel: "gpt-4",
			expectError:   false,
		},
		{
			name: "with text format",
			request: &types.ResponseRequest{
				Model: "gpt-4",
				Input: "Hello",
				Text: &types.TextConfig{
					Format: &types.ResponseFormat{
						Type: "json_object",
					},
				},
			},
			expectedMsgs:  1,
			expectedModel: "gpt-4",
			expectError:   false,
		},
		{
			name: "with temperature",
			request: &types.ResponseRequest{
				Model:       "gpt-4",
				Input:       "Hello",
				Temperature: float64Ptr(0.7),
			},
			expectedMsgs:  1,
			expectedModel: "gpt-4",
			expectError:   false,
		},
		{
			name: "invalid input type",
			request: &types.ResponseRequest{
				Model: "gpt-4",
				Input: 123,
			},
			expectError:    true,
			expectedErrMsg: "input must be a string or an array of items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatReq, err := ResponseRequestToChatCompletion(tt.request)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedModel, chatReq.Model)
			assert.Len(t, chatReq.Messages, tt.expectedMsgs)

			if tt.request.Temperature != nil {
				assert.Equal(t, *tt.request.Temperature, *chatReq.Temperature)
			}
			if tt.request.Text != nil && tt.request.Text.Format != nil {
				require.NotNil(t, chatReq.ResponseFormat)
				assert.Equal(t, tt.request.Text.Format.Type, chatReq.ResponseFormat.Type)
			}
		})
	}
}

func TestResponseRequestToChatCompletion_WithInstructions(t *testing.T) {
	req := &types.ResponseRequest{
		Model:        "gpt-4",
		Input:        "Hello",
		Instructions: strPtr("You are a helpful assistant."),
	}

	chatReq, err := ResponseRequestToChatCompletion(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 2)

	assert.Equal(t, "system", chatReq.Messages[0].Role)
	assert.Equal(t, "You are a helpful assistant.", chatReq.Messages[0].Content)

	assert.Equal(t, "user", chatReq.Messages[1].Role)
	assert.Equal(t, "Hello", chatReq.Messages[1].Content)
}

func TestResponseRequestToChatCompletion_WithTools(t *testing.T) {
	req := &types.ResponseRequest{
		Model: "gpt-4",
		Input: "What's the weather?",
		Tools: []types.OpenAITool{
			{
				Type: "function",
				Function: types.Function{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  json.RawMessage(`{"type": "object"}`),
				},
			},
		},
	}

	chatReq, err := ResponseRequestToChatCompletion(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Tools, 1)
	assert.Equal(t, "function", chatReq.Tools[0].Type)
	assert.Equal(t, "get_weather", chatReq.Tools[0].Function.Name)
}

func TestInputItemsToMessages(t *testing.T) {
	tests := []struct {
		name         string
		items        []types.ResponseInputItem
		expectedLen  int
		expectedRole []string
	}{
		{
			name: "single user message",
			items: []types.ResponseInputItem{
				{Type: "message", Role: "user", Content: "Hello"},
			},
			expectedLen:  1,
			expectedRole: []string{"user"},
		},
		{
			name: "user and assistant messages",
			items: []types.ResponseInputItem{
				{Type: "message", Role: "user", Content: "Hello"},
				{Type: "message", Role: "assistant", Content: "Hi there!"},
			},
			expectedLen:  2,
			expectedRole: []string{"user", "assistant"},
		},
		{
			name: "function call",
			items: []types.ResponseInputItem{
				{Type: "function_call", CallID: "call_123", Name: "get_weather", Arguments: `{"location": "Paris"}`},
			},
			expectedLen:  1,
			expectedRole: []string{"assistant"},
		},
		{
			name: "function call output",
			items: []types.ResponseInputItem{
				{Type: "function_call_output", CallID: "call_123", Output: `{"temp": 72}`},
			},
			expectedLen:  1,
			expectedRole: []string{"tool"},
		},
		{
			name: "full tool loop",
			items: []types.ResponseInputItem{
				{Type: "message", Role: "user", Content: "What's the weather?"},
				{Type: "function_call", CallID: "call_123", Name: "get_weather", Arguments: `{"location": "Paris"}`},
				{Type: "function_call_output", CallID: "call_123", Output: `{"temp": 72}`},
			},
			expectedLen:  3,
			expectedRole: []string{"user", "assistant", "tool"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := InputItemsToMessages(tt.items)
			require.NoError(t, err)
			assert.Len(t, messages, tt.expectedLen)

			for i, expectedRole := range tt.expectedRole {
				assert.Equal(t, expectedRole, messages[i].Role)
			}
		})
	}
}

func TestInputItemsToMessages_FunctionCall(t *testing.T) {
	items := []types.ResponseInputItem{
		{Type: "function_call", CallID: "call_123", Name: "get_weather", Arguments: `{"location": "Paris"}`},
	}

	messages, err := InputItemsToMessages(items)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	assert.Equal(t, "assistant", messages[0].Role)
	require.Len(t, messages[0].ToolCalls, 1)
	assert.Equal(t, "call_123", messages[0].ToolCalls[0].ID)
	assert.Equal(t, "get_weather", messages[0].ToolCalls[0].Function.Name)
	assert.Equal(t, `{"location": "Paris"}`, messages[0].ToolCalls[0].Function.Arguments)
}

func TestInputItemsToMessages_FunctionCallOutput(t *testing.T) {
	items := []types.ResponseInputItem{
		{Type: "function_call_output", CallID: "call_123", Output: `{"temp": 72}`},
	}

	messages, err := InputItemsToMessages(items)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	assert.Equal(t, "tool", messages[0].Role)
	assert.Equal(t, "call_123", messages[0].ToolCallID)
	assert.Equal(t, `{"temp": 72}`, messages[0].Content)
}

func TestChatCompletionToResponse(t *testing.T) {
	content := "Hello, world!"
	chatResp := &types.ChatCompletionResponse{
		ID:      "chat_123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ResponseMessage{
					Role:    "assistant",
					Content: &content,
				},
				FinishReason: "stop",
			},
		},
		Usage: &types.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	resp := ChatCompletionToResponse(chatResp)

	assert.Equal(t, "resp_chat_123", resp.ID)
	assert.Equal(t, "response", resp.Object)
	assert.Equal(t, int64(1234567890), resp.CreatedAt)
	assert.Equal(t, "gpt-4", resp.Model)
	assert.Equal(t, "Hello, world!", resp.OutputText)
	assert.Equal(t, "completed", resp.Status)
	require.Len(t, resp.Output, 1)
	assert.Equal(t, "message", resp.Output[0].Type)
	assert.Equal(t, "assistant", resp.Output[0].Role)
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestChatCompletionToResponse_WithToolCalls(t *testing.T) {
	content := ""
	chatResp := &types.ChatCompletionResponse{
		ID:      "chat_123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ResponseMessage{
					Role:    "assistant",
					Content: &content,
					ToolCalls: []types.ToolCall{
						{
							ID:   "call_abc",
							Type: "function",
							Function: types.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "Paris"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	resp := ChatCompletionToResponse(chatResp)

	assert.Equal(t, "completed", resp.Status)
	require.Len(t, resp.Output, 1)
	assert.Equal(t, "function_call", resp.Output[0].Type)
	assert.Equal(t, "call_abc", resp.Output[0].CallID)
	assert.Equal(t, "get_weather", resp.Output[0].Name)
	assert.Equal(t, `{"location": "Paris"}`, resp.Output[0].Arguments)
}

func TestChatCompletionToResponse_WithContentAndToolCalls(t *testing.T) {
	content := "Let me check the weather for you."
	chatResp := &types.ChatCompletionResponse{
		ID:      "chat_123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ResponseMessage{
					Role:    "assistant",
					Content: &content,
					ToolCalls: []types.ToolCall{
						{
							ID:   "call_abc",
							Type: "function",
							Function: types.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location": "Paris"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	resp := ChatCompletionToResponse(chatResp)

	require.Len(t, resp.Output, 2)
	assert.Equal(t, "message", resp.Output[0].Type)
	assert.Equal(t, "function_call", resp.Output[1].Type)
	assert.Equal(t, "Let me check the weather for you.", resp.OutputText)
}

func TestChatCompletionToResponse_Incomplete(t *testing.T) {
	content := "This is a partial response..."
	chatResp := &types.ChatCompletionResponse{
		ID:      "chat_123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ResponseMessage{
					Role:    "assistant",
					Content: &content,
				},
				FinishReason: "length",
			},
		},
	}

	resp := ChatCompletionToResponse(chatResp)
	assert.Equal(t, "incomplete", resp.Status)
}

func TestComputeOutputText(t *testing.T) {
	output := []types.ResponseItem{
		{
			Type: "message",
			Content: []types.ContentOutput{
				{Type: "output_text", Text: "Hello"},
			},
		},
		{
			Type: "function_call",
			Name: "some_function",
		},
		{
			Type: "message",
			Content: []types.ContentOutput{
				{Type: "output_text", Text: "World"},
			},
		},
	}

	text := ComputeOutputText(output)
	assert.Equal(t, "Hello\nWorld", text)
}

func TestValidateStatelessRequest(t *testing.T) {
	tests := []struct {
		name        string
		request     *types.ResponseRequest
		expectError bool
		errorCode   string
	}{
		{
			name: "valid request",
			request: &types.ResponseRequest{
				Model: "gpt-4",
				Input: "Hello",
			},
			expectError: false,
		},
		{
			name: "store true",
			request: &types.ResponseRequest{
				Model: "gpt-4",
				Input: "Hello",
				Store: boolPtr(true),
			},
			expectError: true,
			errorCode:   "STATELESS_VIOLATION",
		},
		{
			name: "store false",
			request: &types.ResponseRequest{
				Model: "gpt-4",
				Input: "Hello",
				Store: boolPtr(false),
			},
			expectError: false,
		},
		{
			name: "previous_response_id",
			request: &types.ResponseRequest{
				Model:              "gpt-4",
				Input:              "Hello",
				PreviousResponseID: strPtr("resp_123"),
			},
			expectError: true,
			errorCode:   "STATELESS_VIOLATION",
		},
		{
			name: "empty previous_response_id",
			request: &types.ResponseRequest{
				Model:              "gpt-4",
				Input:              "Hello",
				PreviousResponseID: strPtr(""),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStatelessRequest(tt.request)
			if tt.expectError {
				require.Error(t, err)
				gatewayErr, ok := err.(*types.GatewayError)
				require.True(t, ok)
				assert.Equal(t, tt.errorCode, gatewayErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMessagesToInputItems(t *testing.T) {
	messages := []types.OpenAIMessage{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
	}

	items := MessagesToInputItems(messages)

	assert.Len(t, items, 2)
	assert.Equal(t, "message", items[0].Type)
	assert.Equal(t, "user", items[0].Role)
	assert.Equal(t, "message", items[1].Type)
	assert.Equal(t, "assistant", items[1].Role)
}

func TestMessagesToInputItems_WithToolCalls(t *testing.T) {
	content := ""
	messages := []types.OpenAIMessage{
		{Role: "user", Content: "What's the weather?"},
		{
			Role:    "assistant",
			Content: &content,
			ToolCalls: []types.ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					Function: types.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location": "Paris"}`,
					},
				},
			},
		},
		{Role: "tool", ToolCallID: "call_123", Content: `{"temp": 72}`},
	}

	items := MessagesToInputItems(messages)

	assert.Len(t, items, 4)
	assert.Equal(t, "message", items[0].Type)
	assert.Equal(t, "message", items[1].Type)
	assert.Equal(t, "function_call", items[2].Type)
	assert.Equal(t, "call_123", items[2].CallID)
	assert.Equal(t, "function_call_output", items[3].Type)
	assert.Equal(t, "call_123", items[3].CallID)
}

func TestFormatToolOutput(t *testing.T) {
	tests := []struct {
		name      string
		ok        bool
		errorType string
		message   string
		data      map[string]any
		expected  string
	}{
		{
			name:     "success",
			ok:       true,
			data:     map[string]any{"temperature": 72},
			expected: `{"ok":true,"temperature":72}`,
		},
		{
			name:      "error",
			ok:        false,
			errorType: "timeout",
			message:   "Service unavailable",
			data:      nil,
			expected:  `{"ok":false,"error":"timeout","message":"Service unavailable"}`,
		},
		{
			name:      "error with data",
			ok:        false,
			errorType: "api_error",
			message:   "Rate limited",
			data:      map[string]any{"retry_after": 60},
			expected:  `{"ok":false,"error":"api_error","message":"Rate limited","retry_after":60}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatToolOutput(tt.ok, tt.errorType, tt.message, tt.data)
			assert.JSONEq(t, tt.expected, result)
		})
	}
}

func TestParseToolOutput(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedOk    bool
		expectedError string
		expectedMsg   string
		expectedData  map[string]any
	}{
		{
			name:         "success",
			input:        `{"ok":true,"temperature":72}`,
			expectedOk:   true,
			expectedData: map[string]any{"temperature": float64(72)},
		},
		{
			name:          "error",
			input:         `{"ok":false,"error":"timeout","message":"Service unavailable"}`,
			expectedOk:    false,
			expectedError: "timeout",
			expectedMsg:   "Service unavailable",
		},
		{
			name:          "invalid json",
			input:         "not json",
			expectedOk:    false,
			expectedError: "parse_error",
			expectedMsg:   "Failed to parse tool output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, errType, msg, data := ParseToolOutput(tt.input)
			assert.Equal(t, tt.expectedOk, ok)
			assert.Equal(t, tt.expectedError, errType)
			assert.Equal(t, tt.expectedMsg, msg)
			if tt.expectedData != nil {
				assert.Equal(t, tt.expectedData, data)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}
