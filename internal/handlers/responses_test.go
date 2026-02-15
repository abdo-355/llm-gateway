package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockResponsesRouter struct {
	deriveRequirementsFn                 func(req types.ChatCompletionRequest, hints *types.RouterHints) types.DerivedRequirements
	generateCandidatesFn                 func() []types.RoutingCandidate
	generateCandidatesFromLogicalModelFn func(logicalModel *types.LogicalModelConfig) []types.RoutingCandidate
	filterCandidatesFn                   func(ctx context.Context, candidates []types.RoutingCandidate, requirements types.DerivedRequirements, req types.ChatCompletionRequest, hints *types.RouterHints) ([]types.RoutingCandidate, map[string]string)
	scoreCandidatesFn                    func(ctx context.Context, candidates []types.RoutingCandidate, hints *types.RouterHints) []types.RoutingCandidate
	compilePlanFn                        func(candidates []types.RoutingCandidate, hints *types.RouterHints, logicalModelSLO *types.LogicalModelSLO) types.RoutingPlan
	executeFn                            func(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) (*types.ExecutionResult, error)
	executeStreamFn                      func(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) types.StreamResult
}

func (m *mockResponsesRouter) DeriveRequirements(req types.ChatCompletionRequest, hints *types.RouterHints) types.DerivedRequirements {
	if m.deriveRequirementsFn != nil {
		return m.deriveRequirementsFn(req, hints)
	}
	return types.DerivedRequirements{Output: "text", Streaming: "forbidden", Tools: "allowed"}
}

func (m *mockResponsesRouter) GenerateCandidates() []types.RoutingCandidate {
	if m.generateCandidatesFn != nil {
		return m.generateCandidatesFn()
	}
	return []types.RoutingCandidate{{Provider: types.ProviderConfig{ID: "groq"}, Model: "llama-3.1-8b-instant", Score: 1.0}}
}

func (m *mockResponsesRouter) GenerateCandidatesFromLogicalModel(logicalModel *types.LogicalModelConfig) []types.RoutingCandidate {
	if m.generateCandidatesFromLogicalModelFn != nil {
		return m.generateCandidatesFromLogicalModelFn(logicalModel)
	}
	return []types.RoutingCandidate{{Provider: types.ProviderConfig{ID: "groq"}, Model: "llama-3.1-8b-instant", Score: 1.0}}
}

func (m *mockResponsesRouter) FilterCandidates(ctx context.Context, candidates []types.RoutingCandidate, requirements types.DerivedRequirements, req types.ChatCompletionRequest, hints *types.RouterHints) ([]types.RoutingCandidate, map[string]string) {
	if m.filterCandidatesFn != nil {
		return m.filterCandidatesFn(ctx, candidates, requirements, req, hints)
	}
	return candidates, nil
}

func (m *mockResponsesRouter) ScoreCandidates(ctx context.Context, candidates []types.RoutingCandidate, hints *types.RouterHints) []types.RoutingCandidate {
	if m.scoreCandidatesFn != nil {
		return m.scoreCandidatesFn(ctx, candidates, hints)
	}
	return candidates
}

func (m *mockResponsesRouter) CompilePlan(candidates []types.RoutingCandidate, hints *types.RouterHints, logicalModelSLO *types.LogicalModelSLO) types.RoutingPlan {
	if m.compilePlanFn != nil {
		return m.compilePlanFn(candidates, hints, logicalModelSLO)
	}
	return types.RoutingPlan{Attempts: []types.RoutingAttempt{{ProviderID: "groq", Model: "llama-3.1-8b-instant"}}, MaxAttempts: 1}
}

func (m *mockResponsesRouter) Execute(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) (*types.ExecutionResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, plan, req, requestID)
	}
	content := "Hello!"
	return &types.ExecutionResult{
		Response: types.ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: 1700000000,
			Model:   "llama-3.1-8b-instant",
			Choices: []types.Choice{{Index: 0, Message: types.ResponseMessage{Role: "assistant", Content: &content}, FinishReason: "stop"}},
			Usage:   &types.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
		Attempts:   1,
		ProviderID: "groq",
		Model:      "llama-3.1-8b-instant",
		LatencyMs:  42,
	}, nil
}

