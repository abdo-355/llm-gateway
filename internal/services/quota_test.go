package services

import (
	"net/http"

	"testing"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int { return &v }

func TestQuotaEstimateTokens(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "")

	tests := []struct {
		name string
		req  types.ChatCompletionRequest
		min  int
		max  int
	}{
		{
			name: "simple text message",
			req: types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{
					{Role: "user", Content: "Hello, world!"},
				},
			},
			min: 250,
			max: 1500,
		},
		{
			name: "message with tool calls",
			req: types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{
					{
						Role:    "assistant",
						Content: "Let me help.",
						ToolCalls: []types.ToolCall{
							{
								ID:   "call_1",
								Type: "function",
								Function: types.FunctionCall{
									Name:      "get_weather",
									Arguments: `{"location":"NYC"}`,
								},
							},
						},
					},
				},
			},
			min: 250,
			max: 1500,
		},
		{
			name: "message with max_tokens set",
			req: types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{
					{Role: "user", Content: "Hi"},
				},
				MaxTokens: intPtr(500),
			},
			min: 500,
			max: 600,
		},
		{
			name: "message with max_completion_tokens set",
			req: types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{
					{Role: "user", Content: "Hi"},
				},
				MaxCompletionTokens: intPtr(200),
			},
			min: 200,
			max: 300,
		},
		{
			name: "message with content array (multimodal)",
			req: types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{
					{
						Role: "user",
						Content: []any{
							map[string]any{"type": "text", "text": "What is in this image?"},
							map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/img.png"}},
						},
					},
				},
			},
			min: 250,
			max: 1500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := svc.EstimateTokens(tt.req)
			assert.GreaterOrEqual(t, tokens, tt.min, "tokens should be >= %d, got %d", tt.min, tokens)
			assert.LessOrEqual(t, tokens, tt.max, "tokens should be <= %d, got %d", tt.max, tokens)
		})
	}
}

func TestQuotaEstimateTokens_MaxTokensUsed(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "")

	withDefault := svc.EstimateTokens(types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	})
	withCustom := svc.EstimateTokens(types.ChatCompletionRequest{
		Messages:  []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
		MaxTokens: intPtr(100),
	})

	assert.Greater(t, withDefault, withCustom, "default max_tokens (1000) should produce higher estimate than 100")
}

