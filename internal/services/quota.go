package services

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

type QuotaService struct {
	redis  *redis.Client
	prefix string
}

func NewQuotaService(redisClient *redis.Client, keyPrefix string) *QuotaService {
	prefix := keyPrefix
	if prefix == "" {
		prefix = "quota"
	}
	return &QuotaService{
		redis:  redisClient,
		prefix: prefix,
	}
}

type QuotaStatus struct {
	Rpm  int
	Rph  int
	Rpd  int
	Tpm  int
	Tph  int
	Tpd  int
	Tpmu int
}

type RateLimitInfo struct {
	IsRateLimited     bool
	RetryAfter        int
	IsPaymentRequired bool
	LimitType         string
}

// EstimateTokens estimates token count from request
func (s *QuotaService) EstimateTokens(req types.ChatCompletionRequest) int {
	estimatedChars := 50 // Base overhead

	for _, msg := range req.Messages {
		estimatedChars += 15 // Message overhead

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
						estimatedChars += 10 // Image tokens
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

	maxTokens := 1000
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	} else if req.MaxCompletionTokens != nil {
		maxTokens = *req.MaxCompletionTokens
	}
	estimatedChars += maxTokens * 4

	return (estimatedChars + 3) / 4 // Round up
}

func (s *QuotaService) CheckModelQuota(ctx context.Context, providerID, model string, limits types.ModelLimits, estimatedTokens int) error {
	now := time.Now().UTC()

	keys := s.buildKeys(providerID, model, now)

	// Calculate cutoff times for sliding windows
	rpmCutoff := now.Add(-60 * time.Second).UnixMilli()
	tpmCutoff := now.Add(-60 * time.Second).UnixMilli()

	pipe := s.redis.Pipeline()

	// RPM: Clean expired entries and count remaining
	pipe.ZRemRangeByScore(ctx, keys.RPM, "0", fmt.Sprintf("%d", rpmCutoff))
	pipe.ZCard(ctx, keys.RPM)

	pipe.Get(ctx, keys.RPH)
	pipe.Get(ctx, keys.RPD)

	// TPM: Get entries in window and sum token counts from member payloads.
	pipe.ZRemRangeByScore(ctx, keys.TPM, "0", fmt.Sprintf("%d", tpmCutoff))
	pipe.ZRangeWithScores(ctx, keys.TPM, 0, -1)

	pipe.Get(ctx, keys.TPH)
	pipe.Get(ctx, keys.TPD)
	pipe.Get(ctx, keys.TPMU)

	results, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		logger.Error().
			Str("type", "db").
			Str("event", "quota.check_failed").
			Err(err).
			Msg("Failed to check quota")
		return err
	}

	status := s.parseResults(results, now)

	if limits.Rpm != nil && *limits.Rpm > 0 {
		ratio := float64(status.Rpm) / float64(*limits.Rpm)
		metrics.QuotaUsageRatio.WithLabelValues(providerID, model, "rpm").Set(ratio)
	}
	if limits.Tpm != nil && *limits.Tpm > 0 {
		ratio := float64(status.Tpm) / float64(*limits.Tpm)
		metrics.QuotaUsageRatio.WithLabelValues(providerID, model, "tpm").Set(ratio)
	}
	if limits.Tpmu != nil && *limits.Tpmu > 0 {
		ratio := float64(status.Tpmu) / float64(*limits.Tpmu)
		metrics.QuotaUsageRatio.WithLabelValues(providerID, model, "tpmu").Set(ratio)
	}

	type quotaCheck struct {
		name    string
		current int
		limit   *int
		adding  int
	}

	checks := []quotaCheck{
		{"rpm", status.Rpm, limits.Rpm, 1},
		{"rph", status.Rph, limits.Rph, 1},
		{"rpd", status.Rpd, limits.Rpd, 1},
		{"tpm", status.Tpm, limits.Tpm, estimatedTokens},
		{"tph", status.Tph, limits.Tph, estimatedTokens},
		{"tpd", status.Tpd, limits.Tpd, estimatedTokens},
		{"tpmu", status.Tpmu, limits.Tpmu, estimatedTokens},
	}

	for _, check := range checks {
		if check.limit != nil && check.current+check.adding > *check.limit {
			metrics.QuotaRejectionsTotal.WithLabelValues(providerID, model, check.name).Inc()
			return errors.NewModelQuotaExceededError(
				fmt.Sprintf("%s limit exceeded: %d/%d", strings.ToUpper(check.name), check.current, *check.limit),
				providerID, model, check.name,
			)
		}
	}

	return nil
}

