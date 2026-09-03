package services

import (
	"testing"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func concurrencyTestRouter(t *testing.T) (*Router, *QuotaService) {
	t.Helper()
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "quota")
	healthSvc := NewHealthService(client, "health")
	cfg := types.AppConfig{Providers: []types.ProviderConfig{{
		ID:      "nous",
		BaseURL: "https://inference-api.nousresearch.com/v1",
		Limits:  types.ProviderLimits{MaxConcurrent: intPtr(3)},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{"stealth/ox-alpha"},
			Limits: map[string]types.ModelLimits{
				"stealth/ox-alpha": {MaxConcurrent: ptrInt(2)},
			},
		},
	}}}
	return NewRouterWithConfig(cfg, svc, healthSvc, newProviderService()), svc
}

func TestAcquireAttemptConcurrency_TakesBothScopes(t *testing.T) {
	r, svc := concurrencyTestRouter(t)
	ctx := testContext()

	release, deniedScope, ok := r.acquireAttemptConcurrency(ctx, "nous", "stealth/ox-alpha")
	require.True(t, ok)
	require.NotNil(t, release)
	assert.Empty(t, deniedScope)

	accountUsage, err := svc.GetConcurrencyUsage(ctx, "nous", providerQuotaScopeModel)
	require.NoError(t, err)
	assert.Equal(t, 1, accountUsage)

	modelUsage, err := svc.GetConcurrencyUsage(ctx, "nous", "stealth/ox-alpha")
	require.NoError(t, err)
	assert.Equal(t, 1, modelUsage)

	release()

	accountUsage, _ = svc.GetConcurrencyUsage(ctx, "nous", providerQuotaScopeModel)
	assert.Equal(t, 0, accountUsage, "release must free the account-wide slot")
	modelUsage, _ = svc.GetConcurrencyUsage(ctx, "nous", "stealth/ox-alpha")
	assert.Equal(t, 0, modelUsage, "release must free the per-model slot")
}

func TestAcquireAttemptConcurrency_AccountCapDeniesCheaply(t *testing.T) {
	r, svc := concurrencyTestRouter(t)
	ctx := testContext()

	for i := 0; i < 3; i++ {
		require.NoError(t, svc.AcquireConcurrencySlot(ctx, "nous", providerQuotaScopeModel, 3))
	}

	release, deniedScope, ok := r.acquireAttemptConcurrency(ctx, "nous", "stealth/ox-alpha")
	assert.False(t, ok)
	assert.Nil(t, release)
	assert.Equal(t, "provider", deniedScope)

	modelUsage, _ := svc.GetConcurrencyUsage(ctx, "nous", "stealth/ox-alpha")
	assert.Equal(t, 0, modelUsage, "no model slot may be taken when the account cap denies")
}

func TestAcquireAttemptConcurrency_ModelDenialRollsBackAccountSlot(t *testing.T) {
	r, svc := concurrencyTestRouter(t)
	ctx := testContext()

	require.NoError(t, svc.AcquireConcurrencySlot(ctx, "nous", "stealth/ox-alpha", 2))
	require.NoError(t, svc.AcquireConcurrencySlot(ctx, "nous", "stealth/ox-alpha", 2))

	release, deniedScope, ok := r.acquireAttemptConcurrency(ctx, "nous", "stealth/ox-alpha")
	assert.False(t, ok)
	assert.Nil(t, release)
	assert.Equal(t, "model", deniedScope)

	accountUsage, _ := svc.GetConcurrencyUsage(ctx, "nous", providerQuotaScopeModel)
	assert.Equal(t, 0, accountUsage, "the account slot must be released when the model cap denies")
}

func TestAcquireAttemptConcurrency_NoCapsConfigured(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "quota")
	r := NewRouterWithConfig(types.AppConfig{}, svc, NewHealthService(client, "health"), newProviderService())

	release, _, ok := r.acquireAttemptConcurrency(testContext(), "any-provider", "any-model")
	assert.True(t, ok)
	assert.Nil(t, release)
}

func TestAcquireAttemptConcurrency_ProviderWideSharedAcrossModels(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "quota")
	healthSvc := NewHealthService(client, "health")
	cfg := types.AppConfig{Providers: []types.ProviderConfig{{
		ID:      "opencode",
		BaseURL: "https://opencode.ai/zen/v1",
		Limits:  types.ProviderLimits{MaxConcurrent: intPtr(5)},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{"model-a", "model-b"},
			Limits: map[string]types.ModelLimits{
				"model-a": {},
				"model-b": {},
			},
		},
	}}}
	r := NewRouterWithConfig(cfg, svc, healthSvc, newProviderService())
	ctx := testContext()

	// Acquire 3 on model-a
	var releases []func()
	for i := 0; i < 3; i++ {
		rel, scope, ok := r.acquireAttemptConcurrency(ctx, "opencode", "model-a")
		require.True(t, ok)
		require.Empty(t, scope)
		releases = append(releases, rel)
	}

	// Acquire 2 on model-b
	for i := 0; i < 2; i++ {
		rel, scope, ok := r.acquireAttemptConcurrency(ctx, "opencode", "model-b")
		require.True(t, ok)
		require.Empty(t, scope)
		releases = append(releases, rel)
	}

	// 6th attempt on model-a must fail with provider scope
	rel, scope, ok := r.acquireAttemptConcurrency(ctx, "opencode", "model-a")
	assert.False(t, ok)
	assert.Nil(t, rel)
	assert.Equal(t, "provider", scope)

	// Release 1 and try again
	releases[0]()
	rel, scope, ok = r.acquireAttemptConcurrency(ctx, "opencode", "model-a")
	assert.True(t, ok)
	assert.Empty(t, scope)
	rel()

	for _, release := range releases[1:] {
		release()
	}
}

