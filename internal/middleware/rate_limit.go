package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/abdo-355/llm-gateway/internal/db"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		redisClient := db.GetRedisClient()
		ctx := context.Background()

		key := db.GetRedisKey("ratelimit:" + clientIP)
		window := time.Duration(60000) * time.Millisecond
		maxRequests := 100

		now := float64(time.Now().UnixMilli())
		cutoff := float64(time.Now().Add(-window).UnixMilli())

		pipe := redisClient.Pipeline()
		pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatFloat(cutoff, 'f', 0, 64))
		pipe.ZCard(ctx, key)
		pipe.ZAdd(ctx, key, redis.Z{Score: now, Member: now})
		pipe.Expire(ctx, key, window)

		results, _ := pipe.Exec(ctx)
		currentCount := results[1].(*redis.IntCmd).Val()

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
}
