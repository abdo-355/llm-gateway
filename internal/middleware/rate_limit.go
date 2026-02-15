package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/db"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	redis       *redis.Client
	maxRequests int
	windowMs    int
}

func NewRateLimiter(redis *redis.Client) *RateLimiter {
	env := config.GetEnv()
	return &RateLimiter{
		redis:       redis,
		maxRequests: env.RateLimitPerIP,
		windowMs:    env.RateLimitWindowMs,
	}
}

func (rl *RateLimiter) RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		ctx := c.Request.Context()

		key := db.GetRedisKey("ratelimit:" + clientIP)
		window := time.Duration(rl.windowMs) * time.Millisecond

		now := float64(time.Now().UnixMilli())
		cutoff := float64(time.Now().Add(-window).UnixMilli())

		// Step 1: Clean old entries and check count (without adding)
		checkPipe := rl.redis.Pipeline()
		checkPipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatFloat(cutoff, 'f', 0, 64))
		checkPipe.ZCard(ctx, key)

		results, err := checkPipe.Exec(ctx)
		if err != nil {
			logger.Error().
				Str("type", "middleware").
				Str("event", "ratelimit.check_failed").
				Err(err).
				Msg("Rate limit check failed")
			c.Next()
			return
		}

		currentCount := results[1].(*redis.IntCmd).Val()

		if currentCount >= int64(rl.maxRequests) {
			metrics.RateLimitRejectionsTotal.Inc()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"type":    "rate_limit_error",
					"code":    "RATE_LIMIT_EXCEEDED",
					"message": "Rate limit exceeded. Please try again later.",
				},
			})
			return
		}

		// Step 2: Only add the request if allowed
		addPipe := rl.redis.Pipeline()
		addPipe.ZAdd(ctx, key, redis.Z{Score: now, Member: now})
		addPipe.Expire(ctx, key, window)
		if _, err := addPipe.Exec(ctx); err != nil {
			logger.Error().
				Str("type", "middleware").
				Str("event", "ratelimit.record_failed").
				Err(err).
				Msg("Rate limit record failed")
		}

		c.Next()
	}
}