func (s *QuotaService) RecordModelUsage(ctx context.Context, providerID, model string, tokensUsed int) error {
	now := time.Now().UTC()

	keys := s.buildKeys(providerID, model, now)
	timestamp := now.UnixMilli()
	member := fmt.Sprintf("%d-%d", timestamp, now.Nanosecond())

	pipe := s.redis.Pipeline()

	// RPM: sliding window (60s)
	pipe.ZAdd(ctx, keys.RPM, redis.Z{Score: float64(timestamp), Member: member})
	pipe.Expire(ctx, keys.RPM, 60*time.Second)

	// RPH: hourly counter
	pipe.Incr(ctx, keys.RPH)
	pipe.Expire(ctx, keys.RPH, 2*time.Hour)

	// RPD: daily counter
	pipe.Incr(ctx, keys.RPD)
	pipe.Expire(ctx, keys.RPD, 25*time.Hour)

	// TPM: sliding window with timestamp as score and token count in the member payload.
	pipe.ZAdd(ctx, keys.TPM, redis.Z{
		Score:  float64(timestamp),
		Member: formatTokenWindowMember(timestamp, tokensUsed, now.Nanosecond()),
	})
	pipe.Expire(ctx, keys.TPM, 60*time.Second)

	// TPH: hourly token counter
	pipe.IncrBy(ctx, keys.TPH, int64(tokensUsed))
	pipe.Expire(ctx, keys.TPH, 2*time.Hour)

	// TPD: daily token counter
	pipe.IncrBy(ctx, keys.TPD, int64(tokensUsed))
	pipe.Expire(ctx, keys.TPD, 25*time.Hour)

	// TPMU: monthly token counter
	pipe.IncrBy(ctx, keys.TPMU, int64(tokensUsed))
	pipe.Expire(ctx, keys.TPMU, 31*24*time.Hour)

	_, err := pipe.Exec(ctx)
	if err != nil {
		logger.Error().
			Str("type", "db").
			Str("event", "quota.record_failed").
			Err(err).
			Msg("Failed to record usage")
	}
	return err
}

func (s *QuotaService) AcquireConcurrencySlot(ctx context.Context, providerID, model string, maxConcurrent int) error {
	key := fmt.Sprintf("%s:%s:%s:concurrent", s.prefix, providerID, model)

	count, err := s.redis.Incr(ctx, key).Result()
	if err != nil {
		logger.Error().
			Str("type", "db").
			Str("event", "quota.concurrent_acquire_failed").
			Err(err).
			Msg("Failed to acquire concurrency slot")
		return err
	}

	s.redis.Expire(ctx, key, 120*time.Second)

	if count > int64(maxConcurrent) {
		s.redis.Decr(ctx, key)
		metrics.QuotaRejectionsTotal.WithLabelValues(providerID, model, "concurrent").Inc()
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("Concurrency limit exceeded: %d/%d", count-1, maxConcurrent),
			providerID, model, "concurrent",
		)
	}

	return nil
}

func (s *QuotaService) ReleaseConcurrencySlot(ctx context.Context, providerID, model string) {
	key := fmt.Sprintf("%s:%s:%s:concurrent", s.prefix, providerID, model)
	if _, err := s.redis.Decr(ctx, key).Result(); err != nil {
		logger.Error().
			Str("type", "db").
			Str("event", "quota.concurrent_release_failed").
			Err(err).
			Msg("Failed to release concurrency slot")
	}
}

