package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newProviderService() *ProviderService {
	return NewProviderService()
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt(v int) *int             { return &v }

func TestNewProviderService_DefaultTimeout(t *testing.T) {
	svc := newProviderService()

	assert.Equal(t, defaultRequestTimeout, svc.httpClient.Timeout)
}

func TestNormalizeStructuredOutputForProvider_KeepsStrictJSONSchemaForStrictCapability(t *testing.T) {
	strict := true
	req := types.ChatCompletionRequest{
		ResponseFormat: &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:   "response",
				Schema: json.RawMessage(`{"type":"object"}`),
				Strict: &strict,
			},
		},
		ProviderCapabilities: types.ProviderCapabilities{StructuredOutputs: "json_schema_strict"},
	}

	got := normalizeStructuredOutputForProvider(req, "groq", "openai/gpt-oss-20b")

	require.NotNil(t, got.ResponseFormat)
	require.NotNil(t, got.ResponseFormat.JSONSchema)
	require.NotNil(t, got.ResponseFormat.JSONSchema.Strict)
	assert.True(t, *got.ResponseFormat.JSONSchema.Strict)
	require.NotNil(t, req.ResponseFormat.JSONSchema.Strict, "normalization must not mutate the client contract")
	assert.True(t, *req.ResponseFormat.JSONSchema.Strict)
}

func TestNormalizeStructuredOutputForProvider_StripsStrictJSONSchemaForNonStrictSchemaCapability(t *testing.T) {
	strict := true
	req := types.ChatCompletionRequest{
		ResponseFormat: &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:   "response",
				Schema: json.RawMessage(`{"type":"object"}`),
				Strict: &strict,
			},
		},
		ProviderCapabilities: types.ProviderCapabilities{StructuredOutputs: "json_schema"},
	}

	got := normalizeStructuredOutputForProvider(req, "some-provider", "schema-model")

	require.NotNil(t, got.ResponseFormat)
	require.NotNil(t, got.ResponseFormat.JSONSchema)
	assert.Nil(t, got.ResponseFormat.JSONSchema.Strict)
	require.NotNil(t, req.ResponseFormat.JSONSchema.Strict, "normalization must not mutate the client contract")
	assert.True(t, *req.ResponseFormat.JSONSchema.Strict)
}

func TestNormalizeStructuredOutputForProvider_NormalizesStrictDialectSchema(t *testing.T) {
	strict := true
	originalSchema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"experience":{
				"type":"object",
				"properties":{
					"min":{"type":"number"},
					"max":{"type":"number"}
				}
			},
			"name":{"type":"string"}
		},
		"required":["name"]
	}`)
	req := types.ChatCompletionRequest{
		ResponseFormat: &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:   "response",
				Schema: originalSchema,
				Strict: &strict,
			},
		},
		ProviderCapabilities: types.ProviderCapabilities{StructuredOutputs: "json_schema_strict"},
	}

	got := normalizeStructuredOutputForProvider(req, "groq", "openai/gpt-oss-20b")

	require.NotNil(t, got.ResponseFormat)
	require.NotNil(t, got.ResponseFormat.JSONSchema)
	require.NotNil(t, got.ResponseFormat.JSONSchema.Strict)
	assert.True(t, *got.ResponseFormat.JSONSchema.Strict)
	var normalized map[string]any
	require.NoError(t, json.Unmarshal(got.ResponseFormat.JSONSchema.Schema, &normalized))
	assert.Equal(t, false, normalized["additionalProperties"])
	assert.ElementsMatch(t, []any{"experience", "name"}, normalized["required"])

	properties := normalized["properties"].(map[string]any)
	experience := properties["experience"].(map[string]any)
	assert.Equal(t, false, experience["additionalProperties"])
	assert.ElementsMatch(t, []any{"max", "min"}, experience["required"])

	assert.JSONEq(t, string(originalSchema), string(req.ResponseFormat.JSONSchema.Schema), "normalization must not mutate the client contract")
	require.NotNil(t, req.ResponseFormat.JSONSchema.Strict)
	assert.True(t, *req.ResponseFormat.JSONSchema.Strict)
}

func TestNormalizeStructuredOutputForProvider_DowngradesDirectGeminiSchemaToJSONObject(t *testing.T) {
	strict := true
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Return status"}},
		ResponseFormat: &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:        "status_response",
				Description: "status payload",
				Schema:      json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"additionalProperties":false}`),
				Strict:      &strict,
			},
		},
		ProviderCapabilities: types.ProviderCapabilities{StructuredOutputs: "json_object"},
	}

	got := normalizeStructuredOutputForProvider(req, "gemini", "gemini-2.5-flash")

	require.NotNil(t, got.ResponseFormat)
	assert.Equal(t, "json_object", got.ResponseFormat.Type)
	assert.Nil(t, got.ResponseFormat.JSONSchema)
	require.Len(t, got.Messages, 2)
	instruction, ok := got.Messages[0].Content.(string)
	require.True(t, ok)
	assert.Contains(t, instruction, "JSON Schema")
	assert.Contains(t, instruction, "status_response")
	assert.Contains(t, instruction, `"ok"`)
	assert.Equal(t, "user", got.Messages[1].Role)

	assert.Equal(t, "json_schema", req.ResponseFormat.Type, "normalization must not mutate the client contract")
	require.NotNil(t, req.ResponseFormat.JSONSchema.Strict)
	assert.True(t, *req.ResponseFormat.JSONSchema.Strict)
	assert.Len(t, req.Messages, 1)
}

