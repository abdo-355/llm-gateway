package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/handlers"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/middleware"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

type Server struct {
	*http.Server
}

func New() *Server {
	r := gin.New()

	r.Use(requestid.New())
	r.Use(accessLogMiddleware())
	r.Use(gin.Recovery())

	r.GET("/health", handlers.Health)

	r.Use(middleware.RateLimitGlobal())

	authorized := r.Group("/")
	authorized.Use(middleware.Auth(), middleware.RateLimit())
	authorized.POST("/v1/chat/completions", handlers.Completions)
	authorized.GET("/metrics", handlers.Metrics)

	env := config.GetEnv()

	return &Server{
		Server: &http.Server{
			Addr:         fmt.Sprintf(":%d", env.Port),
			Handler:      r,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

func (s *Server) Start() {
	go func() {
		logger.Info().
			Str("type", "app").
			Str("event", "server.starting").
			Str("port", s.Addr).
			Msg("Starting server")
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().
				Str("type", "app").
				Str("event", "server.start_failed").
				Err(err).
				Msg("Server failed to start")
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().
		Str("type", "app").
		Str("event", "server.shutdown").
		Msg("Shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		logger.Error().
			Str("type", "app").
			Str("event", "server.shutdown_failed").
			Err(err).
			Msg("Server forced to shutdown")
	}

	logger.Info().
		Str("type", "app").
		Str("event", "server.exited").
		Msg("Server exited")
}

func accessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path
		clientIP := c.ClientIP()
		requestID := requestid.Get(c)

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
	}
}