func (m *mockResponsesRouter) ExecuteStream(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) types.StreamResult {
	if m.executeStreamFn != nil {
		return m.executeStreamFn(ctx, plan, req, requestID)
	}
	chunks := make(chan *types.SSEChunk)
	errs := make(chan *types.GatewayError, 1)
	close(chunks)
	errs <- nil
	return types.StreamResult{Chunks: chunks, Err: errs}
}

func setupResponsesRouter(router ResponsesRouter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(requestid.New())
	r.POST("/v1/responses", Responses(router))
	return r
}

func validResponsesRequestBody() string {
	return `{"model":"llama-3.1-8b-instant","input":"Hello"}`
}

func TestResponses_InvalidJSON(t *testing.T) {
	router := &mockResponsesRouter{}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_FAILED", errObj["code"])
}

func TestResponses_StoreTrue(t *testing.T) {
	router := &mockResponsesRouter{}

	requestBody := `{"model":"llama-3.1-8b-instant","input":"Hello","store":true}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "STATELESS_VIOLATION", errObj["code"])
	assert.Contains(t, errObj["message"], "store=true is not supported")
}

func TestResponses_PreviousResponseID(t *testing.T) {
	router := &mockResponsesRouter{}

	requestBody := `{"model":"llama-3.1-8b-instant","input":"Hello","previous_response_id":"resp_123"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "STATELESS_VIOLATION", errObj["code"])
	assert.Contains(t, errObj["message"], "previous_response_id is not supported")
}

func TestResponses_StringInput(t *testing.T) {
	router := &mockResponsesRouter{}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(validResponsesRequestBody()))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body types.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "response", body.Object)
	assert.Equal(t, "completed", body.Status)
	assert.Equal(t, "Hello!", body.OutputText)
	require.Len(t, body.Output, 1)
	assert.Equal(t, "message", body.Output[0].Type)
	assert.Equal(t, "assistant", body.Output[0].Role)

	assert.Equal(t, "groq", w.Header().Get("X-Gateway-Provider"))
	assert.Equal(t, "llama-3.1-8b-instant", w.Header().Get("X-Gateway-Model"))
	assert.Equal(t, "1", w.Header().Get("X-Gateway-Attempts"))
}

