package services

import (
	"context"
	"os"
	"testing"

	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestMain(m *testing.M) {
	os.Setenv("GATEWAY_API_KEY", "test-api-key-that-is-at-least-32-characters-long")
	os.Setenv("GROQ_API_KEY", "test-groq-key")
	os.Setenv("CEREBRAS_API_KEY", "test-cerebras-key")
	os.Setenv("MISTRAL_API_KEY", "test-mistral-key")
	logger.Init("test", "test")
	os.Exit(m.Run())
}

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return client, mr
}

func testContext() context.Context {
	return context.Background()
}
