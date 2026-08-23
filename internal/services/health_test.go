package services

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testProvider = "test-provider"
	testModel    = "test-model"
)

func TestHealthGetCircuitState(t *testing.T) {
	t.Run("default state is CLOSED when no key exists", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		state := svc.GetCircuitState(ctx, testProvider, testModel)
		assert.Equal(t, StateClosed, state)
	})

	t.Run("returns OPEN after setting state to OPEN", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateOpen)

		state := svc.GetCircuitState(ctx, testProvider, testModel)
		assert.Equal(t, StateOpen, state)
	})
}

func TestHealthCanExecute(t *testing.T) {
	t.Run("CLOSED state returns true", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		assert.True(t, svc.CanExecute(ctx, testProvider, testModel))
	})

	t.Run("OPEN state not recovered returns false", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateOpen)
		prefix := svc.buildCircuitKeyPrefix(testProvider, testModel)
		client.Set(ctx, fmt.Sprintf("%s:last_failure", prefix), time.Now().UnixMilli(), 0)

		assert.False(t, svc.CanExecute(ctx, testProvider, testModel))
	})

	t.Run("OPEN state with recovery timeout passed transitions to HALF_OPEN and returns true", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateOpen)
		prefix := svc.buildCircuitKeyPrefix(testProvider, testModel)
		pastTime := time.Now().Add(-31 * time.Second).UnixMilli()
		client.Set(ctx, fmt.Sprintf("%s:last_failure", prefix), pastTime, 0)

		assert.True(t, svc.CanExecute(ctx, testProvider, testModel))
		assert.Equal(t, StateHalfOpen, svc.GetCircuitState(ctx, testProvider, testModel))
	})

	t.Run("HALF_OPEN state with no attempts returns true", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateHalfOpen)
		prefix := svc.buildCircuitKeyPrefix(testProvider, testModel)
		client.Set(ctx, fmt.Sprintf("%s:successes", prefix), 0, 0)
		client.Set(ctx, fmt.Sprintf("%s:failures", prefix), 0, 0)

		assert.True(t, svc.CanExecute(ctx, testProvider, testModel))
	})

	t.Run("HALF_OPEN state with already attempted returns false", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateHalfOpen)
		prefix := svc.buildCircuitKeyPrefix(testProvider, testModel)
		client.Set(ctx, fmt.Sprintf("%s:successes", prefix), 1, 0)
		client.Set(ctx, fmt.Sprintf("%s:failures", prefix), 1, 0)

		assert.False(t, svc.CanExecute(ctx, testProvider, testModel))
	})
}

func TestHealthRecordSuccess(t *testing.T) {
	t.Run("in HALF_OPEN closes circuit after 2 successes", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateHalfOpen)

		svc.RecordSuccess(ctx, testProvider, testModel, 100)
		assert.Equal(t, StateHalfOpen, svc.GetCircuitState(ctx, testProvider, testModel))

		svc.RecordSuccess(ctx, testProvider, testModel, 100)
		assert.Equal(t, StateClosed, svc.GetCircuitState(ctx, testProvider, testModel))
	})

	t.Run("in CLOSED decrements failure count", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		prefix := svc.buildCircuitKeyPrefix(testProvider, testModel)
		client.Set(ctx, fmt.Sprintf("%s:failures", prefix), 3, 0)

		svc.RecordSuccess(ctx, testProvider, testModel, 50)

		val, err := client.Get(ctx, fmt.Sprintf("%s:failures", prefix)).Result()
		require.NoError(t, err)
		assert.Equal(t, "2", val) // Decremented from 3 to 2, not cleared entirely
	})

	t.Run("records latency", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		svc.RecordSuccess(ctx, testProvider, testModel, 150)

		prefix := svc.buildHealthKeyPrefix(testProvider, testModel)
		latencyKey := fmt.Sprintf("%s:%s", prefix, latencyKeySuffix)
		latencies, err := client.LRange(ctx, latencyKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{"150"}, latencies)
	})

	t.Run("caps latency history at the 100 most recent samples", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		for latency := 1; latency <= 150; latency++ {
			svc.RecordSuccess(ctx, testProvider, testModel, latency)
		}

		prefix := svc.buildHealthKeyPrefix(testProvider, testModel)
		latencyKey := fmt.Sprintf("%s:%s", prefix, latencyKeySuffix)
		latencies, err := client.LRange(ctx, latencyKey, 0, -1).Result()
		require.NoError(t, err)
		require.Len(t, latencies, latencyHistoryLimit)
		assert.Equal(t, "150", latencies[0])
		assert.Equal(t, "51", latencies[latencyHistoryLimit-1])

		metrics := svc.GetHealthMetrics(ctx, testProvider, testModel)
		require.NotNil(t, metrics.AverageLatency)
		assert.Equal(t, 100, *metrics.AverageLatency)
	})
}