func TestResponses_ArrayInput(t *testing.T) {
	router := &mockResponsesRouter{}

	requestBody := `{
		"model": "llama-3.1-8b-instant",
		"input": [
			{"type": "message", "role": "user", "content": "Hello"}
		]
	}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body types.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "response", body.Object)
	assert.Equal(t, "completed", body.Status)
}

func TestResponses_WithInstructions(t *testing.T) {
	router := &mockResponsesRouter{}

	requestBody := `{
		"model": "llama-3.1-8b-instant",
		"input": "Hello",
		"instructions": "You are a helpful assistant."
	}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestResponses_NoEligibleProviders(t *testing.T) {
	router := &mockResponsesRouter{
		filterCandidatesFn: func(_ context.Context, _ []types.RoutingCandidate, _ types.DerivedRequirements, _ types.ChatCompletionRequest, _ *types.RouterHints) ([]types.RoutingCandidate, map[string]string) {
			return nil, map[string]string{"groq:llama-3.1-8b-instant": "circuit_open"}
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(validResponsesRequestBody()))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "NO_ELIGIBLE_PROVIDER", errObj["code"])
}

func TestResponses_ExecuteError(t *testing.T) {
	router := &mockResponsesRouter{
		executeFn: func(_ context.Context, _ types.RoutingPlan, _ types.ChatCompletionRequest, _ string) (*types.ExecutionResult, error) {
			return nil, &types.GatewayError{
				Type:    "gateway_error",
				Code:    "RATE_LIMITED",
				Message: "rate limited by provider",
			}
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(validResponsesRequestBody()))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "RATE_LIMITED", errObj["code"])
}

func TestResponses_ExecuteGenericError(t *testing.T) {
	router := &mockResponsesRouter{
		executeFn: func(_ context.Context, _ types.RoutingPlan, _ types.ChatCompletionRequest, _ string) (*types.ExecutionResult, error) {
			return nil, fmt.Errorf("unexpected failure")
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(validResponsesRequestBody()))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "EXECUTION_ERROR", errObj["code"])
}

func TestResponses_WithToolCalls(t *testing.T) {
	router := &mockResponsesRouter{
		executeFn: func(_ context.Context, _ types.RoutingPlan, _ types.ChatCompletionRequest, _ string) (*types.ExecutionResult, error) {
			content := ""
			return &types.ExecutionResult{
				Response: types.ChatCompletionResponse{
					ID:      "chatcmpl-tool",
					Object:  "chat.completion",
					Created: 1700000000,
					Model:   "llama-3.1-8b-instant",
					Choices: []types.Choice{{
						Index: 0,
						Message: types.ResponseMessage{
							Role:    "assistant",
							Content: &content,
							ToolCalls: []types.ToolCall{{
								ID:   "call_abc123",
								Type: "function",
								Function: types.FunctionCall{
									Name:      "get_weather",
									Arguments: `{"location":"Paris"}`,
								},
							}},
						},
						FinishReason: "tool_calls",
					}},
					Usage: &types.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
				},
				Attempts:   1,
				ProviderID: "groq",
				Model:      "llama-3.1-8b-instant",
				LatencyMs:  42,
			}, nil
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(validResponsesRequestBody()))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body types.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "response", body.Object)
	require.Len(t, body.Output, 1)
	assert.Equal(t, "function_call", body.Output[0].Type)
	assert.Equal(t, "call_abc123", body.Output[0].CallID)
	assert.Equal(t, "get_weather", body.Output[0].Name)
	assert.Equal(t, `{"location":"Paris"}`, body.Output[0].Arguments)
}

func TestResponses_WithTextFormat(t *testing.T) {
	router := &mockResponsesRouter{}

	requestBody := `{
		"model": "llama-3.1-8b-instant",
		"input": "Hello",
		"text": {
			"format": {
				"type": "json_object"
			}
		}
	}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestResponses_WithTools(t *testing.T) {
	router := &mockResponsesRouter{}

	requestBody := `{
		"model": "llama-3.1-8b-instant",
		"input": "What's the weather?",
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get weather",
					"parameters": {"type": "object", "properties": {"location": {"type": "string"}}}
				}
			}
		]
	}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestResponses_InvalidInputType(t *testing.T) {
	router := &mockResponsesRouter{}

	requestBody := `{
		"model": "llama-3.1-8b-instant",
		"input": 123
	}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "INVALID_INPUT_TYPE", errObj["code"])
}

func TestResponses_FunctionCallOutputInput(t *testing.T) {
	router := &mockResponsesRouter{
		executeFn: func(_ context.Context, _ types.RoutingPlan, _ types.ChatCompletionRequest, _ string) (*types.ExecutionResult, error) {
			content := "The weather in Paris is 72°F."
			return &types.ExecutionResult{
				Response: types.ChatCompletionResponse{
					ID:      "chatcmpl-result",
					Object:  "chat.completion",
					Created: 1700000000,
					Model:   "llama-3.1-8b-instant",
					Choices: []types.Choice{{
						Index: 0,
						Message: types.ResponseMessage{
							Role:    "assistant",
							Content: &content,
						},
						FinishReason: "stop",
					}},
					Usage: &types.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
				},
				Attempts:   1,
				ProviderID: "groq",
				Model:      "llama-3.1-8b-instant",
				LatencyMs:  42,
			}, nil
		},
	}

	requestBody := `{
		"model": "llama-3.1-8b-instant",
		"input": [
			{"type": "message", "role": "user", "content": "What's the weather?"},
			{"type": "function_call", "call_id": "call_123", "name": "get_weather", "arguments": "{\"location\":\"Paris\"}"},
			{"type": "function_call_output", "call_id": "call_123", "output": "{\"temp\": 72}"}
		]
	}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body types.Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "response", body.Object)
	assert.Equal(t, "The weather in Paris is 72°F.", body.OutputText)
}

func TestResponses_StoreFalse(t *testing.T) {
	router := &mockResponsesRouter{}

	requestBody := `{
		"model": "llama-3.1-8b-instant",
		"input": "Hello",
		"store": false
	}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestResponses_IncludeField(t *testing.T) {
	router := &mockResponsesRouter{}

	requestBody := `{
		"model": "llama-3.1-8b-instant",
		"input": "Hello",
		"include": ["reasoning.encrypted_content"]
	}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	setupResponsesRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
