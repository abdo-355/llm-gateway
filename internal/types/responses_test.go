package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseRequestUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ResponseRequest
	}{
		{
			name: "string input",
			input: `{
				"model": "gpt-4",
				"input": "Hello, world!"
			}`,
			expected: ResponseRequest{
				Model: "gpt-4",
				Input: "Hello, world!",
			},
		},
		{
			name: "array input with messages",
			input: `{
				"model": "gpt-4",
				"input": [
					{"type": "message", "role": "user", "content": "Hello"}
				]
			}`,
			expected: ResponseRequest{
				Model: "gpt-4",
			},
		},
		{
			name: "with instructions",
			input: `{
				"model": "gpt-4",
				"input": "Hello",
				"instructions": "You are a helpful assistant."
			}`,
			expected: ResponseRequest{
				Model:        "gpt-4",
				Input:        "Hello",
				Instructions: strPtr("You are a helpful assistant."),
			},
		},
		{
			name: "with tools",
			input: `{
				"model": "gpt-4",
				"input": "What's the weather?",
				"tools": [
					{
						"type": "function",
						"function": {
							"name": "get_weather",
							"description": "Get weather",
							"parameters": {"type": "object"}
						}
					}
				]
			}`,
			expected: ResponseRequest{
				Model: "gpt-4",
				Input: "What's the weather?",
				Tools: []OpenAITool{
					{
						Type: "function",
						Function: Function{
							Name:        "get_weather",
							Description: "Get weather",
							Parameters:  json.RawMessage(`{"type": "object"}`),
						},
					},
				},
			},
		},
		{
			name: "with text format",
			input: `{
				"model": "gpt-4",
				"input": "Hello",
				"text": {
					"format": {
						"type": "json_object"
					}
				}
			}`,
			expected: ResponseRequest{
				Model: "gpt-4",
				Input: "Hello",
				Text: &TextConfig{
					Format: &ResponseFormat{
						Type: "json_object",
					},
				},
			},
		},
		{
			name: "with temperature",
			input: `{
				"model": "gpt-4",
				"input": "Hello",
				"temperature": 0.7
			}`,
			expected: ResponseRequest{
				Model:       "gpt-4",
				Input:       "Hello",
				Temperature: float64Ptr(0.7),
			},
		},
		{
			name: "with store true",
			input: `{
				"model": "gpt-4",
				"input": "Hello",
				"store": true
			}`,
			expected: ResponseRequest{
				Model: "gpt-4",
				Input: "Hello",
				Store: boolPtr(true),
			},
		},
		{
			name: "with previous_response_id",
			input: `{
				"model": "gpt-4",
				"input": "Hello",
				"previous_response_id": "resp_123"
			}`,
			expected: ResponseRequest{
				Model:              "gpt-4",
				Input:              "Hello",
				PreviousResponseID: strPtr("resp_123"),
			},
		},
		{
			name: "with include",
			input: `{
				"model": "gpt-4",
				"input": "Hello",
				"include": ["reasoning.encrypted_content"]
			}`,
			expected: ResponseRequest{
				Model:   "gpt-4",
				Input:   "Hello",
				Include: []string{"reasoning.encrypted_content"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req ResponseRequest
			err := json.Unmarshal([]byte(tt.input), &req)
			require.NoError(t, err)
			assert.Equal(t, tt.expected.Model, req.Model)
			if tt.expected.Instructions != nil {
				assert.Equal(t, *tt.expected.Instructions, *req.Instructions)
			}
			if tt.expected.Store != nil {
				assert.Equal(t, *tt.expected.Store, *req.Store)
			}
			if tt.expected.Temperature != nil {
				assert.Equal(t, *tt.expected.Temperature, *req.Temperature)
			}
			if tt.expected.Text != nil {
				assert.NotNil(t, req.Text)
				assert.Equal(t, tt.expected.Text.Format.Type, req.Text.Format.Type)
			}
			if tt.expected.Include != nil {
				assert.Equal(t, tt.expected.Include, req.Include)
			}
		})
	}
}

func TestResponseInputItem(t *testing.T) {
	tests := []struct {
		name         string
		item         ResponseInputItem
		isMessage    bool
		isFuncCall   bool
		isFuncOutput bool
	}{
		{
			name:         "message item",
			item:         ResponseInputItem{Type: "message", Role: "user"},
			isMessage:    true,
			isFuncCall:   false,
			isFuncOutput: false,
		},
		{
			name:         "function_call item",
			item:         ResponseInputItem{Type: "function_call", CallID: "call_123"},
			isMessage:    false,
			isFuncCall:   true,
			isFuncOutput: false,
		},
		{
			name:         "function_call_output item",
			item:         ResponseInputItem{Type: "function_call_output", CallID: "call_123"},
			isMessage:    false,
			isFuncCall:   false,
			isFuncOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isMessage, tt.item.IsMessage())
			assert.Equal(t, tt.isFuncCall, tt.item.IsFunctionCall())
			assert.Equal(t, tt.isFuncOutput, tt.item.IsFunctionCallOutput())
		})
	}
}