func TestHealthRecordFailure(t *testing.T) {
	t.Run("in CLOSED below threshold stays CLOSED", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		for i := 0; i < 2; i++ {
			svc.RecordFailure(ctx, testProvider, testModel)
		}

		assert.Equal(t, StateClosed, svc.GetCircuitState(ctx, testProvider, testModel))
	})

	t.Run("in CLOSED reaches threshold opens circuit", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		for i := 0; i < 3; i++ {
			svc.RecordFailure(ctx, testProvider, testModel)
		}

		assert.Equal(t, StateOpen, svc.GetCircuitState(ctx, testProvider, testModel))
	})

	t.Run("in HALF_OPEN opens circuit", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateHalfOpen)

		svc.RecordFailure(ctx, testProvider, testModel)

		assert.Equal(t, StateOpen, svc.GetCircuitState(ctx, testProvider, testModel))
	})
}

func TestHealthGetHealthMetrics(t *testing.T) {
	t.Run("returns correct metrics after recording successes and failures", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		svc.RecordFailure(ctx, testProvider, testModel)
		svc.RecordFailure(ctx, testProvider, testModel)
		svc.RecordSuccess(ctx, testProvider, testModel, 100)
		svc.RecordSuccess(ctx, testProvider, testModel, 200)

		metrics := svc.GetHealthMetrics(ctx, testProvider, testModel)

		assert.Equal(t, testProvider, metrics.ProviderID)
		assert.Equal(t, testModel, metrics.Model)
		assert.Equal(t, StateClosed, metrics.CircuitState)
		assert.Equal(t, 0, metrics.FailureCount)
		require.NotNil(t, metrics.AverageLatency)
		assert.Equal(t, 150, *metrics.AverageLatency)
	})

	t.Run("uses the versioned list alongside the legacy ZSET and preserves expiry", func(t *testing.T) {
		client, mr := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()
		prefix := svc.buildHealthKeyPrefix(testProvider, testModel)
		legacyKey := fmt.Sprintf("%s:latencies", prefix)
		latencyKey := fmt.Sprintf("%s:%s", prefix, latencyKeySuffix)

		require.NoError(t, client.ZAdd(ctx, legacyKey, redis.Z{Score: 999, Member: "legacy"}).Err())
		svc.RecordSuccess(ctx, testProvider, testModel, 250)

		legacyType, err := client.Type(ctx, legacyKey).Result()
		require.NoError(t, err)
		assert.Equal(t, "zset", legacyType)
		latencyType, err := client.Type(ctx, latencyKey).Result()
		require.NoError(t, err)
		assert.Equal(t, "list", latencyType)

		ttl, err := client.TTL(ctx, latencyKey).Result()
		require.NoError(t, err)
		assert.Equal(t, time.Hour, ttl)

		metrics := svc.GetHealthMetrics(ctx, testProvider, testModel)
		require.NotNil(t, metrics.AverageLatency)
		assert.Equal(t, 250, *metrics.AverageLatency)

		mr.FastForward(time.Hour)
		assert.Zero(t, client.Exists(ctx, latencyKey).Val())
		assert.Nil(t, svc.GetHealthMetrics(ctx, testProvider, testModel).AverageLatency)
	})
}

func TestHealthBatchGetHealthMetricsUsesBoundedLatencyHistory(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewHealthService(client, "")
	ctx := context.Background()
	pairs := []ProviderModelPair{
		{ProviderID: "provider-a", Model: "model-a"},
		{ProviderID: "provider-b", Model: "model-b"},
	}

	firstKey := fmt.Sprintf("%s:%s", svc.buildHealthKeyPrefix("provider-a", "model-a"), latencyKeySuffix)
	firstSamples := make([]any, 150)
	for i := range firstSamples {
		firstSamples[i] = strconv.Itoa(i + 1)
	}
	require.NoError(t, client.RPush(ctx, firstKey, firstSamples...).Err())

	svc.RecordSuccess(ctx, "provider-b", "model-b", 200)
	svc.RecordSuccess(ctx, "provider-b", "model-b", 400)

	metrics := svc.BatchGetHealthMetrics(ctx, pairs)
	require.Len(t, metrics, 2)
	require.NotNil(t, metrics[0].AverageLatency)
	assert.Equal(t, 50, *metrics[0].AverageLatency)
	require.NotNil(t, metrics[1].AverageLatency)
	assert.Equal(t, 300, *metrics[1].AverageLatency)
}

func TestHealthCheckCircuitBreaker(t *testing.T) {
	t.Run("closed circuit returns nil error", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		err := svc.CheckCircuitBreaker(ctx, testProvider, testModel)
		assert.NoError(t, err)
	})

	t.Run("open circuit returns CircuitBreakerError", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client, "")
		ctx := testContext()

		for i := 0; i < 5; i++ {
			svc.RecordFailure(ctx, testProvider, testModel)
		}

		err := svc.CheckCircuitBreaker(ctx, testProvider, testModel)
		require.Error(t, err)

		var cbErr *errors.CircuitBreakerError
		assert.ErrorAs(t, err, &cbErr)
		assert.Equal(t, testProvider, cbErr.ProviderID)
		assert.Equal(t, string(StateOpen), cbErr.State)
	})
}
