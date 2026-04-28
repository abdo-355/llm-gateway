package services

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

type CloudflareUsageStats struct {
	CachedInputTokens     int
	NonCachedInputTokens  int
	Neurons               int
	EstimatedUSDIfPaid    float64
	RemainingDailyNeurons int
}

type CloudflareBudgetManager interface {
	EstimateCloudflareRequestNeurons(model string, req types.ChatCompletionRequest) int
	CheckCloudflareDailyNeuronBudget(ctx context.Context, model string, estimatedNeurons int) error
	RecordCloudflareNeuronUsage(ctx context.Context, model string, usage *types.Usage) (CloudflareUsageStats, error)
}

type cloudflareNeuronKeys struct {
	ProviderDaily string
	ModelDaily    string
}

func (s *QuotaService) EstimateCloudflareRequestNeurons(model string, req types.ChatCompletionRequest) int {
	pricing, ok := cloudflarePricingForModel(model)
	if !ok {
		return 0
	}
	promptTokens := s.estimatePromptTokens(req)
	completionTokens := cloudflareEstimatedCompletionTokens(req)
	stats := cloudflareUsageToNeuronStats(pricing, promptTokens, 0, completionTokens)
	return int(math.Ceil(float64(stats.Neurons)))
}

func (s *QuotaService) CheckCloudflareDailyNeuronBudget(ctx context.Context, model string, estimatedNeurons int) error {
	remaining := s.GetCloudflareRemainingDailyNeurons(ctx)
	metrics.ProviderDailyBudgetRemaining.WithLabelValues(cloudflareProviderID).Set(float64(remaining))

	if remaining <= cloudflareMinRemainingDailyBudget {
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("Cloudflare daily neuron budget nearly exhausted: %d remaining", remaining),
			cloudflareProviderID,
			model,
			"daily_neurons",
		)
	}

	if estimatedNeurons > 0 && estimatedNeurons > remaining {
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("Cloudflare daily neuron budget exceeded: need %d, remaining %d", estimatedNeurons, remaining),
			cloudflareProviderID,
			model,
			"daily_neurons",
		)
	}

	return nil
}

func (s *QuotaService) RecordCloudflareNeuronUsage(ctx context.Context, model string, usage *types.Usage) (CloudflareUsageStats, error) {
	if usage == nil {
		return CloudflareUsageStats{}, nil
	}

	pricing, ok := cloudflarePricingForModel(model)
	if !ok {
		return CloudflareUsageStats{}, nil
	}

	cachedTokens := 0
	if usage.PromptTokensDetails != nil {
		cachedTokens = usage.PromptTokensDetails.CachedTokens
	}

	stats := cloudflareUsageToNeuronStats(pricing, usage.PromptTokens, cachedTokens, usage.CompletionTokens)
	if stats.Neurons == 0 {
		stats.RemainingDailyNeurons = s.GetCloudflareRemainingDailyNeurons(ctx)
		return stats, nil
	}

	now := time.Now().UTC()
	keys := s.buildCloudflareNeuronKeys(model, now)
	pipe := s.redis.Pipeline()
	pipe.IncrBy(ctx, keys.ProviderDaily, int64(stats.Neurons))
	pipe.Expire(ctx, keys.ProviderDaily, 25*time.Hour)
	pipe.IncrBy(ctx, keys.ModelDaily, int64(stats.Neurons))
	pipe.Expire(ctx, keys.ModelDaily, 25*time.Hour)

	results, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		logger.Error().
			Str("type", "db").
			Str("event", "cloudflare.neuron_record_failed").
			Err(err).
			Msg("Failed to record Cloudflare neuron usage")
		return CloudflareUsageStats{}, err
	}

	usedToday := 0
	if len(results) > 0 {
		usedToday = int(results[0].(*redis.IntCmd).Val())
	}
	stats.RemainingDailyNeurons = max(0, cloudflareFreeDailyNeuronBudget-usedToday)
	metrics.ProviderDailyBudgetRemaining.WithLabelValues(cloudflareProviderID).Set(float64(stats.RemainingDailyNeurons))
	return stats, nil
}

func (s *QuotaService) GetCloudflareRemainingDailyNeurons(ctx context.Context) int {
	if s.redis == nil {
		return cloudflareFreeDailyNeuronBudget
	}
	now := time.Now().UTC()
	keys := s.buildCloudflareNeuronKeys("", now)
	used, err := s.redis.Get(ctx, keys.ProviderDaily).Int()
	if err != nil && err != redis.Nil {
		logger.Error().
			Str("type", "db").
			Str("event", "cloudflare.neuron_budget_read_failed").
			Err(err).
			Msg("Failed to read Cloudflare neuron budget")
		return 0
	}
	remaining := cloudflareFreeDailyNeuronBudget - used
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *QuotaService) buildCloudflareNeuronKeys(model string, now time.Time) cloudflareNeuronKeys {
	providerPrefix := fmt.Sprintf("%s:%s", s.prefix, cloudflareProviderID)
	keys := cloudflareNeuronKeys{
		ProviderDaily: fmt.Sprintf("%s:neurons:%s", providerPrefix, now.Format("2006-01-02")),
	}
	if model != "" {
		keys.ModelDaily = fmt.Sprintf("%s:%s:neurons:%s", providerPrefix, model, now.Format("2006-01-02"))
	}
	return keys
}

func (s *QuotaService) estimatePromptTokens(req types.ChatCompletionRequest) int {
	estimatedChars := 50

	for _, msg := range req.Messages {
		estimatedChars += 15

		switch content := msg.Content.(type) {
		case string:
			estimatedChars += len(content)
		case []any:
			for _, item := range content {
				if part, ok := item.(map[string]any); ok {
					if text, ok := part["text"].(string); ok {
						estimatedChars += len(text)
					}
					if _, ok := part["image_url"]; ok {
						estimatedChars += 10
					}
				}
			}
		}

		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				estimatedChars += 60
				estimatedChars += len(tc.Function.Name)
				estimatedChars += len(tc.Function.Arguments)
			}
		}
	}

	return max(1, (estimatedChars+3)/4)
}
