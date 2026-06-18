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
