package handlers

import (
	"net/http"
	"time"

	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {
	healthService := services.GetHealthService()

	providers := healthService.GetAllHealthMetrics()

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