func TestQuotaCheckModelQuota(t *testing.T) {
	tests := []struct {
		name            string
		limits          types.ModelLimits
		estimatedTokens int
		preRecord       int
		preTokens       int
		wantErr         bool
		wantLimitType   string
	}{
		{
			name: "all quotas within limits",
			limits: types.ModelLimits{
				Rpm: intPtr(100),
				Tpm: intPtr(100000),
				Rph: intPtr(1000),
			},
			estimatedTokens: 500,
			preRecord:       0,
			wantErr:         false,
		},
		{
			name: "RPM exceeded",
			limits: types.ModelLimits{
				Rpm: intPtr(2),
			},
			estimatedTokens: 100,
			preRecord:       2,
			preTokens:       100,
			wantErr:         true,
			wantLimitType:   "rpm",
		},
		{
			name: "TPM exceeded",
			limits: types.ModelLimits{
				Tpm: intPtr(500),
			},
			estimatedTokens: 600,
			preRecord:       0,
			wantErr:         true,
			wantLimitType:   "tpm",
		},
		{
			name: "RPH exceeded",
			limits: types.ModelLimits{
				Rph: intPtr(3),
			},
			estimatedTokens: 100,
			preRecord:       3,
			preTokens:       100,
			wantErr:         true,
			wantLimitType:   "rph",
		},
		{
			name:            "no limits set (nil pointers) always passes",
			limits:          types.ModelLimits{},
			estimatedTokens: 999999,
			preRecord:       10,
			preTokens:       100,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newTestRedis(t)
			svc := NewQuotaService(client, "")
			ctx := testContext()

			for i := 0; i < tt.preRecord; i++ {
				tokens := tt.preTokens
				if tokens == 0 {
					tokens = 100
				}
				err := svc.RecordModelUsage(ctx, "provider1", "model1", tokens)
				require.NoError(t, err)
			}

			err := svc.CheckModelQuota(ctx, "provider1", "model1", tt.limits, tt.estimatedTokens)

			if tt.wantErr {
				require.Error(t, err)
				var quotaErr *errors.ModelQuotaExceededError
				require.ErrorAs(t, err, &quotaErr)
				assert.Equal(t, tt.wantLimitType, quotaErr.LimitType)
				assert.Equal(t, "provider1", quotaErr.ProviderID)
				assert.Equal(t, "model1", quotaErr.Model)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestQuotaRecordModelUsage(t *testing.T) {
	client, mr := newTestRedis(t)
	svc := NewQuotaService(client, "")
	ctx := testContext()

	err := svc.RecordModelUsage(ctx, "prov1", "gpt-4", 150)
	require.NoError(t, err)

	rpmKeys := mr.Keys()
	var rpmKey string
	for _, k := range rpmKeys {
		if len(k) > 0 {
			if k == "quota:prov1:gpt-4:rpm" {
				rpmKey = k
			}
		}
	}
	require.NotEmpty(t, rpmKey, "RPM sorted set key should exist")

	members, err := mr.ZMembers(rpmKey)
	require.NoError(t, err)
	assert.Len(t, members, 1, "RPM sorted set should have 1 entry")

	status := svc.GetModelQuotaStatus(ctx, "prov1", "gpt-4", nil)
	assert.Equal(t, 1, status.Rpm, "RPM should be 1")
	assert.Equal(t, 1, status.Rph, "RPH should be 1")
	assert.Equal(t, 1, status.Rpd, "RPD should be 1")
	assert.Equal(t, 150, status.Tpm, "TPM should be 150")
	assert.Equal(t, 150, status.Tph, "TPH should be 150")
	assert.Equal(t, 150, status.Tpd, "TPD should be 150")
	assert.Equal(t, 150, status.Tpmu, "TPMU should be 150")
}

func TestQuotaRecordModelUsage_MultipleRecords(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "")
	ctx := testContext()

	require.NoError(t, svc.RecordModelUsage(ctx, "prov1", "gpt-4", 100))
	require.NoError(t, svc.RecordModelUsage(ctx, "prov1", "gpt-4", 250))
	require.NoError(t, svc.RecordModelUsage(ctx, "prov1", "gpt-4", 50))

	status := svc.GetModelQuotaStatus(ctx, "prov1", "gpt-4", nil)
	assert.Equal(t, 3, status.Rpm, "RPM should be 3 after 3 records")
	assert.Equal(t, 3, status.Rph, "RPH should be 3")
	assert.Equal(t, 3, status.Rpd, "RPD should be 3")
	assert.Equal(t, 400, status.Tpm, "TPM should be 400 (100+250+50)")
	assert.Equal(t, 400, status.Tph, "TPH should be 400 (100+250+50)")
	assert.Equal(t, 400, status.Tpd, "TPD should be 400")
	assert.Equal(t, 400, status.Tpmu, "TPMU should be 400")
}

func TestQuotaHandleProviderRateLimit(t *testing.T) {
	tests := []struct {
		name            string
		providerID      string
		statusCode      int
		headers         map[string]string
		wantRateLimited bool
		wantPayment     bool
		wantRetryAfter  int
	}{
		{
			name:            "429 status returns IsRateLimited true",
			providerID:      "prov1",
			statusCode:      429,
			wantRateLimited: true,
		},
		{
			name:       "429 with Retry-After header",
			providerID: "prov1",
			statusCode: 429,
			headers: map[string]string{
				"Retry-After": "30",
			},
			wantRateLimited: true,
			wantRetryAfter:  30,
		},
		{
			name:       "groq request headers treated as daily",
			providerID: "groq",
			statusCode: 429,
			headers: map[string]string{
				"X-RateLimit-Limit-Requests":     "7000",
				"X-RateLimit-Remaining-Requests": "0",
			},
			wantRateLimited: true,
			wantRetryAfter:  0,
		},
		{
			name:            "402 status returns IsPaymentRequired true",
			providerID:      "prov1",
			statusCode:      402,
			wantRateLimited: true,
			wantPayment:     true,
		},
		{
			name:            "200 status returns zero value",
			providerID:      "prov1",
			statusCode:      200,
			wantRateLimited: false,
			wantPayment:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newTestRedis(t)
			svc := NewQuotaService(client, "")
			ctx := testContext()

			resp := &http.Response{
				StatusCode: tt.statusCode,
				Header:     http.Header{},
			}
			for k, v := range tt.headers {
				resp.Header.Set(k, v)
			}
			resp.Body = http.NoBody

			info := svc.HandleProviderRateLimit(ctx, tt.providerID, "model1", resp)
			assert.Equal(t, tt.wantRateLimited, info.IsRateLimited)
			assert.Equal(t, tt.wantPayment, info.IsPaymentRequired)
			assert.Equal(t, tt.wantRetryAfter, info.RetryAfter)
			if tt.name == "groq request headers treated as daily" {
				assert.Equal(t, "rpd", info.LimitType)
			}
		})
	}
}

func TestQuotaHandleProviderRateLimit_UpdatesRedis(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "")
	ctx := testContext()

	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{},
		Body:       http.NoBody,
	}
	resp.Header.Set("X-RateLimit-Limit-Requests", "100")
	resp.Header.Set("X-RateLimit-Remaining-Requests", "10")

	_ = svc.HandleProviderRateLimit(ctx, "prov1", "model1", resp)

	status := svc.GetModelQuotaStatus(ctx, "prov1", "model1", nil)
	assert.Equal(t, 90, status.Rpm)

	err := svc.CheckModelQuota(ctx, "prov1", "model1", types.ModelLimits{Rpm: intPtr(90)}, 1)
	require.Error(t, err)
}

