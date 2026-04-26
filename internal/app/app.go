package app

import (
	"context"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/db"
	"github.com/abdo-355/llm-gateway/internal/handlers"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/server"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/redis/go-redis/v9"
)

type Handlers struct {
	Completions *handlers.CompletionsHandler
	Responses   *handlers.ResponsesHandler
	Health      *handlers.HealthHandler
}

type App struct {
	Router   *services.Router
	Health   *services.HealthService
	Quota    *services.QuotaService
	Provider *services.ProviderService
	Redis    *redis.Client
	Handlers *Handlers
	Server   *server.Server
}

func New(ctx context.Context) (*App, error) {
	env := config.GetEnv()
	logger.Init("llm-gateway", env.Environment, env.LogLevel)

	if err := config.InitGCPCredentials(); err != nil {
		logger.Warn().Err(err).Msg("GCP credentials bootstrap failed")
	}

	if err := services.InitVertexAuth(ctx); err != nil {
		logger.Warn().Err(err).Msg("Vertex auth initialization failed")
	}

	redisClient := db.NewRedisClient()

	quotaSvc := services.NewQuotaService(redisClient, db.GetRedisKey("quota"))
	healthSvc := services.NewHealthService(redisClient, db.GetRedisKey("health"))
	providerSvc := services.NewProviderService()
	routerSvc := services.NewRouter(quotaSvc, healthSvc, providerSvc)

	handlers := &Handlers{
		Completions: handlers.NewCompletionsHandler(routerSvc),
		Responses:   handlers.NewResponsesHandler(routerSvc),
		Health:      handlers.NewHealthHandler(healthSvc),
	}

	srv := server.New(server.Services{
		Router: routerSvc,
		Health: healthSvc,
		Redis:  redisClient,
	})

	return &App{
		Router:   routerSvc,
		Health:   healthSvc,
		Quota:    quotaSvc,
		Provider: providerSvc,
		Redis:    redisClient,
		Handlers: handlers,
		Server:   srv,
	}, nil
}

func (a *App) Start() {
	a.Server.Start()
}
