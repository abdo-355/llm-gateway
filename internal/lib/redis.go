package lib

import (
	"context"
	"fmt"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

func GetRedisClient() *redis.Client {
	if redisClient == nil {
		env := config.GetEnv()

		redisClient = redis.NewClient(&redis.Options{
			Addr: env.RedisURL,
			DB:   0,
			// Retry strategy: exponential backoff up to 2s
			MaxRetries:      3,
			MinRetryBackoff: 50 * time.Millisecond,
			MaxRetryBackoff: 2 * time.Second,
		})

		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := redisClient.Ping(ctx).Err(); err != nil {
			Error("Failed to connect to Redis", "error", err)
		} else {
			Info("Connected to Redis")
		}
	}

	return redisClient
}

func CloseRedis() error {
	if redisClient != nil {
		return redisClient.Close()
	}
	return nil
}

func GetRedisKey(key string) string {
	env := config.GetEnv()
	return fmt.Sprintf("%s:%s", env.RedisKeyPrefix, key)
}
