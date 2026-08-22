package services_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/services/mocks"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

type cloudflareQuotaStub struct {
	estimateTokens           int
	estimatedCloudflareUnits int
	cloudflareErr            error
	markedExhausted          bool
	concurrencyUsage         map[string]int
}

func (s *cloudflareQuotaStub) EstimateTokens(req types.ChatCompletionRequest) int {
	return s.estimateTokens
}

func (s *cloudflareQuotaStub) CheckModelQuota(ctx context.Context, providerID, model string, limits types.ModelLimits, estimatedTokens int) error {
	return nil
}

func (s *cloudflareQuotaStub) RecordModelUsage(ctx context.Context, providerID, model string, tokensUsed int) error {
	return nil
}

func (s *cloudflareQuotaStub) HandleProviderRateLimit(ctx context.Context, providerID, model string, resp *http.Response) services.RateLimitInfo {
	return services.RateLimitInfo{}
}

func (s *cloudflareQuotaStub) AcquireConcurrencySlot(ctx context.Context, providerID, model string, maxConcurrent int) error {
	return nil
}

func (s *cloudflareQuotaStub) ReleaseConcurrencySlot(ctx context.Context, providerID, model string) {}

func (s *cloudflareQuotaStub) CheckConcurrencyLimit(ctx context.Context, providerID, model string, maxConcurrent int) bool {
	return true
}

func (s *cloudflareQuotaStub) GetConcurrencyUsage(ctx context.Context, providerID, model string) (int, error) {
	return s.concurrencyUsage[providerID+"/"+model], nil
}

func (s *cloudflareQuotaStub) GetModelQuotaStatus(ctx context.Context, providerID, model string, limits *types.ModelLimits) services.QuotaStatus {
	return services.QuotaStatus{}
}

func (s *cloudflareQuotaStub) EstimateCloudflareRequestNeurons(model string, req types.ChatCompletionRequest) int {
	return s.estimatedCloudflareUnits
}

func (s *cloudflareQuotaStub) CheckCloudflareDailyNeuronBudget(ctx context.Context, model string, estimatedNeurons int) error {
	return s.cloudflareErr
}

func (s *cloudflareQuotaStub) RecordCloudflareNeuronUsage(ctx context.Context, model string, usage *types.Usage) (services.CloudflareUsageStats, error) {
	return services.CloudflareUsageStats{}, nil
}

func (s *cloudflareQuotaStub) MarkCloudflareDailyBudgetExhausted(ctx context.Context) error {
	s.markedExhausted = true
	return nil
}

func init() {
	os.Setenv("GATEWAY_API_KEY", "test-api-key-that-is-at-least-32-characters-long")
	os.Setenv("GROQ_API_KEY", "test-groq-key")
}

func testConfig() types.AppConfig {
	return types.AppConfig{
		Providers: []types.ProviderConfig{
			{
				ID:      "provider-a",
				BaseURL: "https://api.provider-a.com/v1",
				Auth:    types.ProviderAuth{Type: "bearer", Env: "GROQ_API_KEY"},
				Models: types.ProviderModels{
					Mode: "allowlist",
					List: []string{"model-1", "model-2"},
					Limits: map[string]types.ModelLimits{
						"model-1": {Rpm: intPtr(30)},
						"model-2": {Rpm: intPtr(60)},
					},
				},
				Capabilities: types.ProviderCapabilities{
					Streaming:           true,
					Tools:               true,
					StructuredOutputs:   "json_schema_strict",
					Logprobs:            true,
					Metadata:            true,
					Seed:                true,
					User:                true,
					FrequencyPenalty:    true,
					PresencePenalty:     true,
					MaxTokens:           true,
					MaxCompletionTokens: true,
					MultipleChoices:     true,
					ToolSchema:          "json_schema",
				},
			},
			{
				ID:      "provider-b",
				BaseURL: "https://api.provider-b.com/v1",
				Models: types.ProviderModels{
					Mode: "allowlist",
					List: []string{"model-3"},
					Limits: map[string]types.ModelLimits{
						"model-3": {Rpm: intPtr(30)},
					},
				},
				Capabilities: types.ProviderCapabilities{
					Streaming:           false,
					Tools:               false,
					StructuredOutputs:   "none",
					Logprobs:            false,
					Metadata:            false,
					Seed:                false,
					User:                false,
					FrequencyPenalty:    false,
					PresencePenalty:     false,
					MaxTokens:           false,
					MaxCompletionTokens: false,
					MultipleChoices:     false,
					ToolSchema:          "json_schema",
				},
			},
		},
		Certifications: []types.Certification{
			{Provider: "provider-a", Model: "model-1", StrictSchema: true},
		},
	}
}

func newTestRouter(t *testing.T) (*services.Router, *mocks.MockQuotaChecker, *mocks.MockHealthChecker, *mocks.MockProviderCaller) {
	ctrl := gomock.NewController(t)
	mockQuota := mocks.NewMockQuotaChecker(ctrl)
	mockHealth := mocks.NewMockHealthChecker(ctrl)
	mockProvider := mocks.NewMockProviderCaller(ctrl)
	mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
	mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
	mockQuota.EXPECT().GetModelQuotaStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(services.QuotaStatus{}).AnyTimes()
	r := services.NewRouterWithConfig(testConfig(), mockQuota, mockHealth, mockProvider)
	return r, mockQuota, mockHealth, mockProvider
}

func newExternalTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

func testCooldownConfig() config.CooldownConfig {
	return config.CooldownConfig{
		Enabled:                  true,
		DefaultDuration:          30 * time.Second,
		RateLimitDuration:        5 * time.Second,
		PaymentDuration:          5 * time.Minute,
		Error5xxDuration:         30 * time.Second,
		StructuredOutputDuration: 30 * time.Second,
		MaxRetryAfterDuration:    24 * time.Hour,
	}
}

func streamResult(err *types.GatewayError, chunks ...*types.SSEChunk) types.StreamResult {
	chunkCh := make(chan *types.SSEChunk)
	errCh := make(chan *types.GatewayError, 1)
	go func() {
		defer close(chunkCh)
		defer close(errCh)
		for _, chunk := range chunks {
			chunkCh <- chunk
		}
		errCh <- err
	}()
	return types.StreamResult{Chunks: chunkCh, Err: errCh}
}

// --- DeriveRequirements ---

func TestDeriveRequirements(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	t.Run("defaults", func(t *testing.T) {
		req := types.ChatCompletionRequest{}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "text", reqs.Output)
		assert.Equal(t, "preferred", reqs.Streaming)
		assert.Equal(t, "forbidden", reqs.Tools)
	})

	t.Run("strict JSON from response_format", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			ResponseFormat: &types.ResponseFormat{
				Type: "json_schema",
				JSONSchema: &types.JSONSchema{
					Name:   "test",
					Strict: boolPtr(true),
				},
			},
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "json_schema_strict", reqs.Output)
	})

	t.Run("JSON object from response_format", func(t *testing.T) {
		req := types.ChatCompletionRequest{ResponseFormat: &types.ResponseFormat{Type: "json_object"}}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "json_object", reqs.Output)
	})

	t.Run("non-strict JSON schema from response_format", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			ResponseFormat: &types.ResponseFormat{
				Type: "json_schema",
				JSONSchema: &types.JSONSchema{
					Name:   "test",
					Strict: boolPtr(false),
				},
			},
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "json_schema", reqs.Output)
	})

	t.Run("streaming required", func(t *testing.T) {
		req := types.ChatCompletionRequest{Stream: boolPtr(true)}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "required", reqs.Streaming)
	})

	t.Run("streaming forbidden", func(t *testing.T) {
		req := types.ChatCompletionRequest{Stream: boolPtr(false)}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "forbidden", reqs.Streaming)
	})

	t.Run("tools required", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			Tools:      []types.OpenAITool{{Type: "function"}},
			ToolChoice: "required",
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "required", reqs.Tools)
	})

	t.Run("tools allowed", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			Tools:      []types.OpenAITool{{Type: "function"}},
			ToolChoice: "auto",
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "allowed", reqs.Tools)
	})

	t.Run("tools forbidden via none", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			Tools:      []types.OpenAITool{{Type: "function"}},
			ToolChoice: "none",
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "forbidden", reqs.Tools)
	})

	t.Run("router hints override", func(t *testing.T) {
		req := types.ChatCompletionRequest{Stream: boolPtr(true)}
		hints := &types.RouterHints{
			Requirements: &types.RouterRequirements{
				Output:    strPtr("json_schema_strict"),
				Streaming: strPtr("forbidden"),
				Tools:     strPtr("required"),
			},
		}
		reqs := r.DeriveRequirements(req, hints)
		assert.Equal(t, "json_schema_strict", reqs.Output)
		assert.Equal(t, "forbidden", reqs.Streaming)
		assert.Equal(t, "required", reqs.Tools)
	})

}

