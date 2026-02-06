package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
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
		svc := NewHealthService(client)
		ctx := testContext()

		state := svc.GetCircuitState(ctx, testProvider, testModel)
		assert.Equal(t, StateClosed, state)
	})

	t.Run("returns OPEN after setting state to OPEN", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateOpen)

		state := svc.GetCircuitState(ctx, testProvider, testModel)
		assert.Equal(t, StateOpen, state)
	})
}

func TestHealthCanExecute(t *testing.T) {
	t.Run("CLOSED state returns true", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		assert.True(t, svc.CanExecute(ctx, testProvider, testModel))
	})

	t.Run("OPEN state not recovered returns false", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateOpen)
		prefix := svc.keyPrefix(testProvider, testModel)
		client.Set(ctx, fmt.Sprintf("circuit:%s:last_failure", prefix), time.Now().UnixMilli(), 0)

		assert.False(t, svc.CanExecute(ctx, testProvider, testModel))
	})

	t.Run("OPEN state with recovery timeout passed transitions to HALF_OPEN and returns true", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateOpen)
		prefix := svc.keyPrefix(testProvider, testModel)
		pastTime := time.Now().Add(-31 * time.Second).UnixMilli()
		client.Set(ctx, fmt.Sprintf("circuit:%s:last_failure", prefix), pastTime, 0)

		assert.True(t, svc.CanExecute(ctx, testProvider, testModel))
		assert.Equal(t, StateHalfOpen, svc.GetCircuitState(ctx, testProvider, testModel))
	})

	t.Run("HALF_OPEN state with no attempts returns true", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateHalfOpen)
		prefix := svc.keyPrefix(testProvider, testModel)
		client.Set(ctx, fmt.Sprintf("circuit:%s:successes", prefix), 0, 0)
		client.Set(ctx, fmt.Sprintf("circuit:%s:failures", prefix), 0, 0)

		assert.True(t, svc.CanExecute(ctx, testProvider, testModel))
	})

	t.Run("HALF_OPEN state with already attempted returns false", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateHalfOpen)
		prefix := svc.keyPrefix(testProvider, testModel)
		client.Set(ctx, fmt.Sprintf("circuit:%s:successes", prefix), 1, 0)
		client.Set(ctx, fmt.Sprintf("circuit:%s:failures", prefix), 0, 0)

		assert.False(t, svc.CanExecute(ctx, testProvider, testModel))
	})
}

func TestHealthRecordSuccess(t *testing.T) {
	t.Run("in HALF_OPEN closes circuit", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateHalfOpen)

		svc.RecordSuccess(ctx, testProvider, testModel, 100)

		assert.Equal(t, StateClosed, svc.GetCircuitState(ctx, testProvider, testModel))
	})

	t.Run("in CLOSED clears failure count", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		prefix := svc.keyPrefix(testProvider, testModel)
		client.Set(ctx, fmt.Sprintf("circuit:%s:failures", prefix), 3, 0)

		svc.RecordSuccess(ctx, testProvider, testModel, 50)

		val, err := client.Get(ctx, fmt.Sprintf("circuit:%s:failures", prefix)).Result()
		assert.Error(t, err)
		assert.Empty(t, val)
	})

	t.Run("records latency", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		svc.RecordSuccess(ctx, testProvider, testModel, 150)

		prefix := svc.keyPrefix(testProvider, testModel)
		latencyKey := fmt.Sprintf("health:%s:latencies", prefix)
		members, err := client.ZRange(ctx, latencyKey, 0, -1).Result()
		require.NoError(t, err)
		assert.Len(t, members, 1)
		assert.Equal(t, "150", members[0])
	})
}

func TestHealthRecordFailure(t *testing.T) {
	t.Run("in CLOSED below threshold stays CLOSED", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		for i := 0; i < 4; i++ {
			svc.RecordFailure(ctx, testProvider, testModel)
		}

		assert.Equal(t, StateClosed, svc.GetCircuitState(ctx, testProvider, testModel))
	})

	t.Run("in CLOSED reaches threshold opens circuit", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		for i := 0; i < 5; i++ {
			svc.RecordFailure(ctx, testProvider, testModel)
		}

		assert.Equal(t, StateOpen, svc.GetCircuitState(ctx, testProvider, testModel))
	})

	t.Run("in HALF_OPEN opens circuit", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		svc.setCircuitState(ctx, testProvider, testModel, StateHalfOpen)

		svc.RecordFailure(ctx, testProvider, testModel)

		assert.Equal(t, StateOpen, svc.GetCircuitState(ctx, testProvider, testModel))
	})
}

func TestHealthGetHealthMetrics(t *testing.T) {
	t.Run("returns correct metrics after recording successes and failures", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
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
}

func TestHealthCheckCircuitBreaker(t *testing.T) {
	t.Run("closed circuit returns nil error", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
		ctx := testContext()

		err := svc.CheckCircuitBreaker(ctx, testProvider, testModel)
		assert.NoError(t, err)
	})

	t.Run("open circuit returns CircuitBreakerError", func(t *testing.T) {
		client, _ := newTestRedis(t)
		svc := NewHealthService(client)
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
