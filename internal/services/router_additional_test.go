package services_test

import (
	"context"
	"testing"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/services/mocks"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Ensure mocks import is used
var _ = mocks.NewMockQuotaChecker

// ---------------------------------------------------------------------------
// Additional DeriveRequirements Tests (from TypeScript)
// ---------------------------------------------------------------------------

func TestDeriveRequirements_EdgeCases(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	t.Run("json_object type defaults to text", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			ResponseFormat: &types.ResponseFormat{
				Type: "json_object",
			},
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "text", reqs.Output)
	})

	t.Run("text type response format", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			ResponseFormat: &types.ResponseFormat{
				Type: "text",
			},
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "text", reqs.Output)
	})

	t.Run("json_schema strict false defaults to text", func(t *testing.T) {
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
		assert.Equal(t, "text", reqs.Output)
	})

	t.Run("empty tools array is forbidden", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			Tools: []types.OpenAITool{},
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "forbidden", reqs.Tools)
	})

	t.Run("tools allowed with auto tool_choice", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			Tools:      []types.OpenAITool{{Type: "function", Function: types.Function{Name: "test"}}},
			ToolChoice: "auto",
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "allowed", reqs.Tools)
	})

	t.Run("complex scenario - streaming and strict JSON", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			Stream: boolPtr(true),
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
		assert.Equal(t, "required", reqs.Streaming)
	})

	t.Run("complex scenario - tools and streaming", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			Stream:     boolPtr(true),
			Tools:      []types.OpenAITool{{Type: "function", Function: types.Function{Name: "test"}}},
			ToolChoice: "auto",
		}
		reqs := r.DeriveRequirements(req, nil)
		assert.Equal(t, "required", reqs.Streaming)
		assert.Equal(t, "allowed", reqs.Tools)
	})

	t.Run("router hints do not override when nil", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			Stream: boolPtr(true),
		}
		hints := &types.RouterHints{
			Strategy: strPtr("cheap_fast"),
		}
		reqs := r.DeriveRequirements(req, hints)
		assert.Equal(t, "required", reqs.Streaming)
		assert.Equal(t, "text", reqs.Output)
		assert.Equal(t, "forbidden", reqs.Tools)
	})
}

// ---------------------------------------------------------------------------
// Additional ScoreCandidates Tests (from TypeScript)
// ---------------------------------------------------------------------------

func TestScoreCandidates_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("empty candidates array", func(t *testing.T) {
		r, _, _, _ := newTestRouter(t)
		scored := r.ScoreCandidates(ctx, []types.RoutingCandidate{}, nil)
		assert.Empty(t, scored)
	})

	t.Run("single candidate", func(t *testing.T) {
		r, _, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-a", "model-1").Return(services.HealthMetrics{HealthScore: 1.0})

		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", ScoreBreakdown: map[string]float64{}},
		}
		scored := r.ScoreCandidates(ctx, candidates, nil)
		assert.Len(t, scored, 1)
		assert.Equal(t, "model-1", scored[0].Model)
	})

	t.Run("preference bonus decreases by rank", func(t *testing.T) {
		r, _, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-a", "model-1").Return(services.HealthMetrics{HealthScore: 1.0})
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-b", "model-3").Return(services.HealthMetrics{HealthScore: 1.0})

		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", ScoreBreakdown: map[string]float64{}},
			{Provider: testConfig().Providers[1], Model: "model-3", ScoreBreakdown: map[string]float64{}},
		}
		hints := &types.RouterHints{
			Providers: &types.ProviderPreferences{
				Prefer: []string{"provider-a", "provider-b"},
			},
		}

		scored := r.ScoreCandidates(ctx, candidates, hints)

		// First provider gets higher bonus
		firstBonus := scored[0].ScoreBreakdown["preference_bonus"]
		secondBonus := scored[1].ScoreBreakdown["preference_bonus"]
		assert.Greater(t, firstBonus, secondBonus)
	})

	t.Run("no preference bonus for non-preferred providers", func(t *testing.T) {
		r, _, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-a", "model-1").Return(services.HealthMetrics{HealthScore: 1.0})

		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", ScoreBreakdown: map[string]float64{}},
		}
		hints := &types.RouterHints{
			Providers: &types.ProviderPreferences{
				Prefer: []string{"provider-b"},
			},
		}

		scored := r.ScoreCandidates(ctx, candidates, hints)
		assert.NotContains(t, scored[0].ScoreBreakdown, "preference_bonus")
	})

	t.Run("preserves existing scores", func(t *testing.T) {
		r, _, mockHealth, _ := newTestRouter(t)
		mockHealth.EXPECT().GetHealthMetrics(gomock.Any(), "provider-a", "model-1").Return(services.HealthMetrics{HealthScore: 1.0})

		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", Score: 5.0, ScoreBreakdown: map[string]float64{"existing": 5.0}},
		}

		scored := r.ScoreCandidates(ctx, candidates, nil)
		assert.Greater(t, scored[0].Score, 5.0)
		assert.Contains(t, scored[0].ScoreBreakdown, "existing")
	})
}

// ---------------------------------------------------------------------------
// Additional CompilePlan Tests (from TypeScript)
// ---------------------------------------------------------------------------

