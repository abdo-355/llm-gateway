// Package services provides core business logic.
package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/lib"
	"github.com/redis/go-redis/v9"
)

// CircuitState represents the state of a circuit breaker
type CircuitState string

const (
	StateClosed   CircuitState = "CLOSED"
	StateOpen     CircuitState = "OPEN"
	StateHalfOpen CircuitState = "HALF_OPEN"
)

// HealthMetrics represents health metrics for a provider
type HealthMetrics struct {
	ProviderID      string
	CircuitState    CircuitState
	FailureCount    int
	SuccessCount    int
	LastFailureTime *int64
	AverageLatency  *int
	HealthScore     float64
}

// HealthService handles provider health monitoring with circuit breaker
type HealthService struct {
	redis            *redis.Client
	failureThreshold int
	recoveryTimeout  time.Duration
}

// NewHealthService creates a new health service
func NewHealthService() *HealthService {
	return &HealthService{
		redis:            lib.GetRedisClient(),
		failureThreshold: 5,
		recoveryTimeout:  30 * time.Second,
	}
}

// GetCircuitState returns the current circuit state
func (s *HealthService) GetCircuitState(providerID string) CircuitState {
	ctx := context.Background()
	stateKey := fmt.Sprintf("circuit:%s:state", providerID)

	state, err := s.redis.Get(ctx, stateKey).Result()
	if err == redis.Nil {
		return StateClosed
	}
	if err != nil {
		lib.GetLogger().Error("Failed to get circuit state", "error", err)
		return StateClosed
	}

	return CircuitState(state)
}

// CanExecute checks if a provider can accept requests
func (s *HealthService) CanExecute(providerID string) bool {
	state := s.GetCircuitState(providerID)

	switch state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if recovery timeout has passed
		ctx := context.Background()
		lastFailureKey := fmt.Sprintf("circuit:%s:last_failure", providerID)

		lastFailure, err := s.redis.Get(ctx, lastFailureKey).Int64()
		if err != nil {
			return false
		}

		if time.Since(time.UnixMilli(lastFailure)) >= s.recoveryTimeout {
			// Transition to HALF_OPEN
			s.setCircuitState(providerID, StateHalfOpen)
			s.redis.Set(ctx, fmt.Sprintf("circuit:%s:failures", providerID), 0, 0)
			s.redis.Set(ctx, fmt.Sprintf("circuit:%s:successes", providerID), 0, 0)
			return true
		}
		return false

	case StateHalfOpen:
		// Allow only 1 probe request
		ctx := context.Background()
		successes, _ := s.redis.Get(ctx, fmt.Sprintf("circuit:%s:successes", providerID)).Int()
		failures, _ := s.redis.Get(ctx, fmt.Sprintf("circuit:%s:failures", providerID)).Int()
		return successes+failures < 1
	}

	return false
}

// CheckCircuitBreaker throws error if circuit is open
func (s *HealthService) CheckCircuitBreaker(providerID string) error {
	state := s.GetCircuitState(providerID)

	if state == StateOpen {
		return errors.NewCircuitBreakerError(
			fmt.Sprintf("Circuit breaker is OPEN for provider %s", providerID),
			providerID, string(state),
		)
	}

	if state == StateHalfOpen {
		ctx := context.Background()
		successes, _ := s.redis.Get(ctx, fmt.Sprintf("circuit:%s:successes", providerID)).Int()
		failures, _ := s.redis.Get(ctx, fmt.Sprintf("circuit:%s:failures", providerID)).Int()

		if successes+failures >= 1 {
			return errors.NewCircuitBreakerError(
				fmt.Sprintf("Circuit breaker is HALF_OPEN for provider %s, probe limit reached", providerID),
				providerID, string(state),
			)
		}
	}

	return nil
}

// RecordSuccess records a successful request
func (s *HealthService) RecordSuccess(providerID string, latencyMs int) {
	ctx := context.Background()
	state := s.GetCircuitState(providerID)

	if state == StateHalfOpen {
		successesKey := fmt.Sprintf("circuit:%s:successes", providerID)
		successes, _ := s.redis.Incr(ctx, successesKey).Result()

		if successes >= 1 {
			// Close the circuit
			s.setCircuitState(providerID, StateClosed)
			s.redis.Set(ctx, fmt.Sprintf("circuit:%s:failures", providerID), 0, 0)
			s.redis.Set(ctx, successesKey, 0, 0)
		}
	} else if state == StateClosed {
		// Reset failure count on success
		s.redis.Set(ctx, fmt.Sprintf("circuit:%s:failures", providerID), 0, 0)
	}

	// Record latency (1 hour TTL)
	latencyKey := fmt.Sprintf("health:%s:latency", providerID)
	s.redis.Set(ctx, latencyKey, latencyMs, time.Hour)

	// Update health score
	s.updateHealthScore(providerID)
}

