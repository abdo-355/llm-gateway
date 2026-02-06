package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/gin-gonic/gin"
)

type HealthMonitor interface {
	GetAllHealthMetrics(ctx context.Context) []services.HealthMetrics
}

func Health(healthSvc HealthMonitor) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		metrics := healthSvc.GetAllHealthMetrics(ctx)

		status := "healthy"
		for _, m := range metrics {
			if m.CircuitState == "OPEN" {
				status = "degraded"
				break
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    status,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"models":    metrics,
		})
	}
}
