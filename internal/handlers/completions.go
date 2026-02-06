package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

type Router interface {
	DeriveRequirements(req types.ChatCompletionRequest, hints *types.RouterHints) types.DerivedRequirements
	GenerateCandidates() []types.RoutingCandidate
	GenerateCandidatesFromLogicalModel(logicalModel *types.LogicalModelConfig) []types.RoutingCandidate
	FilterCandidates(ctx context.Context, candidates []types.RoutingCandidate, requirements types.DerivedRequirements, req types.ChatCompletionRequest, hints *types.RouterHints) ([]types.RoutingCandidate, map[string]string)
	ScoreCandidates(ctx context.Context, candidates []types.RoutingCandidate, hints *types.RouterHints) []types.RoutingCandidate
	CompilePlan(candidates []types.RoutingCandidate, hints *types.RouterHints, logicalModelSLO *types.LogicalModelSLO) types.RoutingPlan
	Execute(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) (*types.ExecutionResult, error)
	ExecuteStream(ctx context.Context, plan types.RoutingPlan, req types.ChatCompletionRequest, requestID string) types.StreamResult
}

func Completions(router Router) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		reqID := requestid.Get(c)

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

		var logicalModel *types.LogicalModelConfig
		if req.Model != "" {
			logicalModel = config.GetLogicalModel(req.Model)
		}

		requirements := router.DeriveRequirements(req, req.Router)

		var candidates []types.RoutingCandidate
		if logicalModel != nil {
			candidates = router.GenerateCandidatesFromLogicalModel(logicalModel)
		} else {
			candidates = router.GenerateCandidates()
		}

		eligible, filtered := router.FilterCandidates(ctx, candidates, requirements, req, req.Router)

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

		scored := router.ScoreCandidates(ctx, eligible, req.Router)

		var slo *types.LogicalModelSLO
		if logicalModel != nil {
			slo = logicalModel.SLO
		}

		plan := router.CompilePlan(scored, req.Router, slo)

		if requirements.Streaming == "required" || (requirements.Streaming == "preferred" && req.Stream != nil && *req.Stream) {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")

			streamResult := router.ExecuteStream(ctx, plan, req, reqID)

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

		result, err := router.Execute(ctx, plan, req, reqID)
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

		tokensUsed := 0
		if result.Response.Usage != nil {
			tokensUsed = result.Response.Usage.TotalTokens
		}
		c.Header("X-Gateway-Tokens-Used", strconv.Itoa(tokensUsed))

		c.JSON(http.StatusOK, result.Response)
	}
}
