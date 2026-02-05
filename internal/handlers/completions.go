package handlers

import (
	"net/http"
	"strconv"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-gonic/gin"
)

func Completions(c *gin.Context) {
	requestID := c.GetString("request_id")

	var req types.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "validation_error",
				"code":    "VALIDATION_FAILED",
				"message": "Invalid request body",
				"details": err.Error(),
			},
		})
		return
	}

	routerService := services.GetRouter()

	var logicalModel *types.LogicalModelConfig
	if req.Model != "" {
		logicalModel = config.GetLogicalModel(req.Model)
	}

	requirements := routerService.DeriveRequirements(req, req.Router)

	var candidates []types.RoutingCandidate
	if logicalModel != nil {
		candidates = routerService.GenerateCandidatesFromLogicalModel(logicalModel)
	} else {
		candidates = routerService.GenerateCandidates()
	}

	eligible, filtered := routerService.FilterCandidates(candidates, requirements, req, req.Router)

	if len(eligible) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": gin.H{
				"type":    "gateway_error",
				"code":    "NO_ELIGIBLE_PROVIDER",
				"message": "No eligible provider found",
				"details": gin.H{
					"requirements":       requirements,
					"filtered_providers": filtered,
				},
			},
		})
		return
	}

	scored := routerService.ScoreCandidates(eligible, req.Router)

	var slo *types.LogicalModelSLO
	if logicalModel != nil {
		slo = logicalModel.SLO
	}

	plan := routerService.CompilePlan(scored, req.Router, slo)

	if requirements.Streaming == "required" || (requirements.Streaming == "preferred" && req.Stream != nil && *req.Stream) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		streamResult := routerService.ExecuteStream(plan, req, requestID)

		for chunk := range streamResult.Chunks {
			c.SSEvent("message", chunk)
		}

		if err := <-streamResult.Err; err != nil {
			c.SSEvent("error", err)
		} else {
			c.SSEvent("message", "[DONE]")
		}
		return
	}

	result, err := routerService.Execute(plan, req, requestID)
	if err != nil {
		gatewayErr, ok := err.(*types.GatewayError)
		if !ok {
			gatewayErr = &types.GatewayError{
				Type:    "gateway_error",
				Code:    "EXECUTION_ERROR",
				Message: err.Error(),
			}
		}

		status := http.StatusInternalServerError
		if gatewayErr.Code == "RATE_LIMITED" {
			status = http.StatusTooManyRequests
		}

		c.JSON(status, gin.H{"error": gatewayErr})
		return
	}

	c.Header("X-Gateway-Provider", result.ProviderID)
	c.Header("X-Gateway-Model", result.Model)
	if logicalModel != nil {
		c.Header("X-Gateway-Logical-Model", logicalModel.ID)
	}
	c.Header("X-Gateway-Attempts", strconv.Itoa(result.Attempts))

	c.JSON(http.StatusOK, result.Response)
}