func TestQuotaHandleProviderRateLimit_GroqDailyHeaders(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "")
	ctx := testContext()

	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: http.NoBody}
	resp.Header.Set("X-RateLimit-Limit-Requests", "7000")
	resp.Header.Set("X-RateLimit-Remaining-Requests", "6990")

	info := svc.HandleProviderRateLimit(ctx, "groq", "model1", resp)
	assert.Equal(t, "rpd", info.LimitType)

	status := svc.GetModelQuotaStatus(ctx, "groq", "model1", nil)
	assert.Equal(t, 10, status.Rpd)
}

func TestQuotaHandleProviderRateLimit_CerebrasMinuteHeaders(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "")
	ctx := testContext()

	resp := &http.Response{StatusCode: 429, Header: http.Header{}, Body: http.NoBody}
	resp.Header.Set("X-RateLimit-Limit-Requests-Minute", "30")
	resp.Header.Set("X-RateLimit-Remaining-Requests-Minute", "5")

	info := svc.HandleProviderRateLimit(ctx, "cerebras", "model1", resp)
	assert.Equal(t, "rpm", info.LimitType)

	status := svc.GetModelQuotaStatus(ctx, "cerebras", "model1", nil)
	assert.Equal(t, 25, status.Rpm)
}

func TestQuotaGetModelQuotaStatus(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "")
	ctx := testContext()

	status := svc.GetModelQuotaStatus(ctx, "prov1", "gpt-4", nil)
	assert.Equal(t, 0, status.Rpm)
	assert.Equal(t, 0, status.Rph)
	assert.Equal(t, 0, status.Rpd)
	assert.Equal(t, 0, status.Tpm)
	assert.Equal(t, 0, status.Tph)
	assert.Equal(t, 0, status.Tpd)
	assert.Equal(t, 0, status.Tpmu)

	require.NoError(t, svc.RecordModelUsage(ctx, "prov1", "gpt-4", 200))
	require.NoError(t, svc.RecordModelUsage(ctx, "prov1", "gpt-4", 300))

	status = svc.GetModelQuotaStatus(ctx, "prov1", "gpt-4", nil)
	assert.Equal(t, 2, status.Rpm)
	assert.Equal(t, 2, status.Rph)
	assert.Equal(t, 2, status.Rpd)
	assert.Equal(t, 500, status.Tpm)
	assert.Equal(t, 500, status.Tph)
	assert.Equal(t, 500, status.Tpd)
	assert.Equal(t, 500, status.Tpmu)
}

func TestQuotaGetModelQuotaStatus_IsolatesModels(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "")
	ctx := testContext()

	require.NoError(t, svc.RecordModelUsage(ctx, "prov1", "gpt-4", 100))
	require.NoError(t, svc.RecordModelUsage(ctx, "prov1", "gpt-3.5", 200))

	s1 := svc.GetModelQuotaStatus(ctx, "prov1", "gpt-4", nil)
	s2 := svc.GetModelQuotaStatus(ctx, "prov1", "gpt-3.5", nil)

	assert.Equal(t, 1, s1.Rpm)
	assert.Equal(t, 100, s1.Tph)

	assert.Equal(t, 1, s2.Rpm)
	assert.Equal(t, 200, s2.Tph)
}