func TestParseResponseInput(t *testing.T) {
	tests := []struct {
		name        string
		input       any
		expectedLen int
		expectError bool
	}{
		{
			name:        "string input",
			input:       "Hello, world!",
			expectedLen: 1,
			expectError: false,
		},
		{
			name: "array of items",
			input: []any{
				map[string]any{"type": "message", "role": "user", "content": "Hello"},
				map[string]any{"type": "function_call", "call_id": "call_123", "name": "test"},
			},
			expectedLen: 2,
			expectError: false,
		},
		{
			name:        "invalid type",
			input:       123,
			expectedLen: 0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := ParseResponseInput(tt.input)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, items, tt.expectedLen)
		})
	}
}

func TestParseResponseInputString(t *testing.T) {
	items, err := ParseResponseInput("Hello")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "message", items[0].Type)
	assert.Equal(t, "user", items[0].Role)
	assert.Equal(t, "Hello", items[0].Content)
}

func TestParseResponseInputArray(t *testing.T) {
	input := []any{
		map[string]any{
			"type":    "message",
			"role":    "user",
			"content": "Hello",
		},
		map[string]any{
			"type":      "function_call",
			"call_id":   "call_123",
			"name":      "get_weather",
			"arguments": `{"location": "Paris"}`,
		},
		map[string]any{
			"type":    "function_call_output",
			"call_id": "call_123",
			"output":  `{"temp": 72}`,
		},
	}

	items, err := ParseResponseInput(input)
	require.NoError(t, err)
	require.Len(t, items, 3)

	assert.Equal(t, "message", items[0].Type)
	assert.Equal(t, "user", items[0].Role)
	assert.Equal(t, "Hello", items[0].Content)

	assert.Equal(t, "function_call", items[1].Type)
	assert.Equal(t, "call_123", items[1].CallID)
	assert.Equal(t, "get_weather", items[1].Name)

	assert.Equal(t, "function_call_output", items[2].Type)
	assert.Equal(t, "call_123", items[2].CallID)
}

func TestResponseMarshal(t *testing.T) {
	resp := Response{
		ID:         "resp_123",
		Object:     "response",
		CreatedAt:  1234567890,
		Model:      "gpt-4",
		OutputText: "Hello, world!",
		Status:     "completed",
		Output: []ResponseItem{
			{
				ID:     "msg_123",
				Type:   "message",
				Role:   "assistant",
				Status: "completed",
				Content: []ContentOutput{
					{Type: "output_text", Text: "Hello, world!"},
				},
			},
		},
		Usage: &Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	bytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var unmarshaled Response
	err = json.Unmarshal(bytes, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, resp.ID, unmarshaled.ID)
	assert.Equal(t, resp.Object, unmarshaled.Object)
	assert.Equal(t, resp.Model, unmarshaled.Model)
	assert.Equal(t, resp.OutputText, unmarshaled.OutputText)
	assert.Equal(t, resp.Status, unmarshaled.Status)
	assert.Len(t, unmarshaled.Output, 1)
}

func TestResponseItemWithFunctionCall(t *testing.T) {
	item := ResponseItem{
		ID:        "fc_123",
		Type:      "function_call",
		CallID:    "call_abc",
		Name:      "get_weather",
		Arguments: `{"location": "Paris"}`,
		Status:    "completed",
	}

	bytes, err := json.Marshal(item)
	require.NoError(t, err)

	var unmarshaled ResponseItem
	err = json.Unmarshal(bytes, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, "function_call", unmarshaled.Type)
	assert.Equal(t, "call_abc", unmarshaled.CallID)
	assert.Equal(t, "get_weather", unmarshaled.Name)
	assert.Equal(t, `{"location": "Paris"}`, unmarshaled.Arguments)
}

func TestResponseItemWithFunctionCallOutput(t *testing.T) {
	item := ResponseItem{
		ID:     "fco_123",
		Type:   "function_call_output",
		CallID: "call_abc",
		Output: `{"temperature": 72, "unit": "F"}`,
		Status: "completed",
	}

	bytes, err := json.Marshal(item)
	require.NoError(t, err)

	var unmarshaled ResponseItem
	err = json.Unmarshal(bytes, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, "function_call_output", unmarshaled.Type)
	assert.Equal(t, "call_abc", unmarshaled.CallID)
}

func TestResponseError(t *testing.T) {
	resp := Response{
		ID:     "resp_123",
		Object: "response",
		Model:  "gpt-4",
		Status: "failed",
		Error: &ResponseError{
			Type:    "server_error",
			Code:    "INTERNAL_ERROR",
			Message: "Something went wrong",
		},
	}

	bytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var unmarshaled Response
	err = json.Unmarshal(bytes, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, "failed", unmarshaled.Status)
	require.NotNil(t, unmarshaled.Error)
	assert.Equal(t, "server_error", unmarshaled.Error.Type)
	assert.Equal(t, "INTERNAL_ERROR", unmarshaled.Error.Code)
}

func strPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

func boolPtr(b bool) *bool {
	return &b
}