func TestCompilePlan_EdgeCases(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	t.Run("handles fewer candidates than max_attempts", func(t *testing.T) {
		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", Score: 1.0},
		}
		hints := &types.RouterHints{
			Fallback: &types.FallbackConfig{MaxAttempts: intPtr(5)},
		}

		plan := r.CompilePlan(candidates, hints, nil)
		assert.Len(t, plan.Attempts, 1)
	})

	t.Run("partial retry policy override", func(t *testing.T) {
		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", Score: 1.0},
		}
		hints := &types.RouterHints{
			Fallback: &types.FallbackConfig{
				On429: boolPtr(false),
			},
		}

		plan := r.CompilePlan(candidates, hints, nil)
		assert.False(t, plan.RetryOn429)
		assert.True(t, plan.RetryOnTimeout)
		assert.True(t, plan.RetryOn5xx)
	})

	t.Run("attempt structure includes all fields", func(t *testing.T) {
		candidates := []types.RoutingCandidate{
			{Provider: testConfig().Providers[0], Model: "model-1", Score: 1.5},
		}

		plan := r.CompilePlan(candidates, nil, nil)
		require.Len(t, plan.Attempts, 1)

		attempt := plan.Attempts[0]
		assert.Equal(t, "provider-a", attempt.ProviderID)
		assert.Equal(t, "model-1", attempt.Model)
		assert.NotEmpty(t, attempt.BaseURL)
		assert.NotEmpty(t, attempt.APIKey)
		assert.Equal(t, 1.5, attempt.Score)
		assert.Equal(t, 300000, attempt.TimeoutMs)
	})
}

// ---------------------------------------------------------------------------
// Additional ShouldRetry Tests (from TypeScript - edge cases)
// ---------------------------------------------------------------------------

func TestShouldRetry_EdgeCases(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	twoAttempts := types.RoutingPlan{
		Attempts:       make([]types.RoutingAttempt, 2),
		RetryOn429:     true,
		RetryOnTimeout: true,
		RetryOn5xx:     true,
	}

	t.Run("not retry on last attempt", func(t *testing.T) {
		err := errors.NewRateLimitError("rate limited", 60, "rpm")
		// Index 1 is the last of 2 attempts
		result := r.ShouldRetry(err, twoAttempts, 1)
		assert.False(t, result)
	})

	t.Run("no retry when no more attempts", func(t *testing.T) {
		err := errors.NewRateLimitError("rate limited", 60, "rpm")
		plan := types.RoutingPlan{Attempts: []types.RoutingAttempt{}}
		result := r.ShouldRetry(err, plan, 0)
		assert.False(t, result)
	})

	t.Run("retry on first attempt when multiple remain", func(t *testing.T) {
		err := &errors.ProviderError{Message: "error", StatusCode: 503, IsRetryable: true}
		result := r.ShouldRetry(err, twoAttempts, 0)
		assert.True(t, result)
	})

	t.Run("negative status code with IsRetryable=true is retryable", func(t *testing.T) {
		// Implementation checks IsRetryable flag, not just status code
		err := &errors.ProviderError{Message: "error", StatusCode: -1, IsRetryable: true}
		result := r.ShouldRetry(err, twoAttempts, 0)
		assert.True(t, result)
	})

	t.Run("status code 599 (edge of 5xx) is retryable", func(t *testing.T) {
		err := &errors.ProviderError{Message: "error", StatusCode: 599, IsRetryable: true}
		result := r.ShouldRetry(err, twoAttempts, 0)
		assert.True(t, result)
	})

	t.Run("CircuitBreakerError triggers retry", func(t *testing.T) {
		err := errors.NewCircuitBreakerError("circuit open", "provider-a", "OPEN")
		result := r.ShouldRetry(err, twoAttempts, 0)
		assert.True(t, result)
	})

	t.Run("ModelQuotaExceededError triggers retry", func(t *testing.T) {
		err := errors.NewModelQuotaExceededError("quota exceeded", "provider-a", "model-1", "rpm")
		result := r.ShouldRetry(err, twoAttempts, 0)
		assert.True(t, result)
	})

	t.Run("ValidationError does not trigger retry", func(t *testing.T) {
		err := errors.NewValidationError("validation failed", nil)
		result := r.ShouldRetry(err, twoAttempts, 0)
		assert.False(t, result)
	})
}

// ---------------------------------------------------------------------------
// Additional CreateGatewayError Tests (from TypeScript)
// ---------------------------------------------------------------------------

func TestCreateGatewayError_EdgeCases(t *testing.T) {
	r, _, _, _ := newTestRouter(t)

	t.Run("generic error becomes PROVIDER_ERROR", func(t *testing.T) {
		err := r.CreateGatewayError(assert.AnError, 1, "req-123")
		assert.Equal(t, "PROVIDER_ERROR", err.Code)
		assert.Equal(t, "upstream_error", err.Type)
	})

	t.Run("preserves error message", func(t *testing.T) {
		customErr := &errors.ProviderError{Message: "Custom error message", StatusCode: 500, IsRetryable: true}
		gatewayErr := r.CreateGatewayError(customErr, 1, "req-123")
		assert.Contains(t, gatewayErr.Message, "Custom error message")
	})

	t.Run("includes attempts in details", func(t *testing.T) {
		err := r.CreateGatewayError(assert.AnError, 3, "req-123")
		assert.Equal(t, 3, err.Details["attempts"])
	})

	t.Run("handles zero attempts", func(t *testing.T) {
		err := r.CreateGatewayError(assert.AnError, 0, "req-123")
		assert.Equal(t, 0, err.Details["attempts"])
	})

	t.Run("handles single attempt", func(t *testing.T) {
		pErr := &errors.ProviderError{Message: "Error", StatusCode: 500, IsRetryable: true}
		err := r.CreateGatewayError(pErr, 1, "req-123")
		assert.Equal(t, 1, err.Details["attempts"])
	})

	t.Run("handles error with no message", func(t *testing.T) {
		err := r.CreateGatewayError(&errors.ProviderError{StatusCode: 500, IsRetryable: true}, 1, "req-123")
		assert.Equal(t, "PROVIDER_ERROR", err.Code)
	})
}

// Note: strPtr is defined in router_test.go
