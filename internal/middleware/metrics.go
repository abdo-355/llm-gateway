package middleware

import (
	"strconv"
	"time"

	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/gin-gonic/gin"
)

const unmatchedRouteMetricPath = "__unmatched__"

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
		path = metricPath(c)

		ctx := c.Request.Context()
		tier := metrics.GetTier(ctx)
		strategy := metrics.GetStrategy(ctx)

		metrics.HTTPRequestsTotal.WithLabelValues(
			method, path, statusStr, tier, strategy,
		).Inc()

		metrics.HTTPRequestDurationSeconds.WithLabelValues(
			method, path, tier, strategy,
		).Observe(elapsed.Seconds())
	}
}

func metricPath(c *gin.Context) string {
	if path := c.FullPath(); path != "" {
		return path
	}
	return unmatchedRouteMetricPath
}
