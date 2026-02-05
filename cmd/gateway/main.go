package main

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/handlers"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/middleware"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		stdlog.Println("No .env file found")
	}

	env := config.GetEnv()

	// Initialize logger
	isProduction := strings.EqualFold(env.Environment, "production")
	logger.Init("llm-gateway", env.Environment)

	// Set Gin mode
	if isProduction {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()

	// Request ID middleware
	r.Use(requestid.New())

	// Custom access log middleware
	r.Use(func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path
		clientIP := c.ClientIP()
		requestID := requestid.Get(c)

		// Skip health endpoint
		if path == "/health" {
			return
		}

		logger.Info().
			Str("type", "http").
			Str("event", "request.completed").
			Str("method", method).
			Str("path", path).
			Int("status", status).
			Str("client_ip", clientIP).
			Str("latency", latency.String()).
			Str("request_id", requestID).
			Msg("HTTP request completed")
	})

	// Recovery middleware
	r.Use(gin.Recovery())

	r.GET("/health", handlers.Health)

	// Global rate limit: DDoS protection before authentication
	r.Use(middleware.RateLimitGlobal())

	authorized := r.Group("/")
	authorized.Use(middleware.Auth(), middleware.RateLimit())
	authorized.POST("/v1/chat/completions", handlers.Completions)
	authorized.GET("/metrics", handlers.Metrics)

	// Server timeouts
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", env.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info().
			Str("type", "app").
			Str("event", "server.starting").
			Int("port", env.Port).
			Msg("Starting server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Error().
			Str("type", "app").
			Str("event", "server.start_failed").
			Err(err).
			Msg("Server failed to start")
		os.Exit(1)
	case <-quit:
		logger.Info().
			Str("type", "app").
			Str("event", "server.shutdown").
			Msg("Shutting down server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error().
				Str("type", "app").
				Str("event", "server.shutdown_failed").
				Err(err).
				Msg("Server forced to shutdown")
		}
		services.CloseServices()
		logger.Info().
			Str("type", "app").
			Str("event", "server.exited").
			Msg("Server exited")
	}
}
