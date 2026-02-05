package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	// Global rate limit constants for DDoS protection
	// These are fixed regardless of authentication status
	rateLimitGlobalMaxRequests = 1000
	rateLimitGlobalWindowMs    = 60000
)

// RateLimitGlobal provides DDoS protection before authentication
// Uses fixed high limits (1000 req/min) to allow legitimate traffic while blocking abuse
func RateLimitGlobal() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		applyRateLimit(c, clientIP, rateLimitGlobalMaxRequests, rateLimitGlobalWindowMs)
	}
}

// RateLimit provides per-user rate limiting after authentication
// Uses configurable limits from environment variables
func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		env := config.GetEnv()
		applyRateLimit(c, clientIP, env.RateLimitPerIP, env.RateLimitWindowMs)
	}
}

// applyRateLimit implements sliding window rate limiting using Redis sorted sets
// Algorithm: Store each request as a member with timestamp as score
// - Remove entries outside the window
// - Count remaining entries
// - Add current request
// - Set expiration on the key
func applyRateLimit(c *gin.Context, clientIP string, maxRequests, windowMs int) {
	redisClient := db.GetRedisClient()
	ctx := context.Background()

	// Key format: ratelimit:global:{ip} - prefixed by RedisKeyPrefix
	key := db.GetRedisKey("ratelimit:global:" + clientIP)
	window := time.Duration(windowMs) * time.Millisecond

	now := float64(time.Now().UnixMilli())
	cutoff := float64(time.Now().Add(-window).UnixMilli())

	// Use pipeline to reduce round trips:
	// 1. Remove expired entries (outside window)
	// 2. Count current entries
	// 3. Add new request
	// 4. Set key expiration
	pipe := redisClient.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatFloat(cutoff, 'f', 0, 64))
	pipe.ZCard(ctx, key)
	pipe.ZAdd(ctx, key, redis.Z{Score: now, Member: now})
	pipe.Expire(ctx, key, window)

	results, _ := pipe.Exec(ctx)
	currentCount := results[1].(*redis.IntCmd).Val()

	// Check if over limit
	if currentCount >= int64(maxRequests) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": gin.H{
				"type":    "rate_limit_error",
				"code":    "RATE_LIMIT_EXCEEDED",
				"message": "Rate limit exceeded. Please try again later.",
			},
		})
		return
	}

	c.Next()
}