func TestNormalizeStructuredOutputForProvider_KeepsSchemaForStrictOCIGemini(t *testing.T) {
	strict := true
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Return status"}},
		ResponseFormat: &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:        "status_response",
				Description: "status payload",
				Schema:      json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"additionalProperties":false}`),
				Strict:      &strict,
			},
		},
		ProviderCapabilities: types.ProviderCapabilities{StructuredOutputs: "json_schema_strict"},
	}

	got := normalizeStructuredOutputForProvider(req, "oci", "google.gemini-2.5-flash")

	require.NotNil(t, got.ResponseFormat)
	assert.Equal(t, "json_schema", got.ResponseFormat.Type)
	require.NotNil(t, got.ResponseFormat.JSONSchema)
	require.NotNil(t, got.ResponseFormat.JSONSchema.Strict)
	assert.True(t, *got.ResponseFormat.JSONSchema.Strict)
	assert.Len(t, got.Messages, 1)

	assert.Equal(t, "json_schema", req.ResponseFormat.Type, "normalization must not mutate the client contract")
	require.NotNil(t, req.ResponseFormat.JSONSchema.Strict)
	assert.True(t, *req.ResponseFormat.JSONSchema.Strict)
}

func TestNormalizeStructuredOutputForProvider_DowngradesSchemaForJSONObjectCapability(t *testing.T) {
	strict := true
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Return status"}},
		ResponseFormat: &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:        "status_response",
				Description: "status payload",
				Schema:      json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`),
				Strict:      &strict,
			},
		},
		ProviderCapabilities: types.ProviderCapabilities{StructuredOutputs: "json_object"},
	}

	got := normalizeStructuredOutputForProvider(req, "some-provider", "json-only-model")

	require.NotNil(t, got.ResponseFormat)
	assert.Equal(t, "json_object", got.ResponseFormat.Type)
	assert.Nil(t, got.ResponseFormat.JSONSchema)
	require.Len(t, got.Messages, 2)
	instruction, ok := got.Messages[0].Content.(string)
	require.True(t, ok)
	assert.Contains(t, instruction, "JSON Schema")
	assert.Contains(t, instruction, "status_response")
	assert.Contains(t, instruction, `"ok"`)
	assert.Equal(t, "user", got.Messages[1].Role)

	assert.Equal(t, "json_schema", req.ResponseFormat.Type, "normalization must not mutate the client contract")
	require.NotNil(t, req.ResponseFormat.JSONSchema.Strict)
	assert.True(t, *req.ResponseFormat.JSONSchema.Strict)
	assert.Len(t, req.Messages, 1)
}

func TestNormalizeStructuredOutputForProvider_KeepsSchemaForSchemaCapability(t *testing.T) {
	strict := true
	req := types.ChatCompletionRequest{
		ResponseFormat: &types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: &types.JSONSchema{
				Name:   "response",
				Schema: json.RawMessage(`{"type":"object"}`),
				Strict: &strict,
			},
		},
		ProviderCapabilities: types.ProviderCapabilities{StructuredOutputs: "json_schema"},
	}

	got := normalizeStructuredOutputForProvider(req, "some-provider", "schema-model")

	require.NotNil(t, got.ResponseFormat)
	assert.Equal(t, "json_schema", got.ResponseFormat.Type)
	require.NotNil(t, got.ResponseFormat.JSONSchema)
	assert.Nil(t, got.ResponseFormat.JSONSchema.Strict)
}

func TestProviderServiceShouldLogRawProviderResponse_DisabledByDefault(t *testing.T) {
	t.Setenv("LOG_RAW_PROVIDER_RESPONSES", "")
	t.Setenv("LOG_RAW_PROVIDER_RESPONSE_FILTERS", "mistral/magistral-*")

	svc := newProviderService()

	assert.False(t, svc.shouldLogRawProviderResponse("mistral", "magistral-medium-2509"))
}

