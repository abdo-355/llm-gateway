package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

func Responses(router services.RouterHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		reqID := requestid.Get(c)

		var req types.ResponseRequest
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

		if err := services.ValidateStatelessRequest(&req); err != nil {
			gatewayErr, ok := err.(*types.GatewayError)
			if !ok {
				gatewayErr = &types.GatewayError{
					Type:    "validation_error",
					Code:    "VALIDATION_ERROR",
					Message: err.Error(),
				}
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": gatewayErr})
			return
		}

		chatReq, err := services.ResponseRequestToChatCompletion(&req)
		if err != nil {
			gatewayErr, ok := err.(*types.GatewayError)
			if !ok {
				gatewayErr = &types.GatewayError{
					Type:    "validation_error",
					Code:    "INPUT_CONVERSION_FAILED",
					Message: err.Error(),
				}
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": gatewayErr})
			return
		}

		var logicalModel *types.LogicalModelConfig
		if req.Model != "" {
			logicalModel = config.GetLogicalModel(req.Model)
		}

		requirements := router.DeriveRequirements(*chatReq, req.Router)

		var candidates []types.RoutingCandidate
		if logicalModel != nil {
			candidates = router.GenerateCandidatesFromLogicalModel(logicalModel)
		} else {
			candidates = router.GenerateCandidates()
			if req.Model != "" {
				candidates = filterCandidatesByModel(candidates, req.Model)
			}
		}

		eligible, filtered := router.FilterCandidates(ctx, candidates, requirements, *chatReq, req.Router)

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
			handleResponsesStream(c, ctx, router, plan, *chatReq, reqID, logicalModel)
			return
		}

		result, err := router.Execute(ctx, plan, *chatReq, reqID)
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

		response := services.ChatCompletionToResponse(&result.Response)

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

		c.JSON(http.StatusOK, response)
	}
}

func handleResponsesStream(
	c *gin.Context,
	ctx context.Context,
	router services.RouterHandler,
	plan types.RoutingPlan,
	chatReq types.ChatCompletionRequest,
	reqID string,
	logicalModel *types.LogicalModelConfig,
) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()

	streamResult := router.ExecuteStream(ctx, plan, chatReq, reqID)

	var lastChunk *types.SSEChunk
	for chunk := range streamResult.Chunks {
		lastChunk = chunk
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
		if lastChunk != nil {
			response := convertStreamChunkToResponse(lastChunk)
			respJSON, _ := json.Marshal(response)
			fmt.Fprintf(c.Writer, "data: %s\n\n", respJSON)
			c.Writer.Flush()
		}
		fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}
}

func convertStreamChunkToResponse(chunk *types.SSEChunk) *types.Response {
	output := make([]types.ResponseItem, 0)
	var outputTexts []string

	if len(chunk.Choices) > 0 {
		choice := chunk.Choices[0]

		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			text := *choice.Delta.Content
			outputTexts = append(outputTexts, text)

			output = append(output, types.ResponseItem{
				ID:     chunk.ID,
				Type:   "message",
				Role:   "assistant",
				Status: "completed",
				Content: []types.ContentOutput{{
					Type: "output_text",
					Text: text,
				}},
			})
		}

		for _, tc := range choice.Delta.ToolCalls {
			callID := tc.ID
			if callID == "" {
				callID = "call_" + chunk.ID
			}

			output = append(output, types.ResponseItem{
				ID:     "fc_" + chunk.ID,
				Type:   "function_call",
				CallID: callID,
				Status: "completed",
			})

			if tc.Function != nil {
				if tc.Function.Name != nil {
					output[len(output)-1].Name = *tc.Function.Name
				}
				if tc.Function.Arguments != nil {
					output[len(output)-1].Arguments = *tc.Function.Arguments
				}
			}
		}
	}

	return &types.Response{
		ID:         "resp_" + chunk.ID,
		Object:     "response",
		CreatedAt:  chunk.Created,
		Model:      chunk.Model,
		Output:     output,
		OutputText: concatStrings(outputTexts),
		Status:     "completed",
	}
}

func concatStrings(strs []string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += "\n"
		}
		result += s
	}
	return result
}

func filterCandidatesByModel(candidates []types.RoutingCandidate, model string) []types.RoutingCandidate {
	var filtered []types.RoutingCandidate
	for _, c := range candidates {
		if c.Model == model {
			filtered = append(filtered, c)
		}
	}
	return filtered
}
