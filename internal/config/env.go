package config

import (
	"fmt"
	"os"
	"strconv"
	"sync"
)

type EnvConfig struct {
	Environment string
	Port        int
	MetricsPort int
	LogLevel    string

	GatewayAPIKey  string
	GroqAPIKey     string
	CerebrasAPIKey string
	MistralAPIKey  string
	GeminiAPIKey   string
	NimAPIKey      string
	OllamaAPIKey   string
	KiloAPIKey     string

	GoogleVertexProjectID string

	RedisURL       string
	RedisKeyPrefix string

	RateLimitGlobal   bool
	RateLimitPerIP    int
	RateLimitWindowMs int

	CORSOrigins string
}

func LoadEnv() (*EnvConfig, error) {
	required := []string{
		"GATEWAY_API_KEY",
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

	environment := getEnvString("ENV", "development")

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

	rateLimitGlobal := getEnvBool("RATE_LIMIT_GLOBAL", false)

	metricsPort := getEnvInt("METRICS_PORT", 9090)
	if metricsPort < 1 || metricsPort > 65535 {
		return nil, fmt.Errorf("METRICS_PORT must be between 1 and 65535")
	}

	return &EnvConfig{
		Environment:           environment,
		Port:                  port,
		MetricsPort:           metricsPort,
		LogLevel:              getEnvString("LOG_LEVEL", "info"),
		GatewayAPIKey:         gatewayKey,
		GroqAPIKey:            os.Getenv("GROQ_API_KEY"),
		CerebrasAPIKey:        os.Getenv("CEREBRAS_API_KEY"),
		MistralAPIKey:         os.Getenv("MISTRAL_API_KEY"),
		GeminiAPIKey:          os.Getenv("GEMINI_API_KEY"),
		NimAPIKey:             os.Getenv("NIM_API_KEY"),
		OllamaAPIKey:          os.Getenv("OLLAMA_API_KEY"),
		KiloAPIKey:            os.Getenv("KILO_API_KEY"),
		GoogleVertexProjectID: os.Getenv("GOOGLE_VERTEX_PROJECT_ID"),
		RedisURL:              getEnvString("REDIS_URL", "redis://localhost:6379"),
		RedisKeyPrefix:        getEnvString("REDIS_KEY_PREFIX", "llm_gateway"),
		RateLimitGlobal:       rateLimitGlobal,
		RateLimitPerIP:        rateLimitPerIP,
		RateLimitWindowMs:     rateLimitWindowMs,
		CORSOrigins:           os.Getenv("CORS_ORIGINS"),
	}, nil
}

func getEnvString(key, defaultValue string) string {
	// Return environment variable or default if not set
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	// Parse integer from environment variable, return default on error
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvBool parses boolean environment variables
// Accepts: "true", "1", "TRUE", "True" as true values
// Everything else (including empty) returns the default value
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "TRUE" || value == "True"
}

// Singleton instance
var (
	envInstance *EnvConfig
	envOnce     sync.Once
)

func GetEnv() *EnvConfig {
	envOnce.Do(func() {
		var err error
		envInstance, err = LoadEnv()
		if err != nil {
			panic(fmt.Sprintf("Failed to load environment: %v", err))
		}
	})
	return envInstance
}