func TestProviderServiceShouldLogRawProviderResponse_EnabledWithoutFilters(t *testing.T) {
	t.Setenv("LOG_RAW_PROVIDER_RESPONSES", "1")
	t.Setenv("LOG_RAW_PROVIDER_RESPONSE_FILTERS", "")

	svc := newProviderService()

	assert.True(t, svc.shouldLogRawProviderResponse("mistral", "mistral-large-2411"))
}

func TestProviderServiceShouldLogRawProviderResponse_FilterMatching(t *testing.T) {
	t.Setenv("LOG_RAW_PROVIDER_RESPONSES", "true")
	t.Setenv("LOG_RAW_PROVIDER_RESPONSE_FILTERS", "mistral/magistral-*")

	svc := newProviderService()

	assert.True(t, svc.shouldLogRawProviderResponse("mistral", "magistral-medium-2509"))
	assert.False(t, svc.shouldLogRawProviderResponse("mistral", "mistral-large-2411"))
	assert.False(t, svc.shouldLogRawProviderResponse("groq", "llama-3.1-8b-instant"))
}

func TestPrepareRequest_GroqShaping(t *testing.T) {
	svc := newProviderService()
	penalty := 0.5
	req := types.ChatCompletionRequest{
		Messages:         []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
		MaxTokens:        ptrInt(10),
		Metadata:         map[string]string{"trace": "abc"},
		FrequencyPenalty: &penalty,
		PresencePenalty:  &penalty,
	}

	body, err := svc.prepareRequest(req, "llama-3.1-8b-instant", "https://api.groq.com/openai/v1", "openai", types.ProviderAuth{Type: "bearer", Env: "GROQ_API_KEY"})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, float64(10), payload["max_completion_tokens"])
	assert.NotContains(t, payload, "max_tokens")
	assert.NotContains(t, payload, "metadata")
	assert.NotContains(t, payload, "frequency_penalty")
	assert.NotContains(t, payload, "presence_penalty")
}

func TestPrepareRequest_GroqConvertsJSONObjectToJSONSchema(t *testing.T) {
	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages:       []types.OpenAIMessage{{Role: "user", Content: "Return JSON."}},
		ResponseFormat: &types.ResponseFormat{Type: "json_object"},
	}

	body, err := svc.prepareRequest(req, "llama-3.1-8b-instant", "https://api.groq.com/openai/v1", "openai", types.ProviderAuth{Type: "bearer", Env: "GROQ_API_KEY"})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	format, ok := payload["response_format"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "json_schema", format["type"])
	schema, ok := format["json_schema"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "response", schema["name"])
}

func TestPrepareRequest_OCIGeminiKeepsNativeJSONObject(t *testing.T) {
	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages:       []types.OpenAIMessage{{Role: "user", Content: "Return JSON."}},
		ResponseFormat: &types.ResponseFormat{Type: "json_object"},
	}

	body, err := svc.prepareRequest(req, "google.gemini-2.5-flash", "https://inference.generativeai.eu-frankfurt-1.oci.oraclecloud.com/openai/v1", "openai", types.ProviderAuth{Type: "bearer", Env: "OCI_API_KEY"})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	format, ok := payload["response_format"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "json_object", format["type"])
}

func TestPrepareRequest_OpenCodeShaping(t *testing.T) {
	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages:            []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
		MaxCompletionTokens: ptrInt(11),
	}

	body, err := svc.prepareRequest(req, "big-pickle", "https://opencode.ai/zen/v1", "openai", types.ProviderAuth{Type: "bearer", Env: "OPENCODE_ZEN_API_KEY"})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, float64(11), payload["max_tokens"])
	assert.NotContains(t, payload, "max_completion_tokens")
}

func TestPrepareRequest_OpenCodeResponsesBackedModelKeepsMaxCompletionTokens(t *testing.T) {
	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages:  []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
		MaxTokens: ptrInt(11),
	}

	body, err := svc.prepareRequest(req, "muse-spark-1.2-contributor-free", "https://opencode.ai/zen/v1", "openai", types.ProviderAuth{Type: "bearer", Env: "OPENCODE_ZEN_API_KEY"})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, float64(11), payload["max_completion_tokens"])
	assert.NotContains(t, payload, "max_tokens")
}