func (s *QuotaService) CheckConcurrencyLimit(ctx context.Context, providerID, model string, maxConcurrent int) bool {
	key := fmt.Sprintf("%s:%s:%s:concurrent", s.prefix, providerID, model)
	count, err := s.redis.Get(ctx, key).Int64()
	if err != nil {
		if err == redis.Nil {
			return true
		}
		return true
	}
	return int(count) < maxConcurrent
}

func (s *QuotaService) HandleProviderRateLimit(ctx context.Context, providerID, model string, resp *http.Response) RateLimitInfo {
	result := RateLimitInfo{}

	if resp.StatusCode == 402 {
		result.IsRateLimited = true
		result.IsPaymentRequired = true
		result.LimitType = "payment_required"
		return result
	}

	if resp.StatusCode != 429 {
		return result
	}

	result.IsRateLimited = true
	result.LimitType = "rpm"

	// Parse rate limit headers
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			result.RetryAfter = seconds
		}
	}
	if result.RetryAfter == 0 {
		for _, header := range []string{"X-RateLimit-Reset-Requests", "X-RateLimit-Reset-Requests-Minute", "X-RateLimit-Reset-Requests-Day"} {
			if seconds := parseHeaderInt(resp.Header, header); seconds > 0 {
				result.RetryAfter = seconds
				break
			}
		}
	}

	// Update local quota based on provider state
	now := time.Now().UTC()
	keys := s.buildKeys(providerID, model, now)

	limit, remaining, limitType := parseRequestLimitHeaders(providerID, resp.Header)
	if limitType != "" {
		result.LimitType = limitType
	}
	if limit > 0 && remaining >= 0 {
		used := limit - remaining
		if used > 0 {
			switch limitType {
			case "rpm", "rph":
				nowMillis := now.UnixMilli()
				entries := make([]redis.Z, 0, used)
				for i := 0; i < used; i++ {
					entries = append(entries, redis.Z{Score: float64(nowMillis), Member: fmt.Sprintf("provider-rate-limit-%d-%d", nowMillis, i)})
				}
				pipe := s.redis.Pipeline()
				pipe.Del(ctx, keys.RPM)
				pipe.ZAdd(ctx, keys.RPM, entries...)
				pipe.Expire(ctx, keys.RPM, 60*time.Second)
				_, _ = pipe.Exec(ctx)
			case "rpd":
				s.redis.Set(ctx, keys.RPD, used, 25*time.Hour)
			}
		}
	}

	return result
}

func parseRequestLimitHeaders(providerID string, headers http.Header) (int, int, string) {
	switch providerID {
	case "groq":
		limit := parseHeaderInt(headers, "X-RateLimit-Limit-Requests")
		remaining := parseHeaderInt(headers, "X-RateLimit-Remaining-Requests")
		if limit > 0 {
			return limit, remaining, "rpd"
		}
	case "cerebras":
		if limit := parseHeaderInt(headers, "X-RateLimit-Limit-Requests-Minute"); limit > 0 {
			return limit, parseHeaderInt(headers, "X-RateLimit-Remaining-Requests-Minute"), "rpm"
		}
		if limit := parseHeaderInt(headers, "X-RateLimit-Limit-Requests-Day"); limit > 0 {
			return limit, parseHeaderInt(headers, "X-RateLimit-Remaining-Requests-Day"), "rpd"
		}
	default:
		if limit := parseHeaderInt(headers, "X-RateLimit-Limit-Requests"); limit > 0 {
			return limit, parseHeaderInt(headers, "X-RateLimit-Remaining-Requests"), "rpm"
		}
	}
	return 0, 0, ""
}

