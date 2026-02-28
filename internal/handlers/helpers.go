package handlers

import (
	"net/http"
	"strconv"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-gonic/gin"
)

func resolveLogicalModel(model string) (*types.LogicalModelConfig, string) {
	if model == "" {
		return nil, ""
	}
	lm := config.GetLogicalModel(model)
	if lm != nil {
		return lm, lm.ID
	}
	return nil, model
}

func writeExecutionError(c *gin.Context, err error) {
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
}

func writeResultHeaders(c *gin.Context, result *types.ExecutionResult, logicalModel *types.LogicalModelConfig) {
	c.Header("X-Gateway-Provider", result.ProviderID)
	c.Header("X-Gateway-Model", result.Model)
	if logicalModel != nil {
		c.Header("X-Gateway-Logical-Model", logicalModel.ID)
	}
	c.Header("X-Gateway-Attempts", strconv.Itoa(result.Attempts))

	tokensUsed := 0
	if result.Response.Usage != nil {
		tokensUsed = result.Response.Usage.TotalTokens
	}
	c.Header("X-Gateway-Tokens-Used", strconv.Itoa(tokensUsed))
}