func TestNormalizeRequestForProvider_OpenCodeTokenParamDialects(t *testing.T) {
	tests := []struct {
		name                string
		model               string
		maxTokens           *int
		maxCompletionTokens *int
		wantMaxTokens       *int
		wantMaxCompTokens   *int
	}{
		{
			name:                "chat dialect maps completion tokens to max_tokens",
			model:               "x-preview-f-free",
			maxCompletionTokens: ptrInt(7),
			wantMaxTokens:       ptrInt(7),
		},
		{
			name:          "chat dialect keeps max_tokens as-is",
			model:         "hy3-free",
			maxTokens:     ptrInt(9),
			wantMaxTokens: ptrInt(9),
		},
		{
			name:              "responses dialect maps max_tokens to completion tokens",
			model:             "muse-spark-1.2-contributor-free",
			maxTokens:         ptrInt(5),
			wantMaxCompTokens: ptrInt(5),
		},
		{
			name:                "responses dialect prefers existing completion tokens",
			model:               "muse-spark-1.2-contributor-free",
			maxTokens:           ptrInt(5),
			maxCompletionTokens: ptrInt(8),
			wantMaxCompTokens:   ptrInt(8),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := types.ChatCompletionRequest{
				Messages:            []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
				MaxTokens:           tt.maxTokens,
				MaxCompletionTokens: tt.maxCompletionTokens,
			}

			got := normalizeRequestForProvider(req, "opencode", tt.model)
			assert.Equal(t, tt.wantMaxTokens, got.MaxTokens)
			assert.Equal(t, tt.wantMaxCompTokens, got.MaxCompletionTokens)
		})
	}
}

func TestProviderCallProvider_CloudflareNativeResponse(t *testing.T) {
	t.Setenv(cloudflareAccountIDEnv, "acct-123")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/accounts/acct-123/ai/run/@cf/openai/gpt-oss-20b", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"success": true,
			"result": {
				"response": "Cloudflare native reply",
				"usage": {
					"prompt_tokens": 31,
					"completion_tokens": 555,
					"total_tokens": 586,
					"prompt_tokens_details": {"cached_tokens": 6}
				}
			}
		}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}}, MaxCompletionTokens: ptrInt(160)}

	resp, err := svc.CallProvider(srv.URL, "test-key", "@cf/openai/gpt-oss-20b", req, 10000, context.Background(), cloudflareProviderType, types.ProviderAuth{Type: "bearer", Env: cloudflareAPITokenEnv}, "")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Choices[0].Message.Content)
	assert.Equal(t, "Cloudflare native reply", *resp.Choices[0].Message.Content)
	assert.Equal(t, 586, resp.Usage.TotalTokens)
	require.NotNil(t, resp.Usage.PromptTokensDetails)
	assert.Equal(t, 6, resp.Usage.PromptTokensDetails.CachedTokens)
}

func TestParseRateLimitDetails_CloudflareDailyAllocationExhausted(t *testing.T) {
	details := parseRateLimitDetails(
		cloudflareProviderID,
		http.Header{"Retry-After": []string{"60"}},
		[]byte(`{"errors":[{"message":"AiError: you have used up your daily free allocation of 10,000 neurons, please upgrade to Cloudflare's Workers Paid plan if you would like to continue usage."}]}`),
	)

	assert.Equal(t, 60, details.RetryAfter)
	assert.Equal(t, "daily_neurons", details.LimitType)
	assert.Equal(t, "quota_exhausted", details.LimitSubtype)
	assert.True(t, details.RetryAfterProvided)
}

func TestParseRateLimitDetails_HTTPDateRetryAfter(t *testing.T) {
	retryAt := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	details := parseRateLimitDetails(
		"openai",
		http.Header{"Retry-After": []string{retryAt}},
		[]byte(`{"error":{"message":"rate limited"}}`),
	)

	assert.GreaterOrEqual(t, details.RetryAfter, 80)
	assert.LessOrEqual(t, details.RetryAfter, 90)
	assert.Equal(t, "rpm", details.LimitType)
	assert.Equal(t, "rate_limit", details.LimitSubtype)
	assert.True(t, details.RetryAfterProvided)
}

func TestParseRateLimitDetails_GeminiQuotaFailure(t *testing.T) {
	body := []byte(`{
		"error": {
			"code": 429,
			"status": "RESOURCE_EXHAUSTED",
			"details": [{
				"@type": "type.googleapis.com/google.rpc.QuotaFailure",
				"violations": [{
					"quotaMetric": "generativelanguage.googleapis.com/generate_content_free_tier_requests",
					"quotaId": "GenerateRequestsPerDayPerProjectPerModel-FreeTier",
					"quotaValue": "20"
				}]
			}]
		}
	}`)

	details := parseRateLimitDetails("gemini", http.Header{}, body)
	quota, ok := parseProviderQuotaDetails(body)

	assert.True(t, ok)
	assert.Equal(t, 60, details.RetryAfter)
	assert.Equal(t, "rpd", details.LimitType)
	assert.Equal(t, "quota_exhausted", details.LimitSubtype)
	assert.False(t, details.RetryAfterProvided, "60s fallback must not masquerade as a provider-supplied retry-after")
	assert.Equal(t, int64(0), details.ResetAtUnixMs, "Gemini supplies no absolute reset timestamp")
	assert.Equal(t, "rpd", quota.LimitType)
	assert.Equal(t, 20, quota.Limit)
	assert.Equal(t, "GenerateRequestsPerDayPerProjectPerModel-FreeTier", quota.ID)
}