// --- GenerateCandidates ---

func TestGenerateCandidates(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	candidates := r.GenerateCandidates()
	assert.Len(t, candidates, 3)
	assert.True(t, candidates[0].IsCertifiedForStrictSchema)
	assert.Equal(t, "model-1", candidates[0].Model)
	assert.False(t, candidates[1].IsCertifiedForStrictSchema)
	assert.False(t, candidates[2].IsCertifiedForStrictSchema)
}

// --- FilterCandidates ---

func TestFilterCandidates(t *testing.T) {
	ctx := context.Background()
	baseReq := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "hi"}}}
	textReqs := types.DerivedRequirements{Output: "text", Streaming: "preferred", Tools: "forbidden"}

	t.Run("passes all when no filters apply", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, textReqs, baseReq, nil)
		assert.Len(t, eligible, 3)
		assert.Empty(t, filtered)
	})

	t.Run("filters by allow list", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		hints := &types.RouterHints{Providers: &types.ProviderPreferences{Allow: []string{"provider-a"}}}
		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, textReqs, baseReq, hints)
		assert.Len(t, eligible, 2)
		assert.Contains(t, filtered, "provider-b/model-3")
	})

	t.Run("filters disabled providers", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		disabler := services.NewProviderDisabler(1, nil)
		disabler.RecordAuthFailure("provider-a", "model-1")
		r.SetProviderDisabler(disabler)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, textReqs, baseReq, nil)
		assert.Len(t, eligible, 1)
		assert.Equal(t, "provider-b", eligible[0].Provider.ID)
		assert.Equal(t, "provider_unavailable", filtered["provider-a/model-1"])
		assert.Equal(t, "provider_unavailable", filtered["provider-a/model-2"])
	})

	t.Run("filters by strict JSON requirement", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		strictReqs := types.DerivedRequirements{Output: "json_schema_strict", Streaming: "preferred", Tools: "forbidden"}
		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, strictReqs, baseReq, nil)
		assert.Len(t, eligible, 2)
		assert.Contains(t, filtered, "provider-b/model-3")
	})

	t.Run("allows non-strict JSON schema providers for strict contract", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockQuota := mocks.NewMockQuotaChecker(ctrl)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)

		cfg := types.AppConfig{Providers: []types.ProviderConfig{
			{
				ID:      "schema-provider",
				BaseURL: "https://schema.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"schema-model"}},
				Capabilities: types.ProviderCapabilities{
					StructuredOutputs: "json_schema",
				},
			},
			{
				ID:      "text-provider",
				BaseURL: "https://text.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"text-model"}},
				Capabilities: types.ProviderCapabilities{
					StructuredOutputs: "none",
				},
			},
		}}

		r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		strictReqs := types.DerivedRequirements{Output: "json_schema_strict", Streaming: "preferred", Tools: "forbidden"}
		eligible, filtered := r.FilterCandidates(ctx, r.GenerateCandidates(), strictReqs, baseReq, nil)
		require.Len(t, eligible, 1)
		assert.Equal(t, "schema-provider", eligible[0].Provider.ID)
		assert.Equal(t, "not_certified_for_strict_json", filtered["text-provider/text-model"])
	})

	t.Run("allows JSON object providers for strict contract", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockQuota := mocks.NewMockQuotaChecker(ctrl)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)

		cfg := types.AppConfig{Providers: []types.ProviderConfig{
			{
				ID:      "json-provider",
				BaseURL: "https://json.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"json-model"}},
				Capabilities: types.ProviderCapabilities{
					StructuredOutputs: "json_object",
				},
			},
			{
				ID:      "text-provider",
				BaseURL: "https://text.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"text-model"}},
				Capabilities: types.ProviderCapabilities{
					StructuredOutputs: "none",
				},
			},
		}}

		r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		strictReqs := types.DerivedRequirements{Output: "json_schema_strict", Streaming: "preferred", Tools: "forbidden"}
		eligible, filtered := r.FilterCandidates(ctx, r.GenerateCandidates(), strictReqs, baseReq, nil)
		require.Len(t, eligible, 1)
		assert.Equal(t, "json-provider", eligible[0].Provider.ID)
		assert.Equal(t, "not_certified_for_strict_json", filtered["text-provider/text-model"])
	})

	t.Run("filters JSON object fallback for strict streaming", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockQuota := mocks.NewMockQuotaChecker(ctrl)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)

		cfg := types.AppConfig{Providers: []types.ProviderConfig{
			{
				ID:      "json-provider",
				BaseURL: "https://json.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"json-model"}},
				Capabilities: types.ProviderCapabilities{
					Streaming:         true,
					StructuredOutputs: "json_object",
				},
			},
			{
				ID:      "schema-provider",
				BaseURL: "https://schema.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"schema-model"}},
				Capabilities: types.ProviderCapabilities{
					Streaming:         true,
					StructuredOutputs: "json_schema",
				},
			},
		}}

		r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		strictStreamingReqs := types.DerivedRequirements{Output: "json_schema_strict", Streaming: "required", Tools: "forbidden"}
		eligible, filtered := r.FilterCandidates(ctx, r.GenerateCandidates(), strictStreamingReqs, baseReq, nil)
		require.Len(t, eligible, 1)
		assert.Equal(t, "schema-provider", eligible[0].Provider.ID)
		assert.Equal(t, "strict_streaming_requires_schema_support", filtered["json-provider/json-model"])
	})

	t.Run("filters JSON object to structured output providers", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		jsonReqs := types.DerivedRequirements{Output: "json_object", Streaming: "preferred", Tools: "forbidden"}
		jsonReq := baseReq
		jsonReq.ResponseFormat = &types.ResponseFormat{Type: "json_object"}

		eligible, filtered := r.FilterCandidates(ctx, r.GenerateCandidates(), jsonReqs, jsonReq, nil)
		assert.Len(t, eligible, 2)
		assert.Contains(t, filtered, "provider-b/model-3")
		assert.Equal(t, "json_object_not_supported", filtered["provider-b/model-3"])
	})

	t.Run("filters by streaming requirement", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		streamReqs := types.DerivedRequirements{Output: "text", Streaming: "required", Tools: "forbidden"}
		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, streamReqs, baseReq, nil)
		assert.Len(t, eligible, 2)
		assert.Contains(t, filtered, "provider-b/model-3")
	})

	t.Run("filters by tools requirement", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		toolsReqs := types.DerivedRequirements{Output: "text", Streaming: "preferred", Tools: "required"}
		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, toolsReqs, baseReq, nil)
		assert.Len(t, eligible, 2)
		assert.Contains(t, filtered, "provider-b/model-3")
	})

	t.Run("filters by circuit breaker open", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), "provider-a", gomock.Any()).Return(false).AnyTimes()
		mockHealth.EXPECT().CanExecute(gomock.Any(), "provider-b", gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, textReqs, baseReq, nil)
		assert.Len(t, eligible, 1)
		assert.Equal(t, "provider-b", eligible[0].Provider.ID)
		assert.Contains(t, filtered, "provider-a/model-1")
		assert.Contains(t, filtered, "provider-a/model-2")
	})

	t.Run("filters by quota exceeded", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), "provider-a", "model-1", gomock.Any(), gomock.Any()).
			Return(errors.NewModelQuotaExceededError("RPM exceeded", "provider-a", "model-1", "rpm"))
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), "provider-a", "model-2", gomock.Any(), gomock.Any()).Return(nil)
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), "provider-b", "model-3", gomock.Any(), gomock.Any()).Return(nil)

		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, textReqs, baseReq, nil)
		assert.Len(t, eligible, 2)
		assert.Contains(t, filtered, "provider-a/model-1")
		assert.Equal(t, "quota_exceeded_rpm", filtered["provider-a/model-1"])
	})

	t.Run("filters by logprobs requirement", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		logprobReq := baseReq
		logprobReq.Logprobs = boolPtr(true)
		logprobReq.TopLogprobs = intPtr(1)

		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, textReqs, logprobReq, nil)
		assert.Len(t, eligible, 2)
		assert.Contains(t, filtered, "provider-b/model-3")
		assert.Equal(t, "logprobs_not_supported", filtered["provider-b/model-3"])
	})

	t.Run("allows cerebras json_object streaming via schema support", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockQuota := mocks.NewMockQuotaChecker(ctrl)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)

		cfg := types.AppConfig{Providers: []types.ProviderConfig{{
			ID:      "cerebras",
			BaseURL: "https://api.cerebras.ai/v1",
			Models:  types.ProviderModels{Mode: "allowlist", List: []string{"model-1"}},
			Capabilities: types.ProviderCapabilities{
				Streaming:           true,
				Tools:               true,
				StructuredOutputs:   "json_schema_strict",
				MaxCompletionTokens: true,
				ToolSchema:          "json_schema",
			},
		}}}

		r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		streamReq := baseReq
		streamReq.Stream = boolPtr(true)
		streamReq.ResponseFormat = &types.ResponseFormat{Type: "json_object"}

		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, types.DerivedRequirements{Output: "text", Streaming: "required", Tools: "forbidden"}, streamReq, nil)
		assert.Len(t, eligible, 1)
		assert.Empty(t, filtered)
	})

	t.Run("filters multiple choices when unsupported", func(t *testing.T) {
		r, mockQuota, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		multiReq := baseReq
		multiReq.N = intPtr(2)

		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, textReqs, multiReq, nil)
		assert.Len(t, eligible, 2)
		assert.Contains(t, filtered, "provider-b/model-3")
		assert.Equal(t, "multiple_choices_not_supported", filtered["provider-b/model-3"])
	})

	t.Run("filters tool schema dialect mismatch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockQuota := mocks.NewMockQuotaChecker(ctrl)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)

		cfg := types.AppConfig{Providers: []types.ProviderConfig{{
			ID:      "provider-openapi-tools",
			BaseURL: "https://api.example.com/v1",
			Auth:    types.ProviderAuth{Type: "bearer", Env: "GROQ_API_KEY"},
			Models:  types.ProviderModels{Mode: "allowlist", List: []string{"model-1"}},
			Capabilities: types.ProviderCapabilities{
				Streaming:         true,
				Tools:             true,
				StructuredOutputs: "json_schema",
				ToolSchema:        "openapi",
			},
		}}}

		r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)
		mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()
		mockQuota.EXPECT().CheckModelQuota(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		toolsReq := baseReq
		toolsReq.Tools = []types.OpenAITool{{Type: "function"}}
		toolsReq.ToolChoice = "required"

		candidates := r.GenerateCandidates()
		eligible, filtered := r.FilterCandidates(ctx, candidates, types.DerivedRequirements{Output: "text", Streaming: "preferred", Tools: "required"}, toolsReq, nil)
		assert.Empty(t, eligible)
		assert.Equal(t, "tool_schema_dialect_not_supported", filtered["provider-openapi-tools/model-1"])
	})
}

