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
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

type QuotaService struct {
	redis  *redis.Client
	prefix string
}

func NewQuotaService(redis *redis.Client) *QuotaService {
	return &QuotaService{
		redis:  redis,
		prefix: "quota",
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

	pipe := s.redis.Pipeline()
	pipe.ZCard(ctx, keys.RPM)
	pipe.Get(ctx, keys.RPH)
	pipe.Get(ctx, keys.RPD)
	pipe.ZCard(ctx, keys.TPM)
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

	status := s.parseResults(results)

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

	// TPM: sliding window with token counts
	tpmMember := fmt.Sprintf("%d-%d", tokensUsed, now.Nanosecond())
	pipe.ZAdd(ctx, keys.TPM, redis.Z{Score: float64(timestamp), Member: tpmMember})
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

func (s *QuotaService) HandleProviderRateLimit(ctx context.Context, providerID, model string, resp *http.Response) RateLimitInfo {
	result := RateLimitInfo{}

	if resp.StatusCode == 402 {
		result.IsRateLimited = true
		result.IsPaymentRequired = true
		return result
	}

	if resp.StatusCode != 429 {
		return result
	}

	result.IsRateLimited = true

	// Parse rate limit headers
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			result.RetryAfter = seconds
		}
	}

	// Update local quota based on provider state
	now := time.Now().UTC()
	keys := s.buildKeys(providerID, model, now)

	if reqLimit := resp.Header.Get("X-RateLimit-Limit-Requests"); reqLimit != "" {
		if reqRemaining := resp.Header.Get("X-RateLimit-Remaining-Requests"); reqRemaining != "" {
			if limit, err1 := strconv.Atoi(reqLimit); err1 == nil {
				if remaining, err2 := strconv.Atoi(reqRemaining); err2 == nil {
					used := limit - remaining
					if used > 0 {
						s.redis.Set(ctx, keys.RPM, used, 60*time.Second)
					}
				}
			}
		}
	}

	return result
}

func (s *QuotaService) GetModelQuotaStatus(ctx context.Context, providerID, model string, limits *types.ModelLimits) QuotaStatus {
	now := time.Now().UTC()
	keys := s.buildKeys(providerID, model, now)

	pipe := s.redis.Pipeline()
	pipe.ZCard(ctx, keys.RPM)
	pipe.Get(ctx, keys.RPH)
	pipe.Get(ctx, keys.RPD)
	pipe.ZCard(ctx, keys.TPM)
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

	return s.parseResults(results)
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

func (s *QuotaService) parseResults(results []redis.Cmder) QuotaStatus {
	status := QuotaStatus{}

	atoi := func(cmd *redis.StringCmd) int {
		if val, err := cmd.Result(); err == nil {
			if n, err := strconv.Atoi(val); err == nil {
				return n
			}
		}
		return 0
	}

	if len(results) > 0 {
		status.Rpm = int(results[0].(*redis.IntCmd).Val())
	}
	if len(results) > 1 {
		status.Rph = atoi(results[1].(*redis.StringCmd))
	}
	if len(results) > 2 {
		status.Rpd = atoi(results[2].(*redis.StringCmd))
	}
	if len(results) > 3 {
		status.Tpm = int(results[3].(*redis.IntCmd).Val())
	}
	if len(results) > 4 {
		status.Tph = atoi(results[4].(*redis.StringCmd))
	}
	if len(results) > 5 {
		status.Tpd = atoi(results[5].(*redis.StringCmd))
	}
	if len(results) > 6 {
		status.Tpmu = atoi(results[6].(*redis.StringCmd))
	}

	return status
}
