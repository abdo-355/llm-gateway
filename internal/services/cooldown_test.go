package services

import (
	"context"
	"testing"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCooldownBatchIsOnCooldownUsesLua(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	svc := NewCooldownService(client, "cooldown", config.CooldownConfig{Enabled: true, DefaultDuration: time.Minute})
	ctx := context.Background()
	svc.ApplyCooldown(ctx, "provider-a", "model-1", time.Minute, CooldownRateLimit)

	got := svc.BatchIsOnCooldown(ctx, []ProviderModelPair{
		{ProviderID: "provider-a", Model: "model-1"},
		{ProviderID: "provider-a", Model: "model-2"},
	})

	assert.True(t, got["provider-a/model-1"])
	assert.False(t, got["provider-a/model-2"])
}

func TestCooldownStructuredOutputDuration(t *testing.T) {
	svc := NewCooldownService(nil, "cooldown", config.CooldownConfig{
		Enabled:                  true,
		DefaultDuration:          30 * time.Second,
		StructuredOutputDuration: 10 * time.Minute,
	})

	assert.Equal(t, 10*time.Minute, svc.GetDurationForReason(CooldownStructuredOutput))
}

func TestCooldownRetryAfterHonorsLongQuotaWindow(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	svc := NewCooldownService(client, "cooldown", config.CooldownConfig{
		Enabled:               true,
		RateLimitDuration:     5 * time.Second,
		MaxRetryAfterDuration: 24 * time.Hour,
	})
	ctx := context.Background()

	svc.ApplyCooldownForReason(ctx, "provider-a", "model-1", CooldownQuota, 86400)

	remaining := svc.GetCooldownRemaining(ctx, "provider-a", "model-1")
	assert.GreaterOrEqual(t, remaining, 23*time.Hour)
	assert.LessOrEqual(t, remaining, 24*time.Hour)
}

func TestCooldownRetryAfterIsCapped(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	svc := NewCooldownService(client, "cooldown", config.CooldownConfig{
		Enabled:               true,
		RateLimitDuration:     5 * time.Second,
		MaxRetryAfterDuration: time.Hour,
	})
	ctx := context.Background()

	svc.ApplyCooldownForReason(ctx, "provider-a", "model-1", CooldownQuota, 86400)

	remaining := svc.GetCooldownRemaining(ctx, "provider-a", "model-1")
	assert.GreaterOrEqual(t, remaining, 59*time.Minute)
	assert.LessOrEqual(t, remaining, time.Hour)
}