// RecordFailure records a failed request
func (s *HealthService) RecordFailure(providerID string) {
	ctx := context.Background()
	state := s.GetCircuitState(providerID)

	failuresKey := fmt.Sprintf("circuit:%s:failures", providerID)
	lastFailureKey := fmt.Sprintf("circuit:%s:last_failure", providerID)

	failures, _ := s.redis.Incr(ctx, failuresKey).Result()
	s.redis.Set(ctx, lastFailureKey, time.Now().UnixMilli(), 0)

	if state == StateHalfOpen {
		// Re-open the circuit
		s.setCircuitState(providerID, StateOpen)
	} else if state == StateClosed && failures >= int64(s.failureThreshold) {
		// Open the circuit
		s.setCircuitState(providerID, StateOpen)
	}

	// Update health score
	s.updateHealthScore(providerID)
}

// GetHealthMetrics returns health metrics for a provider
func (s *HealthService) GetHealthMetrics(providerID string) HealthMetrics {
	ctx := context.Background()

	circuitState := s.GetCircuitState(providerID)

	failuresKey := fmt.Sprintf("circuit:%s:failures", providerID)
	successesKey := fmt.Sprintf("circuit:%s:successes", providerID)
	lastFailureKey := fmt.Sprintf("circuit:%s:last_failure", providerID)
	latencyKey := fmt.Sprintf("health:%s:latency", providerID)
	scoreKey := fmt.Sprintf("health:%s:score", providerID)

	pipe := s.redis.Pipeline()
	failuresCmd := pipe.Get(ctx, failuresKey)
	successesCmd := pipe.Get(ctx, successesKey)
	lastFailureCmd := pipe.Get(ctx, lastFailureKey)
	latencyCmd := pipe.Get(ctx, latencyKey)
	scoreCmd := pipe.Get(ctx, scoreKey)

	pipe.Exec(ctx)

	failureCount, _ := strconv.Atoi(failuresCmd.Val())
	successCount, _ := strconv.Atoi(successesCmd.Val())

	var lastFailureTime *int64
	if lastFailureCmd.Val() != "" {
		if ts, err := strconv.ParseInt(lastFailureCmd.Val(), 10, 64); err == nil {
			lastFailureTime = &ts
		}
	}

	var avgLatency *int
	if latencyCmd.Val() != "" {
		if lat, err := strconv.Atoi(latencyCmd.Val()); err == nil {
			avgLatency = &lat
		}
	}

	healthScore := 1.0
	if scoreCmd.Val() != "" {
		if score, err := strconv.ParseFloat(scoreCmd.Val(), 64); err == nil {
			healthScore = score
		}
	}

	return HealthMetrics{
		ProviderID:      providerID,
		CircuitState:    circuitState,
		FailureCount:    failureCount,
		SuccessCount:    successCount,
		LastFailureTime: lastFailureTime,
		AverageLatency:  avgLatency,
		HealthScore:     healthScore,
	}
}

// GetAllHealthMetrics returns health metrics for all providers
func (s *HealthService) GetAllHealthMetrics() []HealthMetrics {
	ctx := context.Background()

	// Use SCAN to find all circuit keys
	var keys []string
	iter := s.redis.Scan(ctx, 0, "circuit:*:state", 0).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	var metrics []HealthMetrics
	for _, key := range keys {
		// Extract provider ID from key (circuit:{provider}:state)
		var providerID string
		fmt.Sscanf(key, "circuit:%s:state", &providerID)
		if providerID != "" {
			metrics = append(metrics, s.GetHealthMetrics(providerID))
		}
	}

	return metrics
}

func (s *HealthService) setCircuitState(providerID string, state CircuitState) {
	ctx := context.Background()
	stateKey := fmt.Sprintf("circuit:%s:state", providerID)
	s.redis.Set(ctx, stateKey, string(state), 0)
}

func (s *HealthService) updateHealthScore(providerID string) {
	ctx := context.Background()

	failuresKey := fmt.Sprintf("circuit:%s:failures", providerID)
	failures, _ := s.redis.Get(ctx, failuresKey).Int()
	state := s.GetCircuitState(providerID)

	// Calculate health score
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

	scoreKey := fmt.Sprintf("health:%s:score", providerID)
	s.redis.Set(ctx, scoreKey, score, time.Hour)
}

// Global health service instance
var healthService = NewHealthService()

// GetHealthService returns the global health service
func GetHealthService() *HealthService {
	return healthService
}
