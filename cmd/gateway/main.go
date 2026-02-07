package main

import (
	"context"
	"log"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/db"
	"github.com/abdo-355/llm-gateway/internal/logger"
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

	if err := services.InitVertexAuth(context.Background()); err != nil {
		log.Printf("Warning: Vertex ADC not available: %v", err)
	}

	redisClient := db.NewRedisClient()

	quotaSvc := services.NewQuotaService(redisClient)
	healthSvc := services.NewHealthService(redisClient)
	providerSvc := services.NewProviderService()
	routerSvc := services.NewRouter(quotaSvc, healthSvc, providerSvc)

	srv := server.New(server.Services{
		Router: routerSvc,
		Health: healthSvc,
		Redis:  redisClient,
	})
	srv.Start()
}
