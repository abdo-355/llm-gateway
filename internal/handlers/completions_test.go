package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Setenv("GATEWAY_API_KEY", "test-api-key-that-is-at-least-32-characters-long")
	os.Setenv("GROQ_API_KEY", "test-groq-key")
	os.Setenv("CEREBRAS_API_KEY", "test-cerebras-key")
	os.Setenv("MISTRAL_API_KEY", "test-mistral-key")
	os.Setenv("GEMINI_API_KEY", "test-gemini-key")
	os.Exit(m.Run())
}

type mockRouter struct {
	deriveRequirementsFn                 func(req types.ChatCompletionRequest, hints *types.RouterHints) types.DerivedRequirements
	generateCandidatesFn                 func() []types.RoutingCandidate
	generateCandidatesFromLogicalModelFn func(logicalModel *types.LogicalModelConfig) []types.RoutingCandidate
	filterCandidatesFn                   func(ctx context.Context, candidates []types.RoutingCandidate, requirements types.DerivedRequirements, req types.ChatCompletionRequest, hints *types.RouterHints) ([]types.RoutingCandidate, map[string]string)
	scoreCandidatesFn                    func(ctx context.Context, candidates []types.RoutingCandidate, hints *types.RouterHints) []types.RoutingCandidate
	compilePlanFn                        func(candidates []types.RoutingCandidate, hints *types.RouterHints, logicalModelSLO *types.LogicalModelSLO) types.RoutingPlan
	executeFn                            func(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) (*types.ExecutionResult, error)
	executeStreamFn                      func(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) types.StreamResult
}

func (m *mockRouter) DeriveRequirements(req types.ChatCompletionRequest, hints *types.RouterHints) types.DerivedRequirements {
	if m.deriveRequirementsFn != nil {
		return m.deriveRequirementsFn(req, hints)
	}
	return types.DerivedRequirements{Output: "text", Streaming: "forbidden", Tools: "allowed"}
}

func (m *mockRouter) GenerateCandidates() []types.RoutingCandidate {
	if m.generateCandidatesFn != nil {
		return m.generateCandidatesFn()
	}
	return []types.RoutingCandidate{{Provider: types.ProviderConfig{ID: "groq"}, Model: "llama-3.1-8b-instant", Score: 1.0}}
}

func (m *mockRouter) GenerateCandidatesFromLogicalModel(logicalModel *types.LogicalModelConfig) []types.RoutingCandidate {
	if m.generateCandidatesFromLogicalModelFn != nil {
		return m.generateCandidatesFromLogicalModelFn(logicalModel)
	}
	return []types.RoutingCandidate{{Provider: types.ProviderConfig{ID: "groq"}, Model: "llama-3.1-8b-instant", Score: 1.0}}
}

func (m *mockRouter) FilterCandidates(ctx context.Context, candidates []types.RoutingCandidate, requirements types.DerivedRequirements, req types.ChatCompletionRequest, hints *types.RouterHints) ([]types.RoutingCandidate, map[string]string) {
	if m.filterCandidatesFn != nil {
		return m.filterCandidatesFn(ctx, candidates, requirements, req, hints)
	}
	return candidates, nil
}

func (m *mockRouter) ScoreCandidates(ctx context.Context, candidates []types.RoutingCandidate, hints *types.RouterHints) []types.RoutingCandidate {
	if m.scoreCandidatesFn != nil {
		return m.scoreCandidatesFn(ctx, candidates, hints)
	}
	return candidates
}

func (m *mockRouter) CompilePlan(candidates []types.RoutingCandidate, hints *types.RouterHints, logicalModelSLO *types.LogicalModelSLO) types.RoutingPlan {
	if m.compilePlanFn != nil {
		return m.compilePlanFn(candidates, hints, logicalModelSLO)
	}
	return types.RoutingPlan{Attempts: []types.RoutingAttempt{{ProviderID: "groq", Model: "llama-3.1-8b-instant"}}, MaxAttempts: 1}
}