func TestFilterCandidates_FiltersUnavailableProviders(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockQuota := mocks.NewMockQuotaChecker(ctrl)
	mockHealth := mocks.NewMockHealthChecker(ctrl)
	mockProvider := mocks.NewMockProviderCaller(ctrl)
	mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()

	cfg := types.AppConfig{
		Providers: []types.ProviderConfig{{
			ID:      "provider-missing-key",
			BaseURL: "https://api.example.com/v1",
			Auth:    types.ProviderAuth{Type: "bearer", Env: "MISSING_PROVIDER_KEY"},
			Models: types.ProviderModels{
				Mode: "allowlist",
				List: []string{"model-1"},
			},
			Capabilities: types.ProviderCapabilities{Streaming: true, Tools: true, StructuredOutputs: "json_schema_strict"},
		}},
	}

	r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "hi"}}}
	reqs := types.DerivedRequirements{Output: "text", Streaming: "preferred", Tools: "forbidden"}
	candidates := r.GenerateCandidates()

	eligible, filtered := r.FilterCandidates(context.Background(), candidates, reqs, req, nil)
	assert.Empty(t, eligible)
	assert.Equal(t, "provider_unavailable", filtered["provider-missing-key/model-1"])
}

func TestFilterCandidates_FiltersCloudflareWithoutAccountID(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "test-cloudflare-token")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")

	ctrl := gomock.NewController(t)
	mockQuota := mocks.NewMockQuotaChecker(ctrl)
	mockHealth := mocks.NewMockHealthChecker(ctrl)
	mockProvider := mocks.NewMockProviderCaller(ctrl)
	mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100).AnyTimes()

	cfg := types.AppConfig{
		Providers: []types.ProviderConfig{{
			ID:      "cloudflare",
			BaseURL: "https://api.cloudflare.com/client/v4",
			Auth:    types.ProviderAuth{Type: "bearer", Env: "CLOUDFLARE_API_TOKEN"},
			Models: types.ProviderModels{
				Mode: "allowlist",
				List: []string{"@cf/openai/gpt-oss-20b"},
			},
			Capabilities: types.ProviderCapabilities{Streaming: false, Tools: false, StructuredOutputs: "none"},
			ProviderType: "cloudflare_workers_ai",
		}},
	}

	r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "hi"}}}
	reqs := types.DerivedRequirements{Output: "text", Streaming: "preferred", Tools: "forbidden"}
	candidates := r.GenerateCandidates()

	eligible, filtered := r.FilterCandidates(context.Background(), candidates, reqs, req, nil)
	assert.Empty(t, eligible)
	assert.Equal(t, "provider_unavailable", filtered["cloudflare/@cf/openai/gpt-oss-20b"])
}

func TestFilterCandidates_FiltersCloudflareWhenNeuronBudgetExceeded(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "test-cloudflare-token")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "test-account")

	ctrl := gomock.NewController(t)
	quotaStub := &cloudflareQuotaStub{
		estimateTokens:           100,
		estimatedCloudflareUnits: 400,
		cloudflareErr:            errors.NewModelQuotaExceededError("budget exhausted", "cloudflare", "@cf/openai/gpt-oss-20b", "daily_neurons"),
	}
	mockHealth := mocks.NewMockHealthChecker(ctrl)
	mockProvider := mocks.NewMockProviderCaller(ctrl)
	mockHealth.EXPECT().CanExecute(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()

	cfg := types.AppConfig{
		Providers: []types.ProviderConfig{{
			ID:      "cloudflare",
			BaseURL: "https://api.cloudflare.com/client/v4",
			Auth:    types.ProviderAuth{Type: "bearer", Env: "CLOUDFLARE_API_TOKEN"},
			Models: types.ProviderModels{
				Mode:   "allowlist",
				List:   []string{"@cf/openai/gpt-oss-20b"},
				Limits: map[string]types.ModelLimits{"@cf/openai/gpt-oss-20b": {Rpm: intPtr(5)}},
			},
			Capabilities: types.ProviderCapabilities{Streaming: false, Tools: false, StructuredOutputs: "none", MaxTokens: true},
			ProviderType: "cloudflare_workers_ai",
		}},
	}

	r := services.NewRouterWithConfig(cfg, quotaStub, mockHealth, mockProvider)
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "hi"}}, MaxTokens: intPtr(200)}
	reqs := types.DerivedRequirements{Output: "text", Streaming: "preferred", Tools: "forbidden"}
	candidates := r.GenerateCandidates()

	eligible, filtered := r.FilterCandidates(context.Background(), candidates, reqs, req, nil)
	assert.Empty(t, eligible)
	assert.Equal(t, "quota_exceeded_daily_neurons", filtered["cloudflare/@cf/openai/gpt-oss-20b"])
}