func TestParseRateLimitDetails_KiloDailyLimitReached(t *testing.T) {
	body := []byte(`{"error":{"message":"429 limit_rpd/thinkingmachines/inkling-20260715/org/abc Daily limit reached for org-xyz."}}`)

	details := parseRateLimitDetails("kilo", http.Header{}, body)

	assert.Equal(t, "rpd", details.LimitType)
	assert.Equal(t, "quota_exhausted", details.LimitSubtype, "daily-limit-reached bodies are day-scale quota exhaustion")
}

func TestParseRateLimitDetails_OpenRouterFreeModelDailyCap(t *testing.T) {
	resetMs := time.Now().Add(2 * time.Hour).UnixMilli()
	headers := http.Header{
		"X-Ratelimit-Limit":     []string{"1000"},
		"X-Ratelimit-Remaining": []string{"0"},
		"X-Ratelimit-Reset":     []string{strconv.FormatInt(resetMs, 10)},
	}
	body := []byte(fmt.Sprintf(`{
		"error": {
			"message": "Rate limit exceeded: free-models-per-day-stealth. ",
			"code": 429,
			"metadata": {
				"headers": {
					"X-RateLimit-Limit": "1000",
					"X-RateLimit-Remaining": "0",
					"X-RateLimit-Reset": "%d"
				}
			}
		}
	}`, resetMs))

	details := parseRateLimitDetails("openrouter-alpha", headers, body)

	assert.Equal(t, "rpd", details.LimitType)
	assert.Equal(t, "rate_limit", details.LimitSubtype)
	assert.InDelta(t, resetMs, details.ResetAtUnixMs, 1000, "reset epoch-ms must be honored from headers")

	err := errors.NewRateLimitErrorWithSubtype("limited", details.RetryAfter, details.LimitType, details.LimitSubtype, nil)
	applyProviderQuotaSync(err, headers, body)
	assert.Equal(t, 1000, err.ProviderQuotaLimit, "plain X-RateLimit-Limit feeds the rpd local sync")
}

func TestParseResetEpochMs_Variants(t *testing.T) {
	now := time.Now()

	ms, ok := normalizeResetEpochMs(strconv.FormatInt(now.Add(2*time.Hour).UnixMilli(), 10))
	assert.True(t, ok)
	assert.InDelta(t, now.Add(2*time.Hour).UnixMilli(), ms, 1000)

	secs, ok := normalizeResetEpochMs(strconv.FormatInt(now.Add(2*time.Hour).Unix(), 10))
	assert.True(t, ok, "epoch seconds must be scaled to milliseconds")
	assert.InDelta(t, now.Add(2*time.Hour).UnixMilli(), secs, 1000)

	past, ok := normalizeResetEpochMs(strconv.FormatInt(now.Add(-time.Hour).UnixMilli(), 10))
	assert.False(t, ok, "past timestamps are implausible resets")
	assert.Zero(t, past)

	farFuture, ok := normalizeResetEpochMs(strconv.FormatInt(now.Add(72*time.Hour).UnixMilli(), 10))
	assert.False(t, ok, "resets beyond the 48h horizon are rejected")
	assert.Zero(t, farFuture)

	httpDate, ok := normalizeResetEpochMs(now.Add(time.Hour).UTC().Format(http.TimeFormat))
	assert.True(t, ok)
	assert.InDelta(t, now.Add(time.Hour).UnixMilli(), httpDate, 2000)
}

func TestProviderCallProvider_ParsesArrayContentResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":"chatcmpl-array",
			"object":"chat.completion",
			"created":1700000000,
			"model":"magistral-medium-2509",
			"choices":[{
				"index":0,
				"message":{
					"role":"assistant",
					"content":[
						{"type":"thinking","thinking":[{"type":"text","text":"Hidden"}]},
						{"type":"text","text":"Visible answer"}
					]
				},
				"finish_reason":"stop"
			}]
		}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}}}

	resp, err := svc.CallProvider(srv.URL, "key", "magistral-medium-2509", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer", Env: "MISTRAL_API_KEY"}, "")
	require.NoError(t, err)
	require.NotNil(t, resp.Choices[0].Message.Content)
	assert.Equal(t, "Visible answer", *resp.Choices[0].Message.Content)
}

