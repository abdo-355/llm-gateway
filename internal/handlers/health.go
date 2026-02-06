package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/gin-gonic/gin"
)

// HealthMonitor is the interface for health-related operations
type HealthMonitor interface {
	GetAllHealthMetrics(ctx context.Context) []services.HealthMetrics
}

type HealthHandler struct {
	healthService HealthMonitor
}

func NewHealthHandler(healthSvc HealthMonitor) *HealthHandler {
	return &HealthHandler{
		healthService: healthSvc,
	}
}

func (h *HealthHandler) Health(c *gin.Context) {
	ctx := c.Request.Context()
	providers := h.healthService.GetAllHealthMetrics(ctx)

	status := "healthy"
	for _, p := range providers {
		if p.CircuitState == "OPEN" {
			status = "degraded"
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    status,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"providers": providers,
	})
}