func TestFilterCandidates_UsesEffectiveProviderAndModelLimits(t *testing.T) {
	ctx := context.Background()
	providerRPM := 10
	providerConcurrent := 2
	modelTPM := 1000
	modelRPM := 3

	cfg := types.AppConfig{
		Providers: []types.ProviderConfig{
			{
				ID:      "provider-a",
				BaseURL: "https://api.provider-a.com/v1",
				Auth:    types.ProviderAuth{Type: "bearer", Env: "GROQ_API_KEY"},
				Models: types.ProviderModels{
					Mode: "allowlist",
					List: []string{"model-provider-limits", "model-override"},
					Limits: map[string]types.ModelLimits{
						"model-provider-limits": {Tpm: &modelTPM},
						"model-override":        {Rpm: &modelRPM},
					},
				},
				Capabilities: types.ProviderCapabilities{
					Streaming:           true,
					StructuredOutputs:   "json_schema_strict",
					MaxTokens:           true,
					MaxCompletionTokens: true,
				},
				Limits: types.ProviderLimits{Rpm: &providerRPM, MaxConcurrent: &providerConcurrent},
			},
		},
	}

	ctrl := gomock.NewController(t)
	mockQuota := mocks.NewMockQuotaChecker(ctrl)
	mockHealth := mocks.NewMockHealthChecker(ctrl)
	mockProvider := mocks.NewMockProviderCaller(ctrl)
	r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)

	mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100)
	mockHealth.EXPECT().CanExecute(gomock.Any(), "provider-a", "model-provider-limits").Return(true)
	mockQuota.EXPECT().CheckModelQuota(
		gomock.Any(),
		"provider-a",
		"__provider__",
		gomock.AssignableToTypeOf(types.ModelLimits{}),
		100,
	).DoAndReturn(func(ctx context.Context, providerID, model string, limits types.ModelLimits, estimatedTokens int) error {
		require.NotNil(t, limits.Rpm)
		assert.Equal(t, providerRPM, *limits.Rpm)
		return nil
	}).Times(2)
	mockQuota.EXPECT().CheckModelQuota(
		gomock.Any(),
		"provider-a",
		"model-provider-limits",
		gomock.AssignableToTypeOf(types.ModelLimits{}),
		100,
	).DoAndReturn(func(ctx context.Context, providerID, model string, limits types.ModelLimits, estimatedTokens int) error {
		require.NotNil(t, limits.Rpm)
		assert.Equal(t, providerRPM, *limits.Rpm)
		require.NotNil(t, limits.Tpm)
		assert.Equal(t, modelTPM, *limits.Tpm)
		return nil
	})
	mockQuota.EXPECT().CheckConcurrencyLimit(gomock.Any(), "provider-a", "model-provider-limits", providerConcurrent).Return(true)
	mockHealth.EXPECT().CanExecute(gomock.Any(), "provider-a", "model-override").Return(true)
	mockQuota.EXPECT().CheckModelQuota(
		gomock.Any(),
		"provider-a",
		"model-override",
		gomock.AssignableToTypeOf(types.ModelLimits{}),
		100,
	).DoAndReturn(func(ctx context.Context, providerID, model string, limits types.ModelLimits, estimatedTokens int) error {
		require.NotNil(t, limits.Rpm)
		assert.Equal(t, modelRPM, *limits.Rpm)
		return nil
	})
	mockQuota.EXPECT().CheckConcurrencyLimit(gomock.Any(), "provider-a", "model-override", providerConcurrent).Return(true)

	eligible, filtered := r.FilterCandidates(ctx, r.GenerateCandidates(), types.DerivedRequirements{Output: "text", Streaming: "preferred", Tools: "forbidden"}, types.ChatCompletionRequest{}, nil)

	assert.Len(t, eligible, 2)
	assert.Empty(t, filtered)
}

// --- ScoreCandidates ---

func TestScoreCandidates(t *testing.T) {
	ctx := context.Background()

	t.Run("applies preference bonus and health score", func(t *testing.T) {
		r, _, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-a", "model-1").Return(services.HealthMetrics{HealthScore: 1.0})
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-b", "model-3").Return(services.HealthMetrics{HealthScore: 1.0})

		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", ScoreBreakdown: map[string]float64{}},
			{Provider: testConfig().Providers[1], Model: "model-3", ScoreBreakdown: map[string]float64{}},
		}
		hints := &types.RouterHints{Providers: &types.ProviderPreferences{Prefer: []string{"provider-b"}}}

		scored := r.ScoreCandidates(ctx, candidates, types.DerivedRequirements{Output: "text"}, hints)
		assert.Equal(t, "model-3", scored[0].Model)
		assert.Greater(t, scored[0].Score, scored[1].Score)
		assert.Contains(t, scored[0].ScoreBreakdown, "preference_bonus")
	})

	t.Run("sorts by descending score", func(t *testing.T) {
		r, _, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-a", "model-1").Return(services.HealthMetrics{HealthScore: 0.2})
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-b", "model-3").Return(services.HealthMetrics{HealthScore: 1.0})

		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", ScoreBreakdown: map[string]float64{}},
			{Provider: testConfig().Providers[1], Model: "model-3", ScoreBreakdown: map[string]float64{}},
		}

		scored := r.ScoreCandidates(ctx, candidates, types.DerivedRequirements{Output: "text"}, nil)
		assert.Equal(t, "model-3", scored[0].Model)
	})

	t.Run("applies success ratio as extra score component", func(t *testing.T) {
		r, _, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-a", "model-1").Return(services.HealthMetrics{
			HealthScore:  1.0,
			SuccessCount: 0,
			FailureCount: 0, // defaults ratio to 1.0 (1/1 behavior)
		})
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-b", "model-3").Return(services.HealthMetrics{
			HealthScore:  1.0,
			SuccessCount: 1,
			FailureCount: 3, // ratio 0.25
		})

		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", ScoreBreakdown: map[string]float64{}},
			{Provider: testConfig().Providers[1], Model: "model-3", ScoreBreakdown: map[string]float64{}},
		}

		scored := r.ScoreCandidates(ctx, candidates, types.DerivedRequirements{Output: "text"}, nil)
		assert.Equal(t, "model-1", scored[0].Model)
		assert.InDelta(t, 1.0, scored[0].ScoreBreakdown["success_ratio"], 0.0001)
		assert.InDelta(t, 0.25, scored[1].ScoreBreakdown["success_ratio"], 0.0001)
	})

	t.Run("combines success ratio with existing score formula", func(t *testing.T) {
		r, _, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-a", "model-1").Return(services.HealthMetrics{
			HealthScore:  0.6,
			SuccessCount: 3,
			FailureCount: 1, // ratio 0.75
		})

		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", ScoreBreakdown: map[string]float64{}},
		}

		scored := r.ScoreCandidates(ctx, candidates, types.DerivedRequirements{Output: "text"}, nil)
		assert.InDelta(t, 1.55, scored[0].Score, 0.0001) // 1.0*0.5 + 0.6*0.5 + 0.75
	})

	t.Run("penalizes high concurrency utilization", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)
		limit := 10
		cfg := types.AppConfig{Providers: []types.ProviderConfig{
			{
				ID:      "busy-provider",
				BaseURL: "https://busy.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models: types.ProviderModels{
					Mode:   "allowlist",
					List:   []string{"busy-model"},
					Limits: map[string]types.ModelLimits{"busy-model": {MaxConcurrent: &limit}},
				},
			},
			{
				ID:      "idle-provider",
				BaseURL: "https://idle.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models: types.ProviderModels{
					Mode:   "allowlist",
					List:   []string{"idle-model"},
					Limits: map[string]types.ModelLimits{"idle-model": {MaxConcurrent: &limit}},
				},
			},
		}}
		quotaStub := &cloudflareQuotaStub{concurrencyUsage: map[string]int{"busy-provider/busy-model": 9}}
		r := services.NewRouterWithConfig(cfg, quotaStub, mockHealth, mockProvider)

		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "busy-provider", "busy-model").Return(services.HealthMetrics{HealthScore: 1.0})
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "idle-provider", "idle-model").Return(services.HealthMetrics{HealthScore: 1.0})

		candidates := []types.RoutingCandidate{
			{Provider: cfg.Providers[0], Model: "busy-model", ScoreBreakdown: map[string]float64{}},
			{Provider: cfg.Providers[1], Model: "idle-model", ScoreBreakdown: map[string]float64{}},
		}

		scored := r.ScoreCandidates(ctx, candidates, types.DerivedRequirements{Output: "text"}, nil)
		require.Len(t, scored, 2)
		assert.Equal(t, "idle-provider", scored[0].Provider.ID)
		assert.InDelta(t, 0.675, scored[1].ScoreBreakdown["concurrency_load_penalty"], 0.0001)
	})

	t.Run("ranks strict schema support before adapter fallback", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockQuota := mocks.NewMockQuotaChecker(ctrl)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)
		cfg := types.AppConfig{Providers: []types.ProviderConfig{
			{
				ID:      "json-provider",
				BaseURL: "https://json.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"json-model"}},
				Capabilities: types.ProviderCapabilities{
					StructuredOutputs: "json_object",
				},
			},
			{
				ID:      "model-dependent-provider",
				BaseURL: "https://model-dependent.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"model-dependent-model"}},
				Capabilities: types.ProviderCapabilities{
					StructuredOutputs: "model_dependent",
				},
			},
			{
				ID:      "schema-provider",
				BaseURL: "https://schema.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"schema-model"}},
				Capabilities: types.ProviderCapabilities{
					StructuredOutputs: "json_schema",
				},
			},
			{
				ID:      "strict-provider",
				BaseURL: "https://strict.example/v1",
				Auth:    types.ProviderAuth{Type: "bearer"},
				Models:  types.ProviderModels{Mode: "allowlist", List: []string{"strict-model"}},
				Capabilities: types.ProviderCapabilities{
					StructuredOutputs: "json_schema_strict",
				},
			},
		}}
		r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)

		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "json-provider", "json-model").Return(services.HealthMetrics{HealthScore: 1.0})
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "model-dependent-provider", "model-dependent-model").Return(services.HealthMetrics{HealthScore: 1.0})
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "schema-provider", "schema-model").Return(services.HealthMetrics{HealthScore: 1.0})
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "strict-provider", "strict-model").Return(services.HealthMetrics{HealthScore: 1.0})

		candidates := []types.RoutingCandidate{
			{Provider: cfg.Providers[0], Model: "json-model", Score: 2.5, ScoreBreakdown: map[string]float64{}},
			{Provider: cfg.Providers[1], Model: "model-dependent-model", Score: 0.8, ScoreBreakdown: map[string]float64{}},
			{Provider: cfg.Providers[2], Model: "schema-model", Score: 0.6, ScoreBreakdown: map[string]float64{}},
			{Provider: cfg.Providers[3], Model: "strict-model", Score: 0.4, ScoreBreakdown: map[string]float64{}},
		}

		scored := r.ScoreCandidates(ctx, candidates, types.DerivedRequirements{Output: "json_schema_strict"}, nil)

		require.Len(t, scored, 4)
		assert.Equal(t, "strict-provider", scored[0].Provider.ID)
		assert.Equal(t, "schema-provider", scored[1].Provider.ID)
		assert.Equal(t, "model-dependent-provider", scored[2].Provider.ID)
		assert.Equal(t, "json-provider", scored[3].Provider.ID)
		assert.InDelta(t, 3.0, scored[0].ScoreBreakdown["strict_structured_output_bonus"], 0.0001)
		assert.InDelta(t, 2.0, scored[1].ScoreBreakdown["strict_structured_output_bonus"], 0.0001)
		assert.InDelta(t, 1.0, scored[2].ScoreBreakdown["strict_structured_output_bonus"], 0.0001)
		assert.InDelta(t, 2.0, scored[3].ScoreBreakdown["strict_structured_output_penalty"], 0.0001)
	})
}

