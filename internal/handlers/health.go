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

type HealthHandler struct {
	healthSvc HealthMonitor
}

func NewHealthHandler(healthSvc HealthMonitor) *HealthHandler {
	return &HealthHandler{healthSvc: healthSvc}
}

func (h *HealthHandler) Handle(c *gin.Context) {
	ctx := c.Request.Context()
	metrics := h.healthSvc.GetAllHealthMetrics(ctx)

	status := "healthy"
	for _, m := range metrics {
		if m.CircuitState == services.StateOpen {
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

type HealthHandler struct {
	healthSvc HealthMonitor
}

func NewHealthHandler(healthSvc HealthMonitor) *HealthHandler {
	return &HealthHandler{healthSvc: healthSvc}
}

func (h *HealthHandler) Handle(c *gin.Context) {
	ctx := c.Request.Context()
	metrics := h.healthSvc.GetAllHealthMetrics(ctx)

	status := "healthy"
	for _, m := range metrics {
		if m.CircuitState == services.StateOpen {
			status = "degraded"
			break
		}
	}

	vertexProject := config.GetEnv().GoogleCloudProject
	if vertexProject == "" {
		vertexProject = config.GetEnv().GoogleVertexProjectID
	}

	c.JSON(http.StatusOK, gin.H{
		"status":           status,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
		"models":          metrics,
		"vertex_configured": config.IsVertexAuthConfigured(),
		"vertex_project":  vertexProject,
	})
}
