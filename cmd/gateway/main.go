package main

import (
	"log"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/db"
	"github.com/abdo-355/llm-gateway/internal/handlers"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/middleware"
	"github.com/abdo-355/llm-gateway/internal/server"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	env := config.GetEnv()
	logger.Init("llm-gateway", env.Environment)

	// Initialize infrastructure
	redisClient := db.NewRedisClient()

	// Initialize services (in dependency order)
	quotaSvc := services.NewQuotaService(redisClient)
	healthSvc := services.NewHealthService(redisClient)
	providerSvc := services.NewProviderService()
	routerSvc := services.NewRouter(quotaSvc, healthSvc, providerSvc)

	// Initialize handlers
	healthHandler := handlers.NewHealthHandler(healthSvc)
	completionsHandler := handlers.NewCompletionsHandler(routerSvc)
	metricsHandler := handlers.NewMetricsHandler()

	// Initialize middleware
	rateLimiter := middleware.NewRateLimiter(redisClient)
	authMiddleware := middleware.Auth()

	// Create and start server
	srv := server.New(healthHandler, completionsHandler, metricsHandler, authMiddleware, rateLimiter.RateLimit())
	srv.Start()
}