// --- CompilePlan ---

func TestCompilePlan(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	candidates := []types.RoutingCandidate{
		{Provider: testConfig().Providers[0], Model: "model-1", Score: 1.0},
		{Provider: testConfig().Providers[0], Model: "model-2", Score: 0.8},
		{Provider: testConfig().Providers[1], Model: "model-3", Score: 0.5},
	}

	t.Run("default plan", func(t *testing.T) {
		plan := r.CompilePlan(candidates, nil, nil)
		assert.Len(t, plan.Attempts, 3)
		assert.Equal(t, 3, plan.MaxAttempts)
		assert.Equal(t, 300000, plan.Attempts[0].TimeoutMs)
		assert.True(t, plan.RetryOn429)
		assert.True(t, plan.RetryOnTimeout)
		assert.True(t, plan.RetryOn5xx)
		assert.Nil(t, plan.HardTimeoutMs)
	})

	t.Run("limits by maxAttempts from hints", func(t *testing.T) {
		hints := &types.RouterHints{Fallback: &types.FallbackConfig{MaxAttempts: intPtr(1)}}
		plan := r.CompilePlan(candidates, hints, nil)
		assert.Len(t, plan.Attempts, 1)
	})

	t.Run("uses SLO timeout from hints", func(t *testing.T) {
		hints := &types.RouterHints{SLO: &types.SLOConfig{MaxLatencyMs: intPtr(5000)}}
		plan := r.CompilePlan(candidates, hints, nil)
		assert.Equal(t, 5000, plan.Attempts[0].TimeoutMs)
	})

	t.Run("uses tier SLO timeout without capping attempts", func(t *testing.T) {
		slo := &types.TierSLO{MaxLatencyMs: intPtr(10000), MaxAttempts: intPtr(2)}
		plan := r.CompilePlan(candidates, nil, slo)
		assert.Equal(t, 10000, plan.Attempts[0].TimeoutMs)
		assert.Len(t, plan.Attempts, 3)
		assert.Equal(t, 3, plan.MaxAttempts)
	})

	t.Run("per-model timeout overrides tier SLO", func(t *testing.T) {
		provider := testConfig().Providers[0]
		provider.Models.Limits["model-1"] = types.ModelLimits{Rpm: intPtr(30), TimeoutMs: intPtr(60000)}
		overrideCandidates := []types.RoutingCandidate{
			{Provider: provider, Model: "model-1", Score: 1.0},
			{Provider: testConfig().Providers[1], Model: "model-3", Score: 0.9},
		}
		slo := &types.TierSLO{MaxLatencyMs: intPtr(30000)}
		plan := r.CompilePlan(overrideCandidates, nil, slo)
		require.Len(t, plan.Attempts, 2)
		assert.Equal(t, 60000, plan.Attempts[0].TimeoutMs, "model override must beat the tier SLO")
		assert.Equal(t, 30000, plan.Attempts[1].TimeoutMs, "models without an override keep the tier SLO")
	})

	t.Run("custom retry policy", func(t *testing.T) {
		hints := &types.RouterHints{
			Fallback: &types.FallbackConfig{
				On429:     boolPtr(false),
				OnTimeout: boolPtr(false),
				On5xx:     boolPtr(false),
			},
		}
		plan := r.CompilePlan(candidates, hints, nil)
		assert.False(t, plan.RetryOn429)
		assert.False(t, plan.RetryOnTimeout)
		assert.False(t, plan.RetryOn5xx)
	})

	t.Run("sets hard timeout", func(t *testing.T) {
		hints := &types.RouterHints{SLO: &types.SLOConfig{HardTimeoutMs: intPtr(60000)}}
		plan := r.CompilePlan(candidates, hints, nil)
		require.NotNil(t, plan.HardTimeoutMs)
		assert.Equal(t, 60000, *plan.HardTimeoutMs)
	})

	t.Run("stores resolved model capabilities", func(t *testing.T) {
		cfg := testConfig()
		jsonObject := "json_object"
		cfg.Providers[0].Models.Capabilities = map[string]types.ModelCapabilities{
			"model-1": {StructuredOutputs: &jsonObject},
		}
		r := services.NewRouterWithConfig(cfg, nil, nil, nil)
		plan := r.CompilePlan([]types.RoutingCandidate{{Provider: cfg.Providers[0], Model: "model-1"}}, nil, nil)

		require.Len(t, plan.Attempts, 1)
		assert.Equal(t, "json_object", plan.Attempts[0].Capabilities.StructuredOutputs)
	})
}

// --- ShouldRetry ---

