package services

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/abdo-355/llm-gateway/internal/db"
	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/redis/go-redis/v9"
)

type QuotaService struct {
	redis  *redis.Client
	prefix string
}

func NewQuotaService() *QuotaService {
	return &QuotaService{
		redis:  db.GetRedisClient(),
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

func (s *QuotaService) CheckModelQuota(providerID, model string, limits types.ModelLimits, estimatedTokens int) error {
	ctx := context.Background()
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

	results, _ := pipe.Exec(ctx)
	status := s.parseResults(results)
	if limits.Rpm != nil && status.Rpm+1 > *limits.Rpm {
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("RPM limit exceeded: %d/%d", status.Rpm, *limits.Rpm),
			providerID, model, "rpm",
		)
	}

	if limits.Rph != nil && status.Rph+1 > *limits.Rph {
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("RPH limit exceeded: %d/%d", status.Rph, *limits.Rph),
			providerID, model, "rph",
		)
	}

	if limits.Rpd != nil && status.Rpd+1 > *limits.Rpd {
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("RPD limit exceeded: %d/%d", status.Rpd, *limits.Rpd),
			providerID, model, "rpd",
		)
	}

	if limits.Tpm != nil && status.Tpm+estimatedTokens > *limits.Tpm {
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("TPM limit exceeded: %d/%d (est: %d)", status.Tpm, *limits.Tpm, estimatedTokens),
			providerID, model, "tpm",
		)
	}

	if limits.Tph != nil && status.Tph+estimatedTokens > *limits.Tph {
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("TPH limit exceeded: %d/%d (est: %d)", status.Tph, *limits.Tph, estimatedTokens),
			providerID, model, "tph",
		)
	}

	if limits.Tpd != nil && status.Tpd+estimatedTokens > *limits.Tpd {
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("TPD limit exceeded: %d/%d (est: %d)", status.Tpd, *limits.Tpd, estimatedTokens),
			providerID, model, "tpd",
		)
	}

	if limits.Tpmu != nil && status.Tpmu+estimatedTokens > *limits.Tpmu {
		return errors.NewModelQuotaExceededError(
			fmt.Sprintf("TPMU limit exceeded: %d/%d (est: %d)", status.Tpmu, *limits.Tpmu, estimatedTokens),
			providerID, model, "tpmu",
		)
	}

	return nil
}

func (s *QuotaService) RecordModelUsage(providerID, model string, tokensUsed int) {
	ctx := context.Background()
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

	if _, err := pipe.Exec(ctx); err != nil {
		logger.Error("Failed to record usage", "error", err)
	}
}

func (s *QuotaService) HandleProviderRateLimit(providerID, model string, resp *http.Response) RateLimitInfo {
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
	ctx := context.Background()
	now := time.Now().UTC()
	keys := s.buildKeys(providerID, model, now)

	if reqLimit := resp.Header.Get("X-RateLimit-Limit-Requests"); reqLimit != "" {
		if reqRemaining := resp.Header.Get("X-RateLimit-Remaining-Requests"); reqRemaining != "" {
			if limit, err1 := strconv.Atoi(reqLimit); err1 == nil {
				if remaining, err2 := strconv.Atoi(reqRemaining); err2 == nil {
					used := limit - remaining
					s.redis.Set(ctx, keys.RPH, used, 2*time.Hour)
				}
			}
		}
	}

	return result
}

func (s *QuotaService) GetModelQuotaStatus(providerID, model string, limits types.ModelLimits) QuotaStatus {
	ctx := context.Background()
	now := time.Now().UTC()

	keys := s.buildKeys(providerID, model, now)

	pipe := s.redis.Pipeline()
	cmd1 := pipe.ZCard(ctx, keys.RPM)
	cmd2 := pipe.Get(ctx, keys.RPH)
	cmd3 := pipe.Get(ctx, keys.RPD)
	cmd4 := pipe.ZCard(ctx, keys.TPM)
	cmd5 := pipe.Get(ctx, keys.TPH)
	cmd6 := pipe.Get(ctx, keys.TPD)
	cmd7 := pipe.Get(ctx, keys.TPMU)

	pipe.Exec(ctx)

	return QuotaStatus{
		Rpm:  int(cmd1.Val()),
		Rph:  atoi(cmd2.Val()),
		Rpd:  atoi(cmd3.Val()),
		Tpm:  int(cmd4.Val()),
		Tph:  atoi(cmd5.Val()),
		Tpd:  atoi(cmd6.Val()),
		Tpmu: atoi(cmd7.Val()),
	}
}

func atoi(s string) int {
	if s == "" {
		return 0
	}
	i, _ := strconv.Atoi(s)
	return i
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
	base := fmt.Sprintf("%s:%s:%s", s.prefix, providerID, model)
	year, month, day := now.Date()
	hour := now.Hour()

	return quotaKeys{
		RPM:  fmt.Sprintf("%s:rpm", base),
		RPH:  fmt.Sprintf("%s:rph:%d-%02d-%02d-%02d", base, year, month, day, hour),
		RPD:  fmt.Sprintf("%s:rpd:%d-%02d-%02d", base, year, month, day),
		TPM:  fmt.Sprintf("%s:tpm", base),
		TPH:  fmt.Sprintf("%s:tph:%d-%02d-%02d-%02d", base, year, month, day, hour),
		TPD:  fmt.Sprintf("%s:tpd:%d-%02d-%02d", base, year, month, day),
		TPMU: fmt.Sprintf("%s:tpmu:%d-%02d", base, year, month),
	}
}

func (s *QuotaService) parseResults(results []redis.Cmder) QuotaStatus {
	status := QuotaStatus{}
	if len(results) >= 7 {
		status.Rpm = int(results[0].(*redis.IntCmd).Val())
		status.Rph = atoi(results[1].(*redis.StringCmd).Val())
		status.Rpd = atoi(results[2].(*redis.StringCmd).Val())
		status.Tpm = int(results[3].(*redis.IntCmd).Val())
		status.Tph = atoi(results[4].(*redis.StringCmd).Val())
		status.Tpd = atoi(results[5].(*redis.StringCmd).Val())
		status.Tpmu = atoi(results[6].(*redis.StringCmd).Val())
	}
	return status
}

var quotaService = NewQuotaService()

func GetQuotaService() *QuotaService {
	return quotaService
}
