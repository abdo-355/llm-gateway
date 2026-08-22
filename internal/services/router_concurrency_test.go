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
