package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

func Completions(router services.RouterHandler) gin.HandlerFunc {
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
		var logicalModelID string
		if req.Model != "" {
			logicalModel, logicalModelID = resolveLogicalModel(req.Model)
		}

		ctx = metrics.SetLogicalModel(ctx, logicalModelID)
		if req.Router != nil && req.Router.Profile != nil {
			ctx = metrics.SetRouterProfile(ctx, *req.Router.Profile)
		} else {
			ctx = metrics.SetRouterProfile(ctx, "default")
		}
		c.Request = c.Request.WithContext(ctx)

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
			c.Writer.Flush()

			streamResult := router.ExecuteStream(ctx, plan, req, reqID)

			for chunk := range streamResult.Chunks {
				chunkJSON, err := json.Marshal(chunk)
				if err != nil {
					continue
				}
				fmt.Fprintf(c.Writer, "data: %s\n\n", chunkJSON)
				c.Writer.Flush()
			}

			if err := <-streamResult.Err; err != nil {
				errJSON, _ := json.Marshal(err)
				fmt.Fprintf(c.Writer, "data: %s\n\n", errJSON)
				c.Writer.Flush()
			} else {
				fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
				c.Writer.Flush()
			}
			return
		}

		result, err := router.Execute(ctx, plan, req, reqID)
		if err != nil {
			writeExecutionError(c, err)
			return
		}

		writeResultHeaders(c, result, logicalModel)
		c.JSON(http.StatusOK, result.Response)
	}
}