func TestProviderStreamProviderChannel_ForwardsThinkingChunksAsReasoning(t *testing.T) {
	visibleChunk := types.SSEChunk{
		ID: "chunk-visible", Object: "chat.completion.chunk", Model: "magistral-medium-2509",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("Visible")}}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, "data: {\"id\":\"chunk-thinking\",\"object\":\"chat.completion.chunk\",\"model\":\"magistral-medium-2509\",\"choices\":[{\"index\":0,\"delta\":{\"content\":[{\"type\":\"thinking\",\"thinking\":[{\"type\":\"text\",\"text\":\"Hidden\"}]}]},\"finish_reason\":null}]}\n\n")
		flusher.Flush()

		data, _ := json.Marshal(visibleChunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}}, Stream: boolPtr(true)}

	result := svc.StreamProviderChannel(srv.URL, "key", "magistral-medium-2509", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer", Env: "MISTRAL_API_KEY"}, "")

	var received []*types.SSEChunk
	for chunk := range result.Chunks {
		received = append(received, chunk)
	}

	select {
	case gErr := <-result.Err:
		if gErr != nil {
			t.Fatalf("unexpected error: %v", gErr)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for completion signal")
	}

	require.Len(t, received, 2)

	// Mistral-style thinking nodes surface as flat reasoning_content plus
	// structured blocks instead of being silently dropped.
	thinking := received[0].Choices[0].Delta
	assert.Equal(t, "Hidden", *thinking.ReasoningContent)
	assert.Nil(t, thinking.Content)
	require.Len(t, thinking.ThinkingBlocks, 1)
	assert.Equal(t, "thinking", thinking.ThinkingBlocks[0].Type)
	assert.Equal(t, "Hidden", thinking.ThinkingBlocks[0].Thinking)

	require.NotNil(t, received[1].Choices[0].Delta.Content)
	assert.Equal(t, "Visible", *received[1].Choices[0].Delta.Content)
}

// ---------------------------------------------------------------------------
// CallProvider (non-streaming)
// ---------------------------------------------------------------------------

func TestProviderCallProvider_Success(t *testing.T) {
	want := types.ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1700000000,
		Model:   "gpt-4",
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ResponseMessage{
				Role:    "assistant",
				Content: ptrString("Hello!"),
			},
			FinishReason: "stop",
		}},
		Usage: &types.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	got, err := svc.CallProvider(srv.URL, "test-key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")
	require.NoError(t, err)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.Model, got.Model)
	assert.Equal(t, "Hello!", *got.Choices[0].Message.Content)
	assert.Equal(t, 15, got.Usage.TotalTokens)
}

func TestProviderCallProvider_429_RateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")
	require.Error(t, err)

	var rlErr *errors.RateLimitError
	assert.ErrorAs(t, err, &rlErr)
	assert.Equal(t, 30, rlErr.RetryAfter)
}

func TestProviderCallProvider_GroqRateLimitDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit-Requests", "7000")
		w.Header().Set("X-RateLimit-Remaining-Requests", "0")
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit reached","type":"rate_limit_error"}}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}}}

	_, err := svc.CallProvider(srv.URL, "key", "llama-3.1-8b-instant", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer", Env: "GROQ_API_KEY"}, "")
	require.Error(t, err)

	var rlErr *errors.RateLimitError
	require.ErrorAs(t, err, &rlErr)
	assert.Equal(t, 12, rlErr.RetryAfter)
	assert.Equal(t, "rpd", rlErr.LimitType)
	assert.Equal(t, "7000", rlErr.Headers["X-Ratelimit-Limit-Requests"])
}

func TestProviderCallProvider_MistralValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"object":"error","message":{"detail":[{"type":"extra_forbidden","loc":["body","seed"],"msg":"Extra inputs are not permitted"}]},"type":"invalid_request_error"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}}}

	_, err := svc.CallProvider(srv.URL, "key", "mistral-large-2411", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer", Env: "MISTRAL_API_KEY"}, "")
	require.Error(t, err)

	var validationErr *errors.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Contains(t, validationErr.Message, "extra_forbidden")
}

func TestProviderCallProvider_NIMDegradedFunctionIsProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":400,"title":"Bad Request","detail":"Function id 'abc': DEGRADED function cannot be invoked"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}}}

	_, err := svc.CallProvider(srv.URL, "key", "qwen/qwen3-next-80b-a3b-instruct", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer", Env: "NIM_API_KEY"}, "")
	require.Error(t, err)

	var providerErr *errors.ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, http.StatusBadRequest, providerErr.StatusCode)
	assert.False(t, providerErr.IsRetryable)
	assert.Contains(t, providerErr.Message, "DEGRADED function cannot be invoked")
}

