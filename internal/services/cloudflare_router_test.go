package services

import (
	"context"
	"net/http"
	"testing"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubHealthChecker struct{}

func (stubHealthChecker) CanExecute(ctx context.Context, providerID, model string) bool { return true }
func (stubHealthChecker) GetCircuitState(ctx context.Context, providerID, model string) CircuitState {
	return StateClosed
}
func (stubHealthChecker) CheckCircuitBreaker(ctx context.Context, providerID, model string) error {
	return nil
}
func (stubHealthChecker) RecordSuccess(ctx context.Context, providerID, model string, latencyMs int) {
}
func (stubHealthChecker) RecordFailure(ctx context.Context, providerID, model string) {}
func (stubHealthChecker) GetHealthMetrics(ctx context.Context, providerID, model string) HealthMetrics {
	return HealthMetrics{}
}
func (stubHealthChecker) GetAllHealthMetrics(ctx context.Context) []HealthMetrics { return nil }

type stubProviderCaller struct{}

func (stubProviderCaller) CallProvider(baseURL, apiKey, model string, request types.ChatCompletionRequest, timeoutMs int, ctx context.Context, providerType string, auth types.ProviderAuth) (*types.ChatCompletionResponse, error) {
	return nil, nil
}
func (stubProviderCaller) StreamProviderChannel(baseURL, apiKey, model string, request types.ChatCompletionRequest, timeoutMs int, ctx context.Context, providerType string, auth types.ProviderAuth) types.StreamResult {
	chunks := make(chan *types.SSEChunk)
	errCh := make(chan *types.GatewayError, 1)
	close(chunks)
	errCh <- nil
	close(errCh)
	return types.StreamResult{Chunks: chunks, Err: errCh}
}

func TestRouterMarksCloudflareBudgetExhaustedAndSkipsFurtherAttempts(t *testing.T) {
	client, _ := newTestRedis(t)
	quotaSvc := NewQuotaService(client, "")
	r := NewRouterWithConfig(types.AppConfig{}, quotaSvc, stubHealthChecker{}, stubProviderCaller{})
	ctx := testContext()

	rateLimitErr := errors.NewRateLimitErrorWithSubtype(
		"daily allocation exhausted",
		60,
		"daily_neurons",
		"quota_exhausted",
		nil,
	)
	r.maybeMarkCloudflareDailyBudgetExhausted(ctx, cloudflareProviderID, rateLimitErr)

	assert.Equal(t, 0, quotaSvc.GetCloudflareRemainingDailyNeurons(ctx))

	err := r.checkCloudflareAttemptBudget(ctx, types.RoutingAttempt{
		ProviderID: cloudflareProviderID,
		Model:      "@cf/moonshotai/kimi-k2.6",
	}, types.ChatCompletionRequest{
		Messages:  []types.OpenAIMessage{{Role: "user", Content: "hi"}},
		MaxTokens: intPtr(160),
	})
	require.Error(t, err)
	var quotaErr *errors.ModelQuotaExceededError
	require.ErrorAs(t, err, &quotaErr)
	assert.Equal(t, "daily_neurons", quotaErr.LimitType)

	resp := buildRateLimitResponse(rateLimitErr)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}
