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

type ProviderModelPair struct {
	ProviderID string
	Model      string
}

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
		failureThreshold: 3,
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

	var failures int
	if state == StateHalfOpen {
		successesKey := fmt.Sprintf("%s:successes", circuitPrefix)
		successes, _ := s.redis.Incr(ctx, successesKey).Result()
		s.redis.Expire(ctx, successesKey, 24*time.Hour)

		if successes >= 2 {
			s.setCircuitState(ctx, providerID, model, StateClosed)
			s.redis.Del(ctx, fmt.Sprintf("%s:failures", circuitPrefix))
			s.redis.Del(ctx, successesKey)
			state = StateClosed
		}
	} else if state == StateClosed {
		failuresKey := fmt.Sprintf("%s:failures", circuitPrefix)
		failures, _ = s.redis.Get(ctx, failuresKey).Int()
		if failures > 0 {
			s.redis.Decr(ctx, failuresKey)
			failures--
		}
	}

	latencyKey := fmt.Sprintf("%s:latencies", healthPrefix)
	now := time.Now()
	member := fmt.Sprintf("%d-%d", now.UnixMilli(), now.Nanosecond())

	score := healthScoreFromState(state, failures)
	scoreKey := fmt.Sprintf("%s:score", healthPrefix)

	pipe := s.redis.Pipeline()
	pipe.ZAdd(ctx, latencyKey, redis.Z{Score: float64(latencyMs), Member: member})
	pipe.Expire(ctx, latencyKey, time.Hour)
	pipe.Set(ctx, scoreKey, score, time.Hour)
	pipe.Exec(ctx)
}

func (s *HealthService) RecordFailure(ctx context.Context, providerID, model string) {
	state := s.GetCircuitState(ctx, providerID, model)
	circuitPrefix := s.buildCircuitKeyPrefix(providerID, model)

	failuresKey := fmt.Sprintf("%s:failures", circuitPrefix)
	lastFailureKey := fmt.Sprintf("%s:last_failure", circuitPrefix)

	failures, _ := s.redis.Incr(ctx, failuresKey).Result()

	if state == StateHalfOpen {
		s.setCircuitState(ctx, providerID, model, StateOpen)
		state = StateOpen
	} else if state == StateClosed && failures >= int64(s.failureThreshold) {
		s.setCircuitState(ctx, providerID, model, StateOpen)
		state = StateOpen
	}

	score := healthScoreFromState(state, int(failures))
	healthPrefix := s.buildHealthKeyPrefix(providerID, model)
	scoreKey := fmt.Sprintf("%s:score", healthPrefix)

	pipe := s.redis.Pipeline()
	pipe.Expire(ctx, failuresKey, 24*time.Hour)
	pipe.Set(ctx, lastFailureKey, time.Now().UnixMilli(), 24*time.Hour)
	pipe.Set(ctx, scoreKey, score, time.Hour)
	pipe.Exec(ctx)
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

func healthScoreFromState(state CircuitState, failures int) float64 {
	switch state {
	case StateOpen:
		return 0.0
	case StateHalfOpen:
		return 0.5
	default:
		if failures > 3 {
			return 0.5
		} else if failures > 0 {
			return 1.0 - float64(failures)*0.1
		}
		return 1.0
	}
}

// BatchCircuitStates reads circuit breaker states for multiple provider/model pairs
// in a single pipeline round trip.
func (s *HealthService) BatchCircuitStates(ctx context.Context, pairs []ProviderModelPair) map[string]CircuitState {
	if len(pairs) == 0 {
		return nil
	}

	pipe := s.redis.Pipeline()
	type cmdEntry struct {
		key  string
		cmd  *redis.StringCmd
		pair ProviderModelPair
	}
	entries := make([]cmdEntry, len(pairs))
	for i, p := range pairs {
		key := fmt.Sprintf("%s:state", s.buildCircuitKeyPrefix(p.ProviderID, p.Model))
		entries[i] = cmdEntry{key: key, cmd: pipe.Get(ctx, key), pair: p}
	}
	pipe.Exec(ctx)

	result := make(map[string]CircuitState, len(pairs))
	for _, e := range entries {
		key := e.pair.ProviderID + "/" + e.pair.Model
		val, err := e.cmd.Result()
		if err != nil || val == "" {
			result[key] = StateClosed
		} else {
			result[key] = CircuitState(val)
		}
	}
	return result
}

// BatchGetHealthMetrics reads health metrics for multiple provider/model pairs
// in a single pipeline round trip.
func (s *HealthService) BatchGetHealthMetrics(ctx context.Context, pairs []ProviderModelPair) []HealthMetrics {
	if len(pairs) == 0 {
		return nil
	}

	pipe := s.redis.Pipeline()
	type cmdGroup struct {
		pair            ProviderModelPair
		stateCmd        *redis.StringCmd
		failuresCmd     *redis.StringCmd
		successesCmd    *redis.StringCmd
		lastFailureCmd  *redis.StringCmd
		scoreCmd        *redis.StringCmd
		latencyCmd      *redis.ZSliceCmd
	}
	groups := make([]cmdGroup, len(pairs))

	for i, p := range pairs {
		circuitPrefix := s.buildCircuitKeyPrefix(p.ProviderID, p.Model)
		healthPrefix := s.buildHealthKeyPrefix(p.ProviderID, p.Model)
		groups[i] = cmdGroup{
			pair:           p,
			stateCmd:       pipe.Get(ctx, fmt.Sprintf("%s:state", circuitPrefix)),
			failuresCmd:    pipe.Get(ctx, fmt.Sprintf("%s:failures", circuitPrefix)),
			successesCmd:   pipe.Get(ctx, fmt.Sprintf("%s:successes", circuitPrefix)),
			lastFailureCmd: pipe.Get(ctx, fmt.Sprintf("%s:last_failure", circuitPrefix)),
			scoreCmd:       pipe.Get(ctx, fmt.Sprintf("%s:score", healthPrefix)),
			latencyCmd:     pipe.ZRangeWithScores(ctx, fmt.Sprintf("%s:latencies", healthPrefix), 0, -1),
		}
	}
	_, _ = pipe.Exec(ctx)

	results := make([]HealthMetrics, len(groups))
	for i, g := range groups {
		failureCount, _ := strconv.Atoi(g.failuresCmd.Val())
		successCount, _ := strconv.Atoi(g.successesCmd.Val())

		circuitState := StateClosed
		if val, err := g.stateCmd.Result(); err == nil && val != "" {
			circuitState = CircuitState(val)
		}

		var lastFailureTime *int64
		if val := g.lastFailureCmd.Val(); val != "" {
			if ts, err := strconv.ParseInt(val, 10, 64); err == nil {
				lastFailureTime = &ts
			}
		}

		var avgLatency *int
		if scores := g.latencyCmd.Val(); len(scores) > 0 {
			sum := 0
			for _, z := range scores {
				sum += int(z.Score)
			}
			avg := sum / len(scores)
			avgLatency = &avg
		}

		healthScore := 1.0
		if val := g.scoreCmd.Val(); val != "" {
			if score, err := strconv.ParseFloat(val, 64); err == nil {
				healthScore = score
			}
		}

		results[i] = HealthMetrics{
			ProviderID:      g.pair.ProviderID,
			Model:           g.pair.Model,
			CircuitState:    circuitState,
			FailureCount:    failureCount,
			SuccessCount:    successCount,
			LastFailureTime: lastFailureTime,
			AverageLatency:  avgLatency,
			HealthScore:     healthScore,
		}
	}
	return results
}