func TestProviderCallProvider_ResponseFormatSchemaDialectErrorFailsOver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"invalid JSON schema for response_format: 'enrichment': /properties/experience/anyOf/0/required: ` + "`required`" + ` is required to be supplied and to be an array including every key in properties. The following properties must be listed in ` + "`required`" + `: max, min"}}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}}}

	_, err := svc.CallProvider(srv.URL, "key", "openai/gpt-oss-20b", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer", Env: "GROQ_API_KEY"}, "")
	require.Error(t, err)

	var providerErr *errors.ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, http.StatusBadRequest, providerErr.StatusCode)
	assert.False(t, providerErr.IsRetryable)
}

func TestProviderCallProvider_UnsupportedResponseFormatFailsOver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"This model does not support response format ` + "`json_schema`" + `"}}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}}}

	_, err := svc.CallProvider(srv.URL, "key", "json-object-model", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")
	require.Error(t, err)

	var providerErr *errors.ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, http.StatusBadRequest, providerErr.StatusCode)
	assert.False(t, providerErr.IsRetryable)

	var validationErr *errors.ValidationError
	assert.NotErrorAs(t, err, &validationErr)
}

func TestProviderCallProvider_RetiredModelIsProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		w.Write([]byte(`{"error":"qwen3-next:80b was retired at 2026-06-16 00:00:00 -0700 PDT"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}}}

	_, err := svc.CallProvider(srv.URL, "key", "qwen3-next:80b", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer", Env: "OLLAMA_API_KEY"}, "")
	require.Error(t, err)

	var providerErr *errors.ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, http.StatusGone, providerErr.StatusCode)
	assert.False(t, providerErr.IsRetryable)
	assert.Contains(t, providerErr.Message, "was retired")
}

func TestClassifyProviderHTTPError_StructuredOutputProvider400IsProviderError(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "oci gemini unsupported schema feature",
			body: `{"error":{"message":"Unsupported JSON Schema feature for Gemini: anyOf"}}`,
		},
		{
			name: "groq failed generation strict json",
			body: `{"error":{"message":"Failed to validate JSON. Please adjust your prompt. See 'failed_generation' for more details.","failed_generation":"{}"}}`,
		},
		{
			name: "bai unavailable response format type",
			body: `{"error":{"message":"The request is invalid: This response_format type is unavailable now. Please check the request body, required fields, and request format."}}`,
		},
		{
			name: "provider response format disabled",
			body: `{"error":{"message":"response_format is disabled for this model"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyProviderHTTPError("oci", http.StatusBadRequest, nil, []byte(tt.body))

			var providerErr *errors.ProviderError
			require.ErrorAs(t, err, &providerErr)
			assert.Equal(t, http.StatusBadRequest, providerErr.StatusCode)

			var validationErr *errors.ValidationError
			assert.NotErrorAs(t, err, &validationErr)
		})
	}
}

func TestProviderCallProvider_402_PaymentRequiredError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error":"payment required"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")
	require.Error(t, err)

	var pErr *errors.PaymentRequiredError
	assert.ErrorAs(t, err, &pErr)
}

