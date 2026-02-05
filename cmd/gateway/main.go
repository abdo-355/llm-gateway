package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/db"
	"github.com/abdo-355/llm-gateway/internal/handlers"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/middleware"
	"github.com/gin-gonic/gin"
)

func main() {
	env := config.GetEnv()
	logger.InitLogger(env)

	if env.Environment == "PRODUCTION" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.Logger())
	r.Use(middleware.Helmet())
	r.Use(middleware.CORS())
	r.Use(middleware.ErrorHandler())

	r.GET("/health", handlers.Health)

	authorized := r.Group("/")
	authorized.Use(middleware.Auth())
	authorized.Use(middleware.RateLimit())
	{
		authorized.POST("/v1/chat/completions", handlers.Completions)
		authorized.GET("/metrics", handlers.Metrics)
	}

	srv := &http.Server{
		Addr:    ":" + string(rune(env.Port)),
		Handler: r,
	}

	go func() {
		logger.Info("Starting server", "port", env.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	db.CloseRedis()
	logger.Info("Server exited")
}
