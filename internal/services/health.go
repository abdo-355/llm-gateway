package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/redis/go-redis/v9"
)

type CircuitState string

const (
	StateClosed   CircuitState = "CLOSED"
	StateOpen     CircuitState = "OPEN"
	StateHalfOpen CircuitState = "HALF_OPEN"
)

type HealthMetrics struct {
	ProviderID      string
	Model           string
	CircuitState    CircuitState
	FailureCount    int
	SuccessCount    int
	LastFailureTime *int64
	AverageLatency  *int
	HealthScore     float64
}

type HealthService struct {
	redis            *redis.Client
	failureThreshold int
	recoveryTimeout  time.Duration
}

func NewHealthService(redis *redis.Client) *HealthService {
	return &HealthService{
		redis:            redis,
		failureThreshold: 5,
		recoveryTimeout:  30 * time.Second,
	}
}

func (s *HealthService) keyPrefix(providerID, model string) string {
	return fmt.Sprintf("%s:%s", providerID, model)
}

func (s *HealthService) GetCircuitState(ctx context.Context, providerID, model string) CircuitState {
	prefix := s.keyPrefix(providerID, model)
	stateKey := fmt.Sprintf("circuit:%s:state", prefix)

	state, err := s.redis.Get(ctx, stateKey).Result()
	if err == redis.Nil {
		return StateClosed
	}
	if err != nil {
		logger.Error().
			Str("type", "app").
			Str("event", "health.circuit_state_failed").
			Err(err).
			Msg("Failed to get circuit state")
		return StateClosed
	}

	return CircuitState(state)
}

func (s *HealthService) CanExecute(ctx context.Context, providerID, model string) bool {
	state := s.GetCircuitState(ctx, providerID, model)
	prefix := s.keyPrefix(providerID, model)

	switch state {
	case StateClosed:
		return true

	case StateOpen:
		lastFailureKey := fmt.Sprintf("circuit:%s:last_failure", prefix)

		lastFailure, err := s.redis.Get(ctx, lastFailureKey).Int64()
		if err != nil {
			return false
		}

		if time.Since(time.UnixMilli(lastFailure)) >= s.recoveryTimeout {
			s.setCircuitState(ctx, providerID, model, StateHalfOpen)
			s.redis.Set(ctx, fmt.Sprintf("circuit:%s:failures", prefix), 0, 0)
			s.redis.Set(ctx, fmt.Sprintf("circuit:%s:successes", prefix), 0, 0)
			return true
		}
		return false

	case StateHalfOpen:
		successes, _ := s.redis.Get(ctx, fmt.Sprintf("circuit:%s:successes", prefix)).Int()
		failures, _ := s.redis.Get(ctx, fmt.Sprintf("circuit:%s:failures", prefix)).Int()
		return successes+failures < 1
	}

	return false
}

func (s *HealthService) CheckCircuitBreaker(ctx context.Context, providerID, model string) error {
	if !s.CanExecute(ctx, providerID, model) {
		state := s.GetCircuitState(ctx, providerID, model)
		return errors.NewCircuitBreakerError(
			fmt.Sprintf("Circuit breaker is %s for provider %s model %s", state, providerID, model),
			providerID, string(state),
		)
	}
	return nil
}

func (s *HealthService) RecordSuccess(ctx context.Context, providerID, model string, latencyMs int) {
	state := s.GetCircuitState(ctx, providerID, model)
	prefix := s.keyPrefix(providerID, model)

	if state == StateHalfOpen {
		successesKey := fmt.Sprintf("circuit:%s:successes", prefix)
		successes, _ := s.redis.Incr(ctx, successesKey).Result()
		s.redis.Expire(ctx, successesKey, 24*time.Hour)

		if successes >= 1 {
			s.setCircuitState(ctx, providerID, model, StateClosed)
			s.redis.Del(ctx, fmt.Sprintf("circuit:%s:failures", prefix))
			s.redis.Del(ctx, successesKey)
		}
	} else if state == StateClosed {
		s.redis.Del(ctx, fmt.Sprintf("circuit:%s:failures", prefix))
	}

	latencyKey := fmt.Sprintf("health:%s:latencies", prefix)
	timestamp := float64(time.Now().UnixMilli())
	s.redis.ZAdd(ctx, latencyKey, redis.Z{Score: timestamp, Member: latencyMs})
	cutoff := float64(time.Now().Add(-time.Hour).UnixMilli())
	s.redis.ZRemRangeByScore(ctx, latencyKey, "0", fmt.Sprintf("%f", cutoff))
	s.redis.Expire(ctx, latencyKey, time.Hour)

	s.updateHealthScore(ctx, providerID, model)
}