func (m *mockRouter) Execute(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) (*types.ExecutionResult, error) {
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

func (m *mockRouter) ExecuteStream(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) types.StreamResult {
	if m.executeStreamFn != nil {
		return m.executeStreamFn(ctx, plan, req, requestID)
	}
	chunks := make(chan *types.SSEChunk)
	errs := make(chan *types.GatewayError, 1)
	close(chunks)
	errs <- nil
	return types.StreamResult{Chunks: chunks, Err: errs}
}

func setupCompletionsRouter(router services.RouterHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(requestid.New())
	handler := NewCompletionsHandler(router)
	r.POST("/v1/chat/completions", handler.Handle)
	return r
}

func validRequestBody() string {
	return `{"model":"llama-3.1-8b-instant","messages":[{"role":"user","content":"Hello"}]}`
}

func TestCompletions_InvalidJSON(t *testing.T) {
	router := &mockRouter{}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	setupCompletionsRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_FAILED", errObj["code"])
}

func TestCompletions_NoEligibleProviders(t *testing.T) {
	router := &mockRouter{
		filterCandidatesFn: func(_ context.Context, _ []types.RoutingCandidate, _ types.DerivedRequirements, _ types.ChatCompletionRequest, _ *types.RouterHints) ([]types.RoutingCandidate, map[string]string) {
			return nil, map[string]string{"groq:llama-3.1-8b-instant": "circuit_open"}
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(validRequestBody()))
	req.Header.Set("Content-Type", "application/json")
	setupCompletionsRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "NO_ELIGIBLE_PROVIDER", errObj["code"])
}

func TestCompletions_SuccessfulRequest(t *testing.T) {
	router := &mockRouter{}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(validRequestBody()))
	req.Header.Set("Content-Type", "application/json")
	setupCompletionsRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body types.ChatCompletionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "chatcmpl-test", body.ID)
	assert.Equal(t, "chat.completion", body.Object)
	require.Len(t, body.Choices, 1)
	assert.Equal(t, "stop", body.Choices[0].FinishReason)

	assert.Equal(t, "groq", w.Header().Get("X-Gateway-Provider"))
	assert.Equal(t, "llama-3.1-8b-instant", w.Header().Get("X-Gateway-Model"))
	assert.Equal(t, "1", w.Header().Get("X-Gateway-Attempts"))
	assert.Equal(t, "15", w.Header().Get("X-Gateway-Tokens-Used"))
}

func TestCompletions_ExecuteError(t *testing.T) {
	router := &mockRouter{
		executeFn: func(_ context.Context, _ types.RoutingPlan, _ types.ChatCompletionRequest, _ string) (*types.ExecutionResult, error) {
			return nil, &types.GatewayError{
				Type:    "gateway_error",
				Code:    "RATE_LIMITED",
				Message: "rate limited by provider",
			}
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(validRequestBody()))
	req.Header.Set("Content-Type", "application/json")
	setupCompletionsRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "RATE_LIMITED", errObj["code"])
}

func TestCompletions_ExecuteGenericError(t *testing.T) {
	router := &mockRouter{
		executeFn: func(_ context.Context, _ types.RoutingPlan, _ types.ChatCompletionRequest, _ string) (*types.ExecutionResult, error) {
			return nil, fmt.Errorf("unexpected failure")
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(validRequestBody()))
	req.Header.Set("Content-Type", "application/json")
	setupCompletionsRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "EXECUTION_ERROR", errObj["code"])
}

func TestCompletions_LogicalModelResolution(t *testing.T) {
	logicalModelCalled := false
	router := &mockRouter{
		generateCandidatesFromLogicalModelFn: func(lm *types.LogicalModelConfig) []types.RoutingCandidate {
			logicalModelCalled = true
			assert.Equal(t, "chat-lite", lm.ID)
			return []types.RoutingCandidate{{Provider: types.ProviderConfig{ID: "groq"}, Model: "llama-3.1-8b-instant", Score: 1.0}}
		},
	}

	body := `{"model":"chat-lite","messages":[{"role":"user","content":"Hi"}]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	setupCompletionsRouter(router).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, logicalModelCalled, "should route through GenerateCandidatesFromLogicalModel")
	assert.Equal(t, "chat-lite", w.Header().Get("X-Gateway-Logical-Model"))
}