func TestShouldRetry(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	twoAttempts := types.RoutingPlan{
		Attempts:       make([]types.RoutingAttempt, 2),
		RetryOn429:     true,
		RetryOnTimeout: true,
		RetryOn5xx:     true,
	}

	tests := []struct {
		name string
		err  error
		plan types.RoutingPlan
		idx  int
		want bool
	}{
		{"RateLimitError retries", errors.NewRateLimitError("limited", 60, "rpm"), twoAttempts, 0, true},
		{"RateLimitError no retry", errors.NewRateLimitError("limited", 60, "rpm"), types.RoutingPlan{Attempts: make([]types.RoutingAttempt, 2), RetryOn429: false}, 0, false},
		{"TimeoutError retries", errors.NewTimeoutError("timeout", "request"), twoAttempts, 0, true},
		{"TimeoutError no retry", errors.NewTimeoutError("timeout", "request"), types.RoutingPlan{Attempts: make([]types.RoutingAttempt, 2), RetryOnTimeout: false}, 0, false},
		{"retryable ProviderError", &errors.ProviderError{Message: "err", StatusCode: 500, IsRetryable: true}, twoAttempts, 0, true},
		{"non-retryable ProviderError", &errors.ProviderError{Message: "err", StatusCode: 400, IsRetryable: false}, twoAttempts, 0, false},
		{"PaymentRequired never retries", errors.NewPaymentRequiredError("pay"), twoAttempts, 0, false},
		{"ValidationError never retries", errors.NewValidationError("bad", nil), twoAttempts, 0, false},
		{"last attempt never retries", errors.NewRateLimitError("limited", 60, "rpm"), twoAttempts, 1, false},
		{"CircuitBreakerError retries", errors.NewCircuitBreakerError("open", "p", "OPEN"), twoAttempts, 0, true},
		{"ModelQuotaExceeded retries", errors.NewModelQuotaExceededError("exceeded", "p", "m", "rpm"), twoAttempts, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, r.ShouldRetry(tt.err, tt.plan, tt.idx))
		})
	}
}

// --- Execute ---

