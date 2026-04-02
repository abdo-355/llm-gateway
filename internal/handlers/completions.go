package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

type CompletionsHandler struct {
	pipeline *Pipeline
}

func NewCompletionsHandler(router services.RouterHandler) *CompletionsHandler {
	return &CompletionsHandler{pipeline: NewPipeline(router)}
}

func (h *CompletionsHandler) Handle(c *gin.Context) {
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

	result, err := h.pipeline.Route(ctx, req.Model, req.Router, req)
	if err != nil {
		writeExecutionError(c, err)
		return
	}

	if result.Requirements.Streaming == "required" || (result.Requirements.Streaming == "preferred" && req.Stream != nil && *req.Stream) {
		h.handleStream(c, ctx, req, reqID, result)
		return
	}

	execResult, err := h.pipeline.router.Execute(ctx, result.Plan, req, reqID)
	if err != nil {
		writeExecutionError(c, err)
		return
	}

	writeResultHeaders(c, execResult, result.LogicalModel)
	c.JSON(http.StatusOK, execResult.Response)
}

func (h *CompletionsHandler) handleStream(c *gin.Context, ctx context.Context, req types.ChatCompletionRequest, reqID string, routeResult *RouteResult) {
	writeStreamHeaders(c)

	streamResult := h.pipeline.router.ExecuteStream(ctx, routeResult.Plan, req, reqID)

	for chunk := range streamResult.Chunks {
		if err := writeSSEChunk(c, chunk); err != nil {
			continue
		}
	}

	if err := <-streamResult.Err; err != nil {
		writeSSEError(c, err)
	} else {
		writeSSEDone(c)
	}
}

type ResponsesHandler struct {
	pipeline *Pipeline
}

func NewResponsesHandler(router services.RouterHandler) *ResponsesHandler {
	return &ResponsesHandler{pipeline: NewPipeline(router)}
}

func (h *ResponsesHandler) Handle(c *gin.Context) {
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
		gatewayErr := newGatewayErrorFromTyped(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": gatewayErr})
		return
	}

	chatReq, err := services.ResponseRequestToChatCompletion(&req)
	if err != nil {
		gatewayErr := newGatewayErrorFromTyped(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": gatewayErr})
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

	requirements := h.pipeline.router.DeriveRequirements(*chatReq, req.Router)

	var candidates []types.RoutingCandidate
	if logicalModel != nil {
		candidates = h.pipeline.router.GenerateCandidatesFromLogicalModel(logicalModel)
	} else {
		candidates = h.pipeline.router.GenerateCandidates()
		if req.Model != "" {
			candidates = filterCandidatesByModel(candidates, req.Model)
		}
	}

	eligible, filtered := h.pipeline.router.FilterCandidates(ctx, candidates, requirements, *chatReq, req.Router)
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

	scored := h.pipeline.router.ScoreCandidates(ctx, eligible, req.Router)

	var slo *types.LogicalModelSLO
	if logicalModel != nil {
		slo = logicalModel.SLO
	}

	plan := h.pipeline.router.CompilePlan(scored, req.Router, slo)

	if requirements.Streaming == "required" || (requirements.Streaming == "preferred" && req.Stream != nil && *req.Stream) {
		h.handleStream(c, ctx, req, reqID, *chatReq, plan, logicalModel)
		return
	}

	execResult, err := h.pipeline.router.Execute(ctx, plan, *chatReq, reqID)
	if err != nil {
		writeExecutionError(c, err)
		return
	}

	response := services.ChatCompletionToResponse(&execResult.Response)

	writeResultHeaders(c, execResult, logicalModel)
	c.JSON(http.StatusOK, response)
}

func (h *ResponsesHandler) handleStream(c *gin.Context, ctx context.Context, req types.ResponseRequest, reqID string, chatReq types.ChatCompletionRequest, plan types.RoutingPlan, logicalModel *types.LogicalModelConfig) {
	writeStreamHeaders(c)

	streamResult := h.pipeline.router.ExecuteStream(ctx, plan, chatReq, reqID)

	var lastChunk *types.SSEChunk
	for chunk := range streamResult.Chunks {
		lastChunk = chunk
		if err := writeSSEChunk(c, chunk); err != nil {
			continue
		}
	}

	if err := <-streamResult.Err; err != nil {
		writeSSEError(c, err)
	} else {
		if lastChunk != nil {
			response := convertStreamChunkToResponse(lastChunk)
			respJSON, _ := json.Marshal(response)
			fmt.Fprintf(c.Writer, "data: %s\n\n", respJSON)
			c.Writer.Flush()
		}
		writeSSEDone(c)
	}
}

func newGatewayErrorFromTyped(err error) *types.GatewayError {
	gatewayErr, ok := err.(*types.GatewayError)
	if !ok {
		gatewayErr = &types.GatewayError{
			Type:    "gateway_error",
			Code:    "EXECUTION_ERROR",
			Message: err.Error(),
		}
	}
	return gatewayErr
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
		OutputText: strings.Join(outputTexts, "\n"),
		Status:     "completed",
	}
}
