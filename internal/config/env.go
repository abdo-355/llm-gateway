package config

import (
	"fmt"
	"os"
	"strconv"
)

type EnvConfig struct {
	Environment string
	Port        int

	GatewayAPIKey      string
	GroqAPIKey         string
	CerebrasAPIKey     string
	MistralAPIKey      string
	GoogleVertexAPIKey string

	RedisURL       string
	RedisKeyPrefix string

	LogLevel string

	RateLimitPerIP    int
	RateLimitWindowMs int

	CORSOrigins string
}

func LoadEnv() (*EnvConfig, error) {
	required := []string{
		"GATEWAY_API_KEY",
		"GROQ_API_KEY",
		"CEREBRAS_API_KEY",
		"MISTRAL_API_KEY",
		"GOOGLE_VERTEX_API_KEY",
	}

	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	environment := getEnvString("ENVIRONMENT", "development")

	gatewayKey := os.Getenv("GATEWAY_API_KEY")
	if len(gatewayKey) < 32 {
		return nil, fmt.Errorf("GATEWAY_API_KEY must be at least 32 characters")
	}

	port := getEnvInt("PORT", 8080)
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("PORT must be between 1 and 65535")
	}

	rateLimitPerIP := getEnvInt("RATE_LIMIT_PER_IP", 100)
	if rateLimitPerIP < 1 {
		return nil, fmt.Errorf("RATE_LIMIT_PER_IP must be positive")
	}

	rateLimitWindowMs := getEnvInt("RATE_LIMIT_WINDOW_MS", 60000)
	if rateLimitWindowMs < 1 {
		return nil, fmt.Errorf("RATE_LIMIT_WINDOW_MS must be positive")
	}

	return &EnvConfig{
		Environment:        environment,
		Port:               port,
		GatewayAPIKey:      gatewayKey,
		GroqAPIKey:         os.Getenv("GROQ_API_KEY"),
		CerebrasAPIKey:     os.Getenv("CEREBRAS_API_KEY"),
		MistralAPIKey:      os.Getenv("MISTRAL_API_KEY"),
		GoogleVertexAPIKey: os.Getenv("GOOGLE_VERTEX_API_KEY"),
		RedisURL:           getEnvString("REDIS_URL", "redis://localhost:6379"),
		RedisKeyPrefix:     getEnvString("REDIS_KEY_PREFIX", "llm_gateway"),
		LogLevel:           getEnvString("LOG_LEVEL", "info"),
		RateLimitPerIP:     rateLimitPerIP,
		RateLimitWindowMs:  rateLimitWindowMs,
		CORSOrigins:        os.Getenv("CORS_ORIGINS"),
	}, nil
}

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// Singleton instance
var envInstance *EnvConfig

func GetEnv() *EnvConfig {
	if envInstance == nil {
		var err error
		envInstance, err = LoadEnv()
		if err != nil {
			panic(fmt.Sprintf("Failed to load environment: %v", err))
		}
	}
	return envInstance
}
