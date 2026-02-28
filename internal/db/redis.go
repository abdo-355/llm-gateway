package db

import (
	"context"
	"fmt"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/redis/go-redis/v9"
)

func NewRedisClient() *redis.Client {
	env := config.GetEnv()

	opt, err := redis.ParseURL(env.RedisURL)
	if err != nil {
		logger.Error().
			Str("type", "db").
			Str("event", "redis.url_parse_failed").
			Err(err).
			Str("url", env.RedisURL).
			Msg("Failed to parse Redis URL")
		panic(fmt.Sprintf("Failed to parse Redis URL: %v", err))
	}

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		logger.Warn().
			Str("type", "db").
			Str("event", "redis.connect_failed").
			Err(err).
			Msg("Failed to connect to Redis — will retry on first use")
	} else {
		logger.Info().
			Str("type", "db").
			Str("event", "redis.connected").
			Str("url", env.RedisURL).
			Msg("Connected to Redis")
	}

	return client
}

func GetRedisKey(key string) string {
	env := config.GetEnv()
	return fmt.Sprintf("%s:%s", env.RedisKeyPrefix, key)
}