func TestProviderCallProvider_500_RetryableProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal server error`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")
	require.Error(t, err)

	var pErr *errors.ProviderError
	require.ErrorAs(t, err, &pErr)
	assert.True(t, pErr.IsRetryable)
	assert.Equal(t, 500, pErr.StatusCode)
}

func TestProviderCallProvider_BearerAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer my-secret-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ChatCompletionResponse{
			ID: "test", Object: "chat.completion", Model: "gpt-4",
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: ptrString("ok")}, FinishReason: "stop"}},
		})
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "my-secret-key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")
	require.NoError(t, err)
}

func TestProviderCallProvider_CustomHeaderAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "my-api-key", r.Header.Get("X-Custom-Auth"))
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ChatCompletionResponse{
			ID: "test", Object: "chat.completion", Model: "gpt-4",
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: ptrString("ok")}, FinishReason: "stop"}},
		})
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "my-api-key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{
		Type:       "header",
		HeaderName: "X-Custom-Auth",
	}, "")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// StreamProviderChannel
// ---------------------------------------------------------------------------

func TestProviderStreamProviderChannel_Success(t *testing.T) {
	chunk1 := types.SSEChunk{
		ID: "chunk-1", Object: "chat.completion.chunk", Model: "gpt-4",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("Hel")}}},
	}
	chunk2 := types.SSEChunk{
		ID: "chunk-2", Object: "chat.completion.chunk", Model: "gpt-4",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("lo!")}}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		data1, _ := json.Marshal(chunk1)
		fmt.Fprintf(w, "data: %s\n\n", data1)
		flusher.Flush()

		data2, _ := json.Marshal(chunk2)
		fmt.Fprintf(w, "data: %s\n\n", data2)
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	result := svc.StreamProviderChannel(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")

	var received []*types.SSEChunk
	for chunk := range result.Chunks {
		received = append(received, chunk)
	}

	select {
	case gErr := <-result.Err:
		if gErr != nil {
			t.Fatalf("unexpected error: %v", gErr)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for completion signal")
	}

	require.Len(t, received, 2)
	assert.Equal(t, "chunk-1", received[0].ID)
	assert.Equal(t, "Hel", *received[0].Choices[0].Delta.Content)
	assert.Equal(t, "chunk-2", received[1].ID)
	assert.Equal(t, "lo!", *received[1].Choices[0].Delta.Content)
}

func TestProviderStreamProviderChannel_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "15")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	result := svc.StreamProviderChannel(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")

	for range result.Chunks {
	}

	select {
	case gErr := <-result.Err:
		require.NotNil(t, gErr)
		assert.Equal(t, "RATE_LIMITED", gErr.Code)
		assert.Equal(t, 15, gErr.Details["retry_after"])
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for error")
	}
}

func TestProviderStreamProviderChannel_DoneTerminates(t *testing.T) {
	chunk1 := types.SSEChunk{
		ID: "c1", Object: "chat.completion.chunk", Model: "gpt-4",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("A")}}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		data, _ := json.Marshal(chunk1)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	result := svc.StreamProviderChannel(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")

	count := 0
	for range result.Chunks {
		count++
	}

	assert.Equal(t, 1, count)

	select {
	case gErr := <-result.Err:
		if gErr != nil {
			t.Fatalf("unexpected error: %v", gErr)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for completion signal")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func ptrString(s string) *string { return &s }

func TestDetectProvider_NewProviders(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      string
		authEnv      string
		expectedProv string
	}{
		{"bai by env", "https://api.example.com", "BAI_API_KEY", "bai"},
		{"bai by url", "https://api.b.ai/v1", "", "bai"},
		{"inferx by env", "https://api.example.com", "INFERX_API_KEY", "inferx"},
		{"inferx by url", "https://model.inferx.net/endpoints/v1", "", "inferx"},
		{"gmi by env", "https://api.example.com", "GMI_API_KEY", "gmi"},
		{"gmi by url", "https://api.gmi-serving.com/v1", "", "gmi"},
		{"orca by env", "https://api.example.com", "ORCAROUTER_API_KEY", "orca"},
		{"orca by url", "https://api.orcarouter.ai/v1", "", "orca"},
		{"vercel by env", "https://api.example.com", "AI_GATEWAY_API_KEY", "vercel"},
		{"vercel by url", "https://ai-gateway.vercel.sh/v1", "", "vercel"},
		{"empero by env", "https://api.example.com", "EMPERO_API_KEY", "empero"},
		{"empero by url", "https://free.empero.org/v1", "", "empero"},
		{"tokenharbor by env standard", "https://api.example.com", "TOKENHARBOR_API_KEY", "tokenharbor"},
		{"tokenharbor by env short", "https://api.example.com", "TH", "tokenharbor"},
		{"tokenharbor by url", "https://tokenharbor.ai/v1", "", "tokenharbor"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectProvider(tc.baseURL, "openai", types.ProviderAuth{Type: "bearer", Env: tc.authEnv})
			assert.Equal(t, tc.expectedProv, got)
		})
	}
}

func TestParseRateLimitDetails_Orca(t *testing.T) {
	t.Run("with retry-after header is rate limit", func(t *testing.T) {
		headers := http.Header{"Retry-After": []string{"42"}}
		body := []byte(`{"error":{"code":"free_rate_limited","message":"rate limit exceeded"}}`)
		details := parseRateLimitDetails("orca", headers, body)

		assert.Equal(t, 42, details.RetryAfter)
		assert.True(t, details.RetryAfterProvided)
		assert.Equal(t, "rpm", details.LimitType)
		assert.Equal(t, "rate_limit", details.LimitSubtype)
	})

	t.Run("without retry-after is prompt cap ceiling", func(t *testing.T) {
		headers := http.Header{}
		body := []byte(`{"error":{"code":"free_rate_limited","message":"prompt tokens exceed free tier allowance"}}`)
		details := parseRateLimitDetails("orca", headers, body)

		assert.Equal(t, 0, details.RetryAfter)
		assert.False(t, details.RetryAfterProvided)
		assert.Equal(t, "prompt_cap", details.LimitType)
		assert.Equal(t, "prompt_too_large", details.LimitSubtype)
	})
}

