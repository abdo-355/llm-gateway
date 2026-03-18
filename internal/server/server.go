package server

import (
	"context"
	"crypto/subtle"
	"fmt"
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
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/redis/go-redis/v9"
)

type Services struct {
	Router *services.Router
	Health *services.HealthService
	Redis  *redis.Client
}

type Server struct {
	httpServer    *http.Server
	metricsServer *http.Server
}

func New(svc Services) *Server {
	env := config.GetEnv()

	if strings.EqualFold(env.Environment, "production") {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()

	r.Use(requestid.New())
	r.Use(gin.Recovery())
	r.Use(middleware.Metrics())
	r.Use(middleware.CORS())
	r.Use(middleware.Helmet())
	r.Use(accessLogMiddleware())
	r.Use(middleware.ErrorHandler())

	r.GET("/health", handlers.Health(svc.Health))

	rateLimiter := middleware.NewRateLimiter(svc.Redis)
	r.Use(rateLimiter.RateLimit())

	authorized := r.Group("/")
	authorized.Use(middleware.Auth())
	authorized.POST("/v1/chat/completions", handlers.Completions(svc.Router))
	authorized.POST("/v1/responses", handlers.Responses(svc.Router))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", env.Port),
			Handler:      r,
			ReadTimeout:  60 * time.Second,
			WriteTimeout: 0,
			IdleTimeout:  120 * time.Second,
		},
		metricsServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", env.MetricsPort),
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
	}
}

func (s *Server) Start() {
	go func() {
		logger.Info().
			Str("type", "app").
			Str("event", "server.starting").
			Str("port", s.httpServer.Addr).
			Msg("Starting HTTP server")
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().
				Str("type", "app").
				Str("event", "server.start_failed").
				Err(err).
				Msg("HTTP server failed to start")
			os.Exit(1)
		}
	}()

	go func() {
		logger.Info().
			Str("type", "app").
			Str("event", "metrics_server.starting").
			Str("port", s.metricsServer.Addr).
			Msg("Starting metrics server")
		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().
				Str("type", "app").
				Str("event", "metrics_server.start_failed").
				Err(err).
				Msg("Metrics server failed to start")
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info().
		Str("type", "app").
		Str("event", "server.shutdown").
		Msg("Shutting down servers")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var httpErr, metricsErr error

	done := make(chan struct{})
	go func() {
		httpErr = s.httpServer.Shutdown(ctx)
		metricsErr = s.metricsServer.Shutdown(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}

	if httpErr != nil {
		logger.Error().
			Str("type", "app").
			Str("event", "server.shutdown_failed").
			Err(httpErr).
			Msg("HTTP server forced to shutdown")
	}

	if metricsErr != nil {
		logger.Error().
			Str("type", "app").
			Str("event", "metrics_server.shutdown_failed").
			Err(metricsErr).
			Msg("Metrics server forced to shutdown")
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
		reqID := requestid.Get(c)

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
			Str("request_id", reqID).
			Msg("HTTP request completed")
	}
}

func authMetricsHandler(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header is required", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Authorization header must be 'Bearer <token>'", http.StatusUnauthorized)
			return
		}

		token := parts[1]
		expected := config.GetEnv().GatewayAPIKey
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}
}