func TestExecute(t *testing.T) {
	ctx := context.Background()
	baseReq := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "hello"}}}

	t.Run("first attempt succeeds", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)

		content := "response text"
		resp := &types.ChatCompletionResponse{
			ID: "test-id", Model: "model-1",
			Usage:   &types.Usage{TotalTokens: 50},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}

		mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil)
		mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-a", "model-1", gomock.Any())
		mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-a", "model-1", 50).Return(nil)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{{
				ProviderID: "provider-a", Model: "model-1",
				BaseURL: "https://a.com/v1", APIKey: "key",
				TimeoutMs: 30000, Auth: types.ProviderAuth{Type: "bearer"},
			}},
			MaxAttempts: 1,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-1")
		require.NoError(t, err)
		assert.Equal(t, "test-id", result.Response.ID)
		assert.Equal(t, 1, result.Attempts)
		assert.Equal(t, "provider-a", result.ProviderID)
	})

	t.Run("passes attempt capabilities to provider request", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)

		content := "response text"
		resp := &types.ChatCompletionResponse{
			ID: "test-id", Model: "model-1",
			Usage:   &types.Usage{TotalTokens: 50},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}

		mockProvider.EXPECT().CallProvider(
			gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).DoAndReturn(func(_ string, _ string, _ string, gotReq types.ChatCompletionRequest, _ int, _ context.Context, _ string, _ types.ProviderAuth, _ string) (*types.ChatCompletionResponse, error) {
			assert.Equal(t, "json_object", gotReq.ProviderCapabilities.StructuredOutputs)
			return resp, nil
		})
		mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-a", "model-1", gomock.Any())
		mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-a", "model-1", 50).Return(nil)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{{
				ProviderID: "provider-a", Model: "model-1",
				BaseURL: "https://a.com/v1", APIKey: "key",
				TimeoutMs:    30000,
				Auth:         types.ProviderAuth{Type: "bearer"},
				Capabilities: types.ProviderCapabilities{StructuredOutputs: "json_object"},
			}},
			MaxAttempts: 1,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-capabilities")
		require.NoError(t, err)
		assert.Equal(t, "test-id", result.Response.ID)
	})

	t.Run("valid adapter fallback output succeeds after strict validation", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)

		schema := []byte(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`)
		strict := true
		req := baseReq
		req.ResponseFormat = &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:   "person",
				Schema: schema,
				Strict: &strict,
			},
		}

		content := `{"name":"Ada"}`
		resp := &types.ChatCompletionResponse{
			ID: "adapter-valid", Model: "model-1",
			Usage:   &types.Usage{TotalTokens: 35},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}

		mockProvider.EXPECT().CallProvider(
			gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).DoAndReturn(func(_ string, _ string, _ string, gotReq types.ChatCompletionRequest, _ int, _ context.Context, _ string, _ types.ProviderAuth, _ string) (*types.ChatCompletionResponse, error) {
			assert.Equal(t, "json_object", gotReq.ProviderCapabilities.StructuredOutputs)
			assert.Equal(t, "json_schema", gotReq.ResponseFormat.Type)
			return resp, nil
		})
		mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-a", "model-1", gomock.Any())
		mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-a", "model-1", 35).Return(nil)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{{
				ProviderID: "provider-a", Model: "model-1",
				BaseURL: "https://a.com/v1", APIKey: "key",
				TimeoutMs:    30000,
				Auth:         types.ProviderAuth{Type: "bearer"},
				Capabilities: types.ProviderCapabilities{StructuredOutputs: "json_object"},
			}},
			MaxAttempts: 1,
		}

		result, err := r.Execute(ctx, plan, req, "req-adapter-valid")
		require.NoError(t, err)
		assert.Equal(t, "adapter-valid", result.Response.ID)
		assert.Equal(t, "provider-a", result.ProviderID)
	})

	t.Run("records provider level quota usage", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockQuota := mocks.NewMockQuotaChecker(ctrl)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)
		cfg := testConfig()
		cfg.Providers[0].Limits = types.ProviderLimits{Rpm: intPtr(10)}
		r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)

		content := "response text"
		resp := &types.ChatCompletionResponse{
			ID: "test-provider-quota", Model: "model-1",
			Usage:   &types.Usage{TotalTokens: 50},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}

		mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil)
		mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-a", "model-1", gomock.Any())
		mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-a", "model-1", 50).Return(nil)
		mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-a", "__provider__", 50).Return(nil)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{{
				ProviderID: "provider-a", Model: "model-1",
				BaseURL: "https://a.com/v1", APIKey: "key",
				TimeoutMs: 30000, Auth: types.ProviderAuth{Type: "bearer"},
			}},
			MaxAttempts: 1,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-provider-quota")
		require.NoError(t, err)
		assert.Equal(t, "provider-a", result.ProviderID)
	})

	t.Run("first fails second succeeds", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)

		providerErr := &errors.ProviderError{Message: "server error", StatusCode: 500, IsRetryable: true}
		content := "ok"
		resp := &types.ChatCompletionResponse{
			ID: "id-2", Model: "model-3",
			Usage:   &types.Usage{TotalTokens: 30},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}

		gomock.InOrder(
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, providerErr),
			mockHealth.EXPECT().RecordFailure(gomock.Any(), "provider-a", "model-1"),
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-3", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil),
			mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-b", "model-3", gomock.Any()),
			mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-b", "model-3", 30).Return(nil),
		)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 2, RetryOn5xx: true,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-2")
		require.NoError(t, err)
		assert.Equal(t, 2, result.Attempts)
		assert.Equal(t, "provider-b", result.ProviderID)
	})

	t.Run("structured output validation failure fails over", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)
		redisClient, _ := newExternalTestRedis(t)
		cooldowns := services.NewCooldownService(redisClient, "cooldown", testCooldownConfig())
		r.SetCooldownService(cooldowns)

		schema := []byte(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`)
		strict := true
		req := baseReq
		req.ResponseFormat = &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:   "person",
				Schema: schema,
				Strict: &strict,
			},
		}

		badContent := `{"name":123}`
		goodContent := `{"name":"Ada"}`
		badResp := &types.ChatCompletionResponse{
			ID:      "bad-structured",
			Model:   "model-1",
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &badContent}}},
		}
		goodResp := &types.ChatCompletionResponse{
			ID:      "good-structured",
			Model:   "model-3",
			Usage:   &types.Usage{TotalTokens: 42},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &goodContent}}},
		}

		gomock.InOrder(
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(badResp, nil),
			mockHealth.EXPECT().RecordFailure(gomock.Any(), "provider-a", "model-1"),
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-3", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(goodResp, nil),
			mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-b", "model-3", gomock.Any()),
			mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-b", "model-3", 42).Return(nil),
		)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 2,
		}

		result, err := r.Execute(ctx, plan, req, "req-structured-failover")
		require.NoError(t, err)
		assert.Equal(t, "provider-b", result.ProviderID)
		assert.Equal(t, "good-structured", result.Response.ID)
		assert.True(t, cooldowns.IsOnCooldown(ctx, "provider-a", "model-1"))
		assert.Equal(t, services.CooldownStructuredOutput, cooldowns.GetCooldownReason(ctx, "provider-a", "model-1"))
		assert.GreaterOrEqual(t, cooldowns.GetCooldownRemaining(ctx, "provider-a", "model-1"), 20*time.Second)
		assert.LessOrEqual(t, cooldowns.GetCooldownRemaining(ctx, "provider-a", "model-1"), 30*time.Second)
	})

	t.Run("provider structured output 400 fails over with structured cooldown", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)
		redisClient, _ := newExternalTestRedis(t)
		cooldowns := services.NewCooldownService(redisClient, "cooldown", testCooldownConfig())
		r.SetCooldownService(cooldowns)

		req := baseReq
		req.ResponseFormat = &types.ResponseFormat{Type: "json_object"}
		providerErr := &errors.ProviderError{
			Message:    "HTTP error 400: Unsupported JSON Schema feature for Gemini: enum",
			StatusCode: http.StatusBadRequest,
		}
		goodContent := `{"ok":true}`
		goodResp := &types.ChatCompletionResponse{
			ID:      "good-after-provider-400",
			Model:   "model-3",
			Usage:   &types.Usage{TotalTokens: 12},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &goodContent}}},
		}

		gomock.InOrder(
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, providerErr),
			mockHealth.EXPECT().RecordFailure(gomock.Any(), "provider-a", "model-1"),
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-3", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(goodResp, nil),
			mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-b", "model-3", gomock.Any()),
			mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-b", "model-3", 12).Return(nil),
		)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 2,
		}

		result, err := r.Execute(ctx, plan, req, "req-provider-structured-400")
		require.NoError(t, err)
		assert.Equal(t, "provider-b", result.ProviderID)
		assert.True(t, cooldowns.IsOnCooldown(ctx, "provider-a", "model-1"))
		assert.Equal(t, services.CooldownStructuredOutput, cooldowns.GetCooldownReason(ctx, "provider-a", "model-1"))
	})

	t.Run("rate limited attempt applies cooldown and failover succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockQuota := mocks.NewMockQuotaChecker(ctrl)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)
		cfg := testConfig()
		cfg.Providers[0].Limits = types.ProviderLimits{Rpm: intPtr(10)}
		r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)
		redisClient, _ := newExternalTestRedis(t)
		cooldowns := services.NewCooldownService(redisClient, "cooldown", testCooldownConfig())
		r.SetCooldownService(cooldowns)

		rateErr := errors.NewRateLimitError("limited", 60, "rpm")
		rateErr.RetryAfterProvided = true
		content := "ok"
		resp := &types.ChatCompletionResponse{
			ID: "id-rate-limit", Model: "model-3",
			Usage:   &types.Usage{TotalTokens: 24},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}

		mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, rateErr)
		mockQuota.EXPECT().HandleProviderRateLimit(gomock.Any(), "provider-a", "model-1", gomock.Any()).Return(services.RateLimitInfo{IsRateLimited: true, RetryAfter: 60, LimitType: "rpm"})
		mockQuota.EXPECT().HandleProviderRateLimit(gomock.Any(), "provider-a", "__provider__", gomock.Any()).Return(services.RateLimitInfo{IsRateLimited: true, RetryAfter: 60, LimitType: "rpm"})
		mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-3", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil)
		mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-b", "model-3", gomock.Any())
		mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-b", "model-3", 24).Return(nil)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 2,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-rate-limit")
		require.NoError(t, err)
		assert.Equal(t, "provider-b", result.ProviderID)
		assert.True(t, cooldowns.IsOnCooldown(ctx, "provider-a", "model-1"))
		assert.True(t, cooldowns.IsOnCooldown(ctx, "provider-a", "model-2"))
		assert.Equal(t, services.CooldownRateLimit, cooldowns.GetCooldownReason(ctx, "provider-a", "model-1"))
		assert.GreaterOrEqual(t, cooldowns.GetCooldownRemaining(ctx, "provider-a", "model-1"), 55*time.Second)

		mockQuota.EXPECT().EstimateTokens(gomock.Any()).Return(100)
		mockHealth.EXPECT().CanExecute(gomock.Any(), "provider-a", "model-1").Return(true)
		eligible, filtered := r.FilterCandidates(
			ctx,
			[]types.RoutingCandidate{{Provider: cfg.Providers[0], Model: "model-1", ScoreBreakdown: map[string]float64{}}},
			types.DerivedRequirements{Output: "text", Streaming: "preferred", Tools: "forbidden"},
			baseReq,
			nil,
		)
		assert.Empty(t, eligible)
		assert.Contains(t, filtered["provider-a/model-1"], "provider_cooldown_active:rate_limit")
	})

	t.Run("retry policy disables failover on rate limit", func(t *testing.T) {
		r, mockQuota, _, mockProvider := newTestRouter(t)

		rateErr := errors.NewRateLimitError("limited", 60, "rpm")
		mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, rateErr)
		mockQuota.EXPECT().HandleProviderRateLimit(gomock.Any(), "provider-a", "model-1", gomock.Any()).Return(services.RateLimitInfo{IsRateLimited: true, RetryAfter: 60, LimitType: "rpm"})

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts:    2,
			RetryOn429:     false,
			RetryOnTimeout: true,
			RetryOn5xx:     true,
			RetryPolicySet: true,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-no-429-failover")
		assert.Nil(t, result)
		require.Error(t, err)
		gatewayErr, ok := err.(*types.GatewayError)
		require.True(t, ok)
		assert.Equal(t, "RATE_LIMITED", gatewayErr.Code)
	})

	t.Run("hard timeout bounds provider attempt timeout", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)
		hardTimeoutMs := 50
		content := "ok"
		resp := &types.ChatCompletionResponse{
			ID:      "id-hard-timeout",
			Model:   "model-1",
			Usage:   &types.Usage{TotalTokens: 12},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}

		mockProvider.EXPECT().CallProvider(
			gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).DoAndReturn(func(baseURL, apiKey, model string, request types.ChatCompletionRequest, timeoutMs int, callCtx context.Context, providerType string, auth types.ProviderAuth, requestID string) (*types.ChatCompletionResponse, error) {
			assert.Positive(t, timeoutMs)
			assert.LessOrEqual(t, timeoutMs, hardTimeoutMs)
			_, hasDeadline := callCtx.Deadline()
			assert.True(t, hasDeadline)
			return resp, nil
		})
		mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-a", "model-1", gomock.Any())
		mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-a", "model-1", 12).Return(nil)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts:   1,
			HardTimeoutMs: &hardTimeoutMs,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-hard-timeout-bound")
		require.NoError(t, err)
		assert.Equal(t, "provider-a", result.ProviderID)
	})

	t.Run("provider 4xx fails over", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)

		providerErr := &errors.ProviderError{Message: "model unavailable", StatusCode: http.StatusNotFound, IsRetryable: false}
		content := "ok"
		resp := &types.ChatCompletionResponse{
			ID: "id-4xx", Model: "model-3",
			Usage:   &types.Usage{TotalTokens: 20},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}

		gomock.InOrder(
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, providerErr),
			mockHealth.EXPECT().RecordFailure(gomock.Any(), "provider-a", "model-1"),
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-3", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil),
			mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-b", "model-3", gomock.Any()),
			mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-b", "model-3", 20).Return(nil),
		)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 2,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-4xx")
		require.NoError(t, err)
		assert.Equal(t, 2, result.Attempts)
		assert.Equal(t, "provider-b", result.ProviderID)
	})

	t.Run("provider auth failure applies provider cooldown", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)
		redisClient, _ := newExternalTestRedis(t)
		cooldowns := services.NewCooldownService(redisClient, "cooldown", testCooldownConfig())
		r.SetCooldownService(cooldowns)

		providerErr := &errors.ProviderError{Message: "HTTP error 401: Unauthorized", StatusCode: http.StatusUnauthorized, IsRetryable: false}
		content := "ok"
		resp := &types.ChatCompletionResponse{
			ID: "id-auth-failover", Model: "model-3",
			Usage:   &types.Usage{TotalTokens: 20},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}

		mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, providerErr)
		mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-3", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil)
		mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-b", "model-3", gomock.Any())
		mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-b", "model-3", 20).Return(nil)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 2,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-auth-4xx")
		require.NoError(t, err)
		assert.Equal(t, "provider-b", result.ProviderID)
		assert.True(t, cooldowns.IsOnCooldown(ctx, "provider-a", "model-1"))
		assert.True(t, cooldowns.IsOnCooldown(ctx, "provider-a", "model-2"))
		assert.False(t, cooldowns.IsOnCooldown(ctx, "provider-b", "model-3"))
		assert.Equal(t, services.CooldownAuth, cooldowns.GetCooldownReason(ctx, "provider-a", "model-1"))
	})

	t.Run("all attempts fail", func(t *testing.T) {
		r, _, mockHealth, mockProvider := newTestRouter(t)

		providerErr := &errors.ProviderError{Message: "error", StatusCode: 500, IsRetryable: true}
		mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, providerErr).Times(2)
		mockHealth.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), gomock.Any()).Times(2)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 2, RetryOn5xx: true,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-3")
		assert.Nil(t, result)
		require.Error(t, err)
		gatewayErr, ok := err.(*types.GatewayError)
		require.True(t, ok)
		assert.Equal(t, "ALL_ATTEMPTS_FAILED", gatewayErr.Code)
	})

	t.Run("payment required fails over instead of aborting", func(t *testing.T) {
		r, mockQuota, mockHealth, mockProvider := newTestRouter(t)

		payErr := errors.NewPaymentRequiredError("payment required")
		content := "ok"
		resp := &types.ChatCompletionResponse{
			ID: "id-payover", Model: "model-3",
			Usage:   &types.Usage{TotalTokens: 12},
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: &content}}},
		}
		gomock.InOrder(
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, payErr),
			mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), "model-3", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil),
		)
		mockHealth.EXPECT().RecordFailure(gomock.Any(), "provider-a", "model-1")
		mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-b", "model-3", gomock.Any())
		mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-b", "model-3", 12).Return(nil)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 2, RetryOn5xx: true,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-4")
		require.NoError(t, err)
		assert.Equal(t, "provider-b", result.ProviderID)
	})

	t.Run("payment required on every attempt surfaces PAYMENT_REQUIRED", func(t *testing.T) {
		r, _, mockHealth, mockProvider := newTestRouter(t)

		payErr := errors.NewPaymentRequiredError("payment required")
		mockProvider.EXPECT().CallProvider(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, payErr).Times(2)
		mockHealth.EXPECT().RecordFailure(gomock.Any(), gomock.Any(), gomock.Any()).Times(2)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
				{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 2, RetryOn5xx: true,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-4b")
		assert.Nil(t, result)
		require.Error(t, err)
		gatewayErr, ok := err.(*types.GatewayError)
		require.True(t, ok)
		assert.Equal(t, "PAYMENT_REQUIRED", gatewayErr.Code)
	})

	t.Run("concurrency denied is included in attempt details", func(t *testing.T) {
		cfg := testConfig()
		maxConcurrent := 1
		cfg.Providers[0].Models.Limits["model-1"] = types.ModelLimits{MaxConcurrent: &maxConcurrent}

		ctrl := gomock.NewController(t)
		mockQuota := mocks.NewMockQuotaChecker(ctrl)
		mockHealth := mocks.NewMockHealthChecker(ctrl)
		mockProvider := mocks.NewMockProviderCaller(ctrl)
		r := services.NewRouterWithConfig(cfg, mockQuota, mockHealth, mockProvider)

		mockQuota.EXPECT().AcquireConcurrencySlot(gomock.Any(), "provider-a", "model-1", 1).Return(assert.AnError)

		plan := types.RoutingPlan{
			Attempts: []types.RoutingAttempt{
				{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			},
			MaxAttempts: 1,
		}

		result, err := r.Execute(ctx, plan, baseReq, "req-concurrency")
		assert.Nil(t, result)
		require.Error(t, err)
		gatewayErr, ok := err.(*types.GatewayError)
		require.True(t, ok)
		assert.Equal(t, "ALL_ATTEMPTS_FAILED", gatewayErr.Code)

		attempts, ok := gatewayErr.Details["attempts"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, attempts, 1)
		assert.Equal(t, "concurrency_denied", attempts[0]["failure_kind"])
		assert.Equal(t, string(types.ActionFailover), attempts[0]["failure_action"])
	})
}

func TestExecuteStream_RateLimitedAttemptAppliesCooldown(t *testing.T) {
	ctx := context.Background()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "hello"}}}
	r, mockQuota, mockHealth, mockProvider := newTestRouter(t)
	redisClient, _ := newExternalTestRedis(t)
	cooldowns := services.NewCooldownService(redisClient, "cooldown", testCooldownConfig())
	r.SetCooldownService(cooldowns)

	rateLimited := &types.GatewayError{
		Type:    "gateway_error",
		Code:    "RATE_LIMITED",
		Message: "limited",
		Details: map[string]any{"retry_after": 60, "limit_type": "rpm"},
	}

	mockProvider.EXPECT().StreamProviderChannel(gomock.Any(), gomock.Any(), "model-1", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(streamResult(rateLimited))
	mockQuota.EXPECT().HandleProviderRateLimit(gomock.Any(), "provider-a", "model-1", gomock.Any()).Return(services.RateLimitInfo{IsRateLimited: true, RetryAfter: 60, LimitType: "rpm"})
	mockProvider.EXPECT().StreamProviderChannel(gomock.Any(), gomock.Any(), "model-3", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(streamResult(nil))
	mockHealth.EXPECT().RecordSuccess(gomock.Any(), "provider-b", "model-3", gomock.Any())
	mockQuota.EXPECT().RecordModelUsage(gomock.Any(), "provider-b", "model-3", 100).Return(nil)

	plan := types.RoutingPlan{
		Attempts: []types.RoutingAttempt{
			{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.com/v1", APIKey: "k1", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
			{ProviderID: "provider-b", Model: "model-3", BaseURL: "https://b.com/v1", APIKey: "k2", TimeoutMs: 5000, Auth: types.ProviderAuth{Type: "bearer"}},
		},
		MaxAttempts: 2,
	}

	result := r.ExecuteStream(ctx, plan, req, "req-stream-rate-limit")
	for range result.Chunks {
	}
	streamErr := <-result.Err
	require.Nil(t, streamErr)
	assert.True(t, cooldowns.IsOnCooldown(ctx, "provider-a", "model-1"))
	assert.Equal(t, services.CooldownRateLimit, cooldowns.GetCooldownReason(ctx, "provider-a", "model-1"))
}

// --- CreateGatewayError ---

func TestCreateGatewayError(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"rate limit", errors.NewRateLimitError("limited", 60, "rpm"), "RATE_LIMITED"},
		{"circuit breaker", errors.NewCircuitBreakerError("open", "p", "OPEN"), "CIRCUIT_BREAKER_OPEN"},
		{"timeout", errors.NewTimeoutError("timeout", "request"), "TIMEOUT"},
		{"quota exceeded", errors.NewModelQuotaExceededError("exceeded", "p", "m", "rpm"), "QUOTA_EXCEEDED"},
		{"payment required", errors.NewPaymentRequiredError("pay"), "PAYMENT_REQUIRED"},
		{"unknown", &errors.ProviderError{Message: "unknown"}, "PROVIDER_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ge := r.CreateGatewayError(tt.err, 1, "req-1")
			assert.Equal(t, tt.wantCode, ge.Code)
		})
	}
}