func (s *HealthService) RecordFailure(ctx context.Context, providerID, model string) {
	state := s.GetCircuitState(ctx, providerID, model)
	prefix := s.keyPrefix(providerID, model)

	failuresKey := fmt.Sprintf("circuit:%s:failures", prefix)
	lastFailureKey := fmt.Sprintf("circuit:%s:last_failure", prefix)

	failures, _ := s.redis.Incr(ctx, failuresKey).Result()
	s.redis.Expire(ctx, failuresKey, 24*time.Hour)
	s.redis.Set(ctx, lastFailureKey, time.Now().UnixMilli(), 24*time.Hour)

	if state == StateHalfOpen {
		s.setCircuitState(ctx, providerID, model, StateOpen)
	} else if state == StateClosed && failures >= int64(s.failureThreshold) {
		s.setCircuitState(ctx, providerID, model, StateOpen)
	}

	s.updateHealthScore(ctx, providerID, model)
}

func (s *HealthService) GetHealthMetrics(ctx context.Context, providerID, model string) HealthMetrics {
	circuitState := s.GetCircuitState(ctx, providerID, model)
	prefix := s.keyPrefix(providerID, model)

	failuresKey := fmt.Sprintf("circuit:%s:failures", prefix)
	successesKey := fmt.Sprintf("circuit:%s:successes", prefix)
	lastFailureKey := fmt.Sprintf("circuit:%s:last_failure", prefix)
	latencyKey := fmt.Sprintf("health:%s:latencies", prefix)
	scoreKey := fmt.Sprintf("health:%s:score", prefix)

	pipe := s.redis.Pipeline()
	failuresCmd := pipe.Get(ctx, failuresKey)
	successesCmd := pipe.Get(ctx, successesKey)
	lastFailureCmd := pipe.Get(ctx, lastFailureKey)
	scoreCmd := pipe.Get(ctx, scoreKey)
	latencyCmd := pipe.ZRange(ctx, latencyKey, 0, -1)

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		logger.Error().
			Str("type", "db").
			Str("event", "health.metrics_failed").
			Err(err).
			Msg("Failed to get health metrics")
	}

	failureCount, _ := strconv.Atoi(failuresCmd.Val())
	successCount, _ := strconv.Atoi(successesCmd.Val())

	var lastFailureTime *int64
	if lastFailureCmd.Val() != "" {
		if ts, err := strconv.ParseInt(lastFailureCmd.Val(), 10, 64); err == nil {
			lastFailureTime = &ts
		}
	}

	var avgLatency *int
	latencies := latencyCmd.Val()
	if len(latencies) > 0 {
		sum := 0
		for _, latStr := range latencies {
			if lat, err := strconv.Atoi(latStr); err == nil {
				sum += lat
			}
		}
		avg := sum / len(latencies)
		avgLatency = &avg
	}

	healthScore := 1.0
	if scoreCmd.Val() != "" {
		if score, err := strconv.ParseFloat(scoreCmd.Val(), 64); err == nil {
			healthScore = score
		}
	}

	return HealthMetrics{
		ProviderID:      providerID,
		Model:           model,
		CircuitState:    circuitState,
		FailureCount:    failureCount,
		SuccessCount:    successCount,
		LastFailureTime: lastFailureTime,
		AverageLatency:  avgLatency,
		HealthScore:     healthScore,
	}
}

func (s *HealthService) GetAllHealthMetrics(ctx context.Context) []HealthMetrics {
	providers := config.GetProviders()

	metrics := make([]HealthMetrics, 0)
	for _, provider := range providers {
		for _, model := range provider.Models.List {
			metrics = append(metrics, s.GetHealthMetrics(ctx, provider.ID, model))
		}
	}

	return metrics
}

func (s *HealthService) setCircuitState(ctx context.Context, providerID, model string, state CircuitState) {
	prefix := s.keyPrefix(providerID, model)
	stateKey := fmt.Sprintf("circuit:%s:state", prefix)
	s.redis.Set(ctx, stateKey, string(state), 24*time.Hour)
}

func (s *HealthService) updateHealthScore(ctx context.Context, providerID, model string) {
	prefix := s.keyPrefix(providerID, model)
	failuresKey := fmt.Sprintf("circuit:%s:failures", prefix)
	failures, _ := s.redis.Get(ctx, failuresKey).Int()
	state := s.GetCircuitState(ctx, providerID, model)

	var score float64 = 1.0

	switch state {
	case StateOpen:
		score = 0.0
	case StateHalfOpen:
		score = 0.5
	default:
		if failures > 3 {
			score = 0.5
		} else if failures > 0 {
			score = 1.0 - float64(failures)*0.1
		}
	}

	scoreKey := fmt.Sprintf("health:%s:score", prefix)
	s.redis.Set(ctx, scoreKey, score, time.Hour)
}
