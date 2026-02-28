package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/redis/go-redis/v9"
)

type CircuitState string

const (
	StateClosed   CircuitState = "CLOSED"
	StateOpen     CircuitState = "OPEN"
	StateHalfOpen CircuitState = "HALF_OPEN"
)

type HealthMetrics struct {
	ProviderID      string       `json:"provider_id"`
	Model           string       `json:"model"`
	CircuitState    CircuitState `json:"circuit_state"`
	FailureCount    int          `json:"failure_count"`
	SuccessCount    int          `json:"success_count"`
	LastFailureTime *int64       `json:"last_failure_time,omitempty"`
	AverageLatency  *int         `json:"average_latency_ms,omitempty"`
	HealthScore     float64      `json:"health_score"`
}

type HealthService struct {
	redis            *redis.Client
	prefix           string
	failureThreshold int
	recoveryTimeout  time.Duration
}

func NewHealthService(redisClient *redis.Client, keyPrefix string) *HealthService {
	prefix := keyPrefix
	if prefix == "" {
		prefix = "health"
	}
	return &HealthService{
		redis:            redisClient,
		prefix:           prefix,
		failureThreshold: 5,
		recoveryTimeout:  30 * time.Second,
	}
}

func (s *HealthService) buildCircuitKeyPrefix(providerID, model string) string {
	return fmt.Sprintf("circuit:%s:%s:%s", s.prefix, providerID, model)
}

func (s *HealthService) buildHealthKeyPrefix(providerID, model string) string {
	return fmt.Sprintf("health:%s:%s:%s", s.prefix, providerID, model)
}

func (s *HealthService) GetCircuitState(ctx context.Context, providerID, model string) CircuitState {
	circuitPrefix := s.buildCircuitKeyPrefix(providerID, model)
	stateKey := fmt.Sprintf("%s:state", circuitPrefix)

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
	circuitPrefix := s.buildCircuitKeyPrefix(providerID, model)

	switch state {
	case StateClosed:
		return true

	case StateOpen:
		lastFailureKey := fmt.Sprintf("%s:last_failure", circuitPrefix)

		lastFailure, err := s.redis.Get(ctx, lastFailureKey).Int64()
		if err != nil {
			return false
		}

		if time.Since(time.UnixMilli(lastFailure)) >= s.recoveryTimeout {
			s.setCircuitState(ctx, providerID, model, StateHalfOpen)
			s.redis.Set(ctx, fmt.Sprintf("%s:failures", circuitPrefix), 0, 0)
			s.redis.Set(ctx, fmt.Sprintf("%s:successes", circuitPrefix), 0, 0)
			return true
		}
		return false

	case StateHalfOpen:
		// Allow up to 2 probe requests in HALF_OPEN before requiring a decision
		successes, _ := s.redis.Get(ctx, fmt.Sprintf("%s:successes", circuitPrefix)).Int()
		failures, _ := s.redis.Get(ctx, fmt.Sprintf("%s:failures", circuitPrefix)).Int()
		return successes+failures < 2
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
	circuitPrefix := s.buildCircuitKeyPrefix(providerID, model)
	healthPrefix := s.buildHealthKeyPrefix(providerID, model)

	if state == StateHalfOpen {
		successesKey := fmt.Sprintf("%s:successes", circuitPrefix)
		successes, _ := s.redis.Incr(ctx, successesKey).Result()
		s.redis.Expire(ctx, successesKey, 24*time.Hour)

		if successes >= 2 {
			s.setCircuitState(ctx, providerID, model, StateClosed)
			s.redis.Del(ctx, fmt.Sprintf("%s:failures", circuitPrefix))
			s.redis.Del(ctx, successesKey)
		}
	} else if state == StateClosed {
		// Gradually decrement failure count instead of clearing entirely
		failuresKey := fmt.Sprintf("%s:failures", circuitPrefix)
		failures, _ := s.redis.Get(ctx, failuresKey).Int()
		if failures > 0 {
			s.redis.Decr(ctx, failuresKey)
		}
	}

	// Issue #8: Use unique member ID with latency as score to prevent deduplication
	latencyKey := fmt.Sprintf("%s:latencies", healthPrefix)
	now := time.Now()
	member := fmt.Sprintf("%d-%d", now.UnixMilli(), now.Nanosecond())
	s.redis.ZAdd(ctx, latencyKey, redis.Z{Score: float64(latencyMs), Member: member})
	// Use Redis key expiration instead of ZRemRangeByScore since we store latency as score
	s.redis.Expire(ctx, latencyKey, time.Hour)

	s.updateHealthScore(ctx, providerID, model)
}

func (s *HealthService) RecordFailure(ctx context.Context, providerID, model string) {
	state := s.GetCircuitState(ctx, providerID, model)
	circuitPrefix := s.buildCircuitKeyPrefix(providerID, model)

	failuresKey := fmt.Sprintf("%s:failures", circuitPrefix)
	lastFailureKey := fmt.Sprintf("%s:last_failure", circuitPrefix)

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
	circuitPrefix := s.buildCircuitKeyPrefix(providerID, model)
	healthPrefix := s.buildHealthKeyPrefix(providerID, model)

	failuresKey := fmt.Sprintf("%s:failures", circuitPrefix)
	successesKey := fmt.Sprintf("%s:successes", circuitPrefix)
	lastFailureKey := fmt.Sprintf("%s:last_failure", circuitPrefix)
	latencyKey := fmt.Sprintf("%s:latencies", healthPrefix)
	scoreKey := fmt.Sprintf("%s:score", healthPrefix)

	pipe := s.redis.Pipeline()
	failuresCmd := pipe.Get(ctx, failuresKey)
	successesCmd := pipe.Get(ctx, successesKey)
	lastFailureCmd := pipe.Get(ctx, lastFailureKey)
	scoreCmd := pipe.Get(ctx, scoreKey)
	// Issue #8: Use ZRangeWithScores to get latency values from scores
	latencyCmd := pipe.ZRangeWithScores(ctx, latencyKey, 0, -1)

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
	latencyScores := latencyCmd.Val()
	if len(latencyScores) > 0 {
		sum := 0
		for _, z := range latencyScores {
			sum += int(z.Score)
		}
		avg := sum / len(latencyScores)
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

func (s *HealthService) setCircuitState(ctx context.Context, providerID, model string, newState CircuitState) {
	circuitPrefix := s.buildCircuitKeyPrefix(providerID, model)
	stateKey := fmt.Sprintf("%s:state", circuitPrefix)

	oldState := s.GetCircuitState(ctx, providerID, model)

	s.redis.Set(ctx, stateKey, string(newState), 24*time.Hour)

	metrics.CircuitBreakerState.WithLabelValues(providerID, model).Set(
		metrics.CircuitStateToFloat64(string(newState)),
	)

	if oldState != newState {
		metrics.CircuitBreakerTransitionsTotal.WithLabelValues(
			providerID, model, string(oldState), string(newState),
		).Inc()
	}
}

func (s *HealthService) updateHealthScore(ctx context.Context, providerID, model string) {
	circuitPrefix := s.buildCircuitKeyPrefix(providerID, model)
	healthPrefix := s.buildHealthKeyPrefix(providerID, model)
	failuresKey := fmt.Sprintf("%s:failures", circuitPrefix)
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

	scoreKey := fmt.Sprintf("%s:score", healthPrefix)
	s.redis.Set(ctx, scoreKey, score, time.Hour)
}
