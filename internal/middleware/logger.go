package middleware

import (
	"time"

	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/gin-gonic/gin"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		requestID := c.GetString("request_id")

		if raw != "" {
			path = path + "?" + raw
		}

		logger.Info("HTTP Request",
			"request_id", requestID,
			"client_ip", clientIP,
			"method", method,
			"path", path,
			"status", statusCode,
			"latency_ms", latency.Milliseconds(),
		)
	}
}
