package services

import (
	"net/http"
	"testing"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dayQuotaTestRouter(t *testing.T, providers []types.ProviderConfig) *Router {
	t.Helper()
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "quota")
	healthSvc := NewHealthService(client, "health")
	cd := NewCooldownService(client, "cooldown", config.CooldownConfig{
		Enabled:                  true,
		DefaultDuration:          30 * time.Second,
		RateLimitDuration:        5 * time.Second,
		PaymentDuration:          5 * time.Minute,
		Error5xxDuration:         30 * time.Second,
		StructuredOutputDuration: 30 * time.Second,
		MaxRetryAfterDuration:    24 * time.Hour,
	})
	r := NewRouterWithConfig(types.AppConfig{Providers: providers}, svc, healthSvc, newProviderService())
	r.SetCooldownService(cd)
	return r
}

func TestEffectiveRateLimitCooldownSeconds_DayScaleWins(t *testing.T) {
	r := dayQuotaTestRouter(t, nil)

	// A small provider-supplied retry-after must NOT shrink a daily-quota bench.
	err := errors.NewRateLimitErrorWithSubtype("daily cap", 18, "rpd", "quota_exhausted", nil)
	err.RetryAfterProvided = true

	got := r.effectiveRateLimitCooldownSeconds(err, "kilo", "m")
	assert.Equal(t, int(dailyQuotaFallbackCooldown.Seconds()), got)

	// Non-day-scale limits keep the retry-after behavior.
	rpmErr := errors.NewRateLimitErrorWithSubtype("limited", 60, "rpm", "rate_limit", nil)
	rpmErr.RetryAfterProvided = true
	assert.Equal(t, 60, r.effectiveRateLimitCooldownSeconds(rpmErr, "kilo", "m"))
}

func TestDailyQuotaCooldownSeconds_ResetTimestampPriority(t *testing.T) {
	resetAt := time.Now().Add(90 * time.Second).UnixMilli()

	got := dailyQuotaCooldownSeconds(resetAt, "openrouter-alpha")

	// reset delta (90s) + 30s buffer
	assert.GreaterOrEqual(t, got, 118)
	assert.LessOrEqual(t, got, 122)
}

func TestDailyQuotaCooldownSeconds_GeminiMidnightPTSchedule(t *testing.T) {
	want := secondsUntilNextMidnightPT(time.Now())
	require.Greater(t, want, 0)

	got := dailyQuotaCooldownSeconds(0, "gemini")

	assert.GreaterOrEqual(t, got, want-5)
	assert.LessOrEqual(t, got, want+5)
}

func TestDailyQuotaCooldownSeconds_StaleResetFallsBackToSchedule(t *testing.T) {
	want := secondsUntilNextMidnightPT(time.Now())
	require.Greater(t, want, 0)

	stale := time.Now().Add(-time.Hour).UnixMilli()
	got := dailyQuotaCooldownSeconds(stale, "gemini")

	assert.Equal(t, want, got, "implausible reset timestamps must fall through to the Gemini midnight schedule")
}

func TestApplyRateLimitCooldown_RosterExemption(t *testing.T) {
	providers := []types.ProviderConfig{
		{
			ID:      "openrouter-alpha",
			BaseURL: "https://openrouter.ai/api/v1",
			Limits:  types.ProviderLimits{Rpd: intPtr(1000)},
			Models: types.ProviderModels{
				Mode: "allowlist",
				List: []string{"stealth/ox-alpha", "other/model"},
			},
		},
		{
			ID:      "kilo",
			BaseURL: "https://kilo.example/v1",
			Limits:  types.ProviderLimits{Rpd: intPtr(10)},
			Models: types.ProviderModels{
				Mode: "allowlist",
				List: []string{"model-a", "model-b"},
			},
		},
	}
	r := dayQuotaTestRouter(t, providers)
	ctx := testContext()

	dayCap := errors.NewRateLimitErrorWithSubtype("free-models-per-day-stealth", 60, "rpd", "rate_limit", nil)

	// openrouter-alpha: only the failed model benches despite matching limits.
	r.applyRateLimitCooldown(ctx, "openrouter-alpha", "stealth/ox-alpha", dayCap)
	assert.True(t, r.cooldownService.IsOnCooldown(ctx, "openrouter-alpha", "stealth/ox-alpha"))
	assert.False(t, r.cooldownService.IsOnCooldown(ctx, "openrouter-alpha", "other/model"),
		"exempt provider must not roster-bench siblings on day-scale caps")

	// kilo: whole-roster benching stays in force for shared-account providers.
	kiloCap := errors.NewRateLimitErrorWithSubtype("limit_rpd/x Daily limit reached", 60, "rpd", "quota_exhausted", nil)
	r.applyRateLimitCooldown(ctx, "kilo", "model-a", kiloCap)
	assert.True(t, r.cooldownService.IsOnCooldown(ctx, "kilo", "model-a"))
	assert.True(t, r.cooldownService.IsOnCooldown(ctx, "kilo", "model-b"))
}

func TestParseRateLimitDetails_DayScaleBodiesClassifiedAsRPD(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantType    string
		wantSubtype string
	}{
		{"openrouter stealth cap", `{"error":{"message":"Rate limit exceeded: free-models-per-day-stealth."}}`, "rpd", "rate_limit"},
		{"generic per-day wording", `{"error":{"message":"You have sent too many requests per day."}}`, "rpd", "rate_limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			details := parseRateLimitDetails("openrouter", http.Header{}, []byte(tc.body))
			assert.Equal(t, tc.wantType, details.LimitType)
			assert.Equal(t, tc.wantSubtype, details.LimitSubtype)
		})
	}
}