func parseHeaderInt(headers http.Header, key string) int {
	value := headers.Get(key)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func (s *QuotaService) GetModelQuotaStatus(ctx context.Context, providerID, model string, limits *types.ModelLimits) QuotaStatus {
	now := time.Now().UTC()
	keys := s.buildKeys(providerID, model, now)

	// Calculate cutoff times for sliding windows
	rpmCutoff := now.Add(-60 * time.Second).UnixMilli()
	tpmCutoff := now.Add(-60 * time.Second).UnixMilli()

	pipe := s.redis.Pipeline()

	// RPM: Clean expired entries and count remaining
	pipe.ZRemRangeByScore(ctx, keys.RPM, "0", fmt.Sprintf("%d", rpmCutoff))
	pipe.ZCard(ctx, keys.RPM)

	pipe.Get(ctx, keys.RPH)
	pipe.Get(ctx, keys.RPD)

	// TPM: Clean expired and sum token counts
	pipe.ZRemRangeByScore(ctx, keys.TPM, "0", fmt.Sprintf("%d", tpmCutoff))
	pipe.ZRangeWithScores(ctx, keys.TPM, 0, -1)

	pipe.Get(ctx, keys.TPH)
	pipe.Get(ctx, keys.TPD)
	pipe.Get(ctx, keys.TPMU)

	results, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		logger.Error().
			Str("type", "db").
			Str("event", "quota.status_failed").
			Err(err).
			Msg("Failed to get quota status")
		return QuotaStatus{}
	}

	return s.parseResults(results, now)
}

type quotaKeys struct {
	RPM  string
	RPH  string
	RPD  string
	TPM  string
	TPH  string
	TPD  string
	TPMU string
}

func (s *QuotaService) buildKeys(providerID, model string, now time.Time) quotaKeys {
	prefix := fmt.Sprintf("%s:%s:%s", s.prefix, providerID, model)
	return quotaKeys{
		RPM:  fmt.Sprintf("%s:rpm", prefix),
		RPH:  fmt.Sprintf("%s:rph:%s", prefix, now.Format("2006-01-02-15")),
		RPD:  fmt.Sprintf("%s:rpd:%s", prefix, now.Format("2006-01-02")),
		TPM:  fmt.Sprintf("%s:tpm", prefix),
		TPH:  fmt.Sprintf("%s:tph:%s", prefix, now.Format("2006-01-02-15")),
		TPD:  fmt.Sprintf("%s:tpd:%s", prefix, now.Format("2006-01-02")),
		TPMU: fmt.Sprintf("%s:tpmu:%s", prefix, now.Format("2006-01")),
	}
}

func (s *QuotaService) parseResults(results []redis.Cmder, now time.Time) QuotaStatus {
	status := QuotaStatus{}

	atoi := func(cmd *redis.StringCmd) int {
		if val, err := cmd.Result(); err == nil {
			if n, err := strconv.Atoi(val); err == nil {
				return n
			}
		}
		return 0
	}

	// Pipeline results (after adding ZRemRangeByScore commands):
	// 0: ZRemRangeByScore RPM (ignored)
	// 1: ZCard RPM
	// 2: Get RPH
	// 3: Get RPD
	// 4: ZRemRangeByScore TPM (ignored)
	// 5: ZRangeWithScores TPM
	// 6: Get TPH
	// 7: Get TPD
	// 8: Get TPMU

	if len(results) > 1 {
		status.Rpm = int(results[1].(*redis.IntCmd).Val())
	}
	if len(results) > 2 {
		status.Rph = atoi(results[2].(*redis.StringCmd))
	}
	if len(results) > 3 {
		status.Rpd = atoi(results[3].(*redis.StringCmd))
	}
	if len(results) > 5 {
		// Sum token counts from the member payloads.
		scores := results[5].(*redis.ZSliceCmd).Val()
		totalTokens := 0
		for _, z := range scores {
			totalTokens += parseTokenWindowMember(z.Member)
		}
		status.Tpm = totalTokens
	}
	if len(results) > 6 {
		status.Tph = atoi(results[6].(*redis.StringCmd))
	}
	if len(results) > 7 {
		status.Tpd = atoi(results[7].(*redis.StringCmd))
	}
	if len(results) > 8 {
		status.Tpmu = atoi(results[8].(*redis.StringCmd))
	}

	return status
}

func formatTokenWindowMember(timestamp int64, tokensUsed, nonce int) string {
	return fmt.Sprintf("%d:%d:%d", timestamp, tokensUsed, nonce)
}

func parseTokenWindowMember(member any) int {
	memberStr, ok := member.(string)
	if !ok {
		return 0
	}

	parts := strings.SplitN(memberStr, ":", 3)
	if len(parts) < 2 {
		return 0
	}

	tokens, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}

	return tokens
}
