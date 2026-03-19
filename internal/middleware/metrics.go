package middleware

import (
	"strconv"
	"time"

	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/gin-gonic/gin"
)

func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/health" || path == "/metrics" {
			c.Next()
			return
		}

		metrics.HTTPRequestsInFlight.Inc()
		defer metrics.HTTPRequestsInFlight.Dec()

		start := time.Now()

		c.Next()

		elapsed := time.Since(start)
		statusStr := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method
		path = c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		ctx := c.Request.Context()
		logicalModel := metrics.GetLogicalModel(ctx)
		routerProfile := metrics.GetRouterProfile(ctx)

		metrics.HTTPRequestsTotal.WithLabelValues(
			method, path, statusStr, logicalModel, routerProfile,
		).Inc()

		metrics.HTTPRequestDurationSeconds.WithLabelValues(
			method, path, logicalModel, routerProfile,
		).Observe(elapsed.Seconds())
	}
}
