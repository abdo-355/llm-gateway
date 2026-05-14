package services

import (
	"context"
	"fmt"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/redis/go-redis/v9"
)

// CooldownReason describes why a provider is on cooldown
type CooldownReason string

const (
	CooldownRateLimit CooldownReason = "rate_limit"
	CooldownOverload  CooldownReason = "overload"
	CooldownBilling   CooldownReason = "billing"
	CooldownAuth      CooldownReason = "auth"
	CooldownQuota     CooldownReason = "quota"
	CooldownDefault   CooldownReason = "default"
)

// CooldownService manages provider cooldowns (separate from circuit breakers)
type CooldownService struct {
	redis  *redis.Client
	prefix string
	config config.CooldownConfig
}

// NewCooldownService creates a new cooldown service
func NewCooldownService(redisClient *redis.Client, keyPrefix string, cfg config.CooldownConfig) *CooldownService {
	prefix := keyPrefix
	if prefix == "" {
		prefix = "cooldown"
	}
	return &CooldownService{
		redis:  redisClient,
		prefix: prefix,
		config: cfg,
	}
}

func (s *CooldownService) buildCooldownKey(providerID, model string) string {
	return fmt.Sprintf("%s:%s:%s", s.prefix, providerID, model)
}

// ApplyCooldown sets a cooldown period for a provider+model
func (s *CooldownService) ApplyCooldown(ctx context.Context, providerID, model string, duration time.Duration, reason CooldownReason) {
	if !s.config.Enabled {
		return
	}

	key := s.buildCooldownKey(providerID, model)
	err := s.redis.Set(ctx, key, string(reason), duration).Err()
	if err != nil {
		logger.Error().
			Str("type", "cooldown").
			Str("event", "cooldown.set_failed").
			Str("provider", providerID).
			Str("model", model).
			Err(err).
			Msg("Failed to set cooldown")
		return
	}

	metrics.CooldownAppliedTotal.WithLabelValues(providerID, model, string(reason)).Inc()
	logger.Info().
		Str("type", "cooldown").
		Str("event", "cooldown.applied").
		Str("provider", providerID).
		Str("model", model).
		Str("reason", string(reason)).
		Dur("duration", duration).
		Msg("Provider cooldown applied")
}

// IsOnCooldown checks if a provider+model is currently on cooldown
func (s *CooldownService) IsOnCooldown(ctx context.Context, providerID, model string) bool {
	if !s.config.Enabled {
		return false
	}

	key := s.buildCooldownKey(providerID, model)
	_, err := s.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return false
	}
	if err != nil {
		logger.Error().
			Str("type", "cooldown").
			Str("event", "cooldown.check_failed").
			Str("provider", providerID).
			Str("model", model).
			Err(err).
			Msg("Failed to check cooldown")
		return false
	}

	return true
}

// GetCooldownRemaining returns the remaining cooldown duration
func (s *CooldownService) GetCooldownRemaining(ctx context.Context, providerID, model string) time.Duration {
	if !s.config.Enabled {
		return 0
	}

	key := s.buildCooldownKey(providerID, model)
	ttl, err := s.redis.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		return 0
	}

	return ttl
}

// GetCooldownReason returns the reason for the current cooldown
func (s *CooldownService) GetCooldownReason(ctx context.Context, providerID, model string) CooldownReason {
	if !s.config.Enabled {
		return CooldownDefault
	}

	key := s.buildCooldownKey(providerID, model)
	reason, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		return CooldownDefault
	}

	return CooldownReason(reason)
}

// ClearCooldown removes a cooldown
func (s *CooldownService) ClearCooldown(ctx context.Context, providerID, model string) {
	key := s.buildCooldownKey(providerID, model)
	s.redis.Del(ctx, key)
}

// GetDurationForReason returns the cooldown duration for a given reason
func (s *CooldownService) GetDurationForReason(reason CooldownReason) time.Duration {
	switch reason {
	case CooldownRateLimit:
		return s.config.RateLimitDuration
	case CooldownBilling:
		return s.config.PaymentDuration
	case CooldownOverload:
		return s.config.Error5xxDuration
	case CooldownAuth:
		return s.config.PaymentDuration
	case CooldownQuota:
		return s.config.RateLimitDuration
	default:
		return s.config.DefaultDuration
	}
}

// ApplyCooldownForReason applies a cooldown with the default duration for the reason
func (s *CooldownService) ApplyCooldownForReason(ctx context.Context, providerID, model string, reason CooldownReason, retryAfterSeconds int) {
	duration := s.GetDurationForReason(reason)

	// If provider gave us a retry-after, use that (capped at max)
	if retryAfterSeconds > 0 {
		retryDuration := time.Duration(retryAfterSeconds) * time.Second
		if retryDuration > 0 && retryDuration < 60*time.Minute {
			duration = retryDuration
		}
	}

	s.ApplyCooldown(ctx, providerID, model, duration, reason)
}

// BatchIsOnCooldown checks cooldown state for multiple provider/model pairs
// in a single pipeline round trip.
func (s *CooldownService) BatchIsOnCooldown(ctx context.Context, pairs []ProviderModelPair) map[string]bool {
	if !s.config.Enabled || len(pairs) == 0 {
		return nil
	}

	pipe := s.redis.Pipeline()
	type cmdEntry struct {
		pair ProviderModelPair
		cmd  *redis.StringCmd
	}
	entries := make([]cmdEntry, len(pairs))
	for i, p := range pairs {
		entries[i] = cmdEntry{
			pair: p,
			cmd:  pipe.Get(ctx, s.buildCooldownKey(p.ProviderID, p.Model)),
		}
	}
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		logger.Error().
			Str("type", "cooldown").
			Str("event", "cooldown.batch_check_failed").
			Err(err).
			Msg("Failed to batch check cooldowns")
	}

	result := make(map[string]bool, len(pairs))
	for _, e := range entries {
		key := e.pair.ProviderID + "/" + e.pair.Model
		_, err := e.cmd.Result()
		result[key] = err == nil
	}
	return result
}
