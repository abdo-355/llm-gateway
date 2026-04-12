package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

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
	c.Request = c.Request.WithContext(result.Ctx)

	if result.Requirements.Streaming == "required" || (result.Requirements.Streaming == "preferred" && req.Stream != nil && *req.Stream) {
		h.handleStream(c, ctx, req, reqID, result)
		return
	}

	execResult, err := h.pipeline.router.Execute(result.Ctx, result.Plan, req, reqID)
	if err != nil {
		writeExecutionError(c, err)
		return
	}

	writeResultHeaders(c, execResult, result.LogicalModel)
	c.JSON(http.StatusOK, execResult.Response)
}

func (h *CompletionsHandler) handleStream(c *gin.Context, ctx context.Context, req types.ChatCompletionRequest, reqID string, routeResult *RouteResult) {
	writeStreamHeaders(c)

	streamResult := h.pipeline.router.ExecuteStream(routeResult.Ctx, routeResult.Plan, req, reqID)

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

	result, err := h.pipeline.Route(ctx, req.Model, req.Router, *chatReq)
	if err != nil {
		writeExecutionError(c, err)
		return
	}
	c.Request = c.Request.WithContext(result.Ctx)

	if result.Requirements.Streaming == "required" || (result.Requirements.Streaming == "preferred" && req.Stream != nil && *req.Stream) {
		h.handleStream(c, ctx, req, reqID, *chatReq, result.Plan, result.LogicalModel)
		return
	}

	execResult, err := h.pipeline.router.Execute(result.Ctx, result.Plan, *chatReq, reqID)
	if err != nil {
		writeExecutionError(c, err)
		return
	}

	response := services.ChatCompletionToResponse(&execResult.Response)

	writeResultHeaders(c, execResult, result.LogicalModel)
	c.JSON(http.StatusOK, response)
}

func (h *ResponsesHandler) handleStream(c *gin.Context, ctx context.Context, req types.ResponseRequest, reqID string, chatReq types.ChatCompletionRequest, plan types.RoutingPlan, logicalModel *types.LogicalModelConfig) {
	writeStreamHeaders(c)

	streamResult := h.pipeline.router.ExecuteStream(ctx, plan, chatReq, reqID)

	accumulator := newStreamResponseAccumulator()
	for chunk := range streamResult.Chunks {
		accumulator.Add(chunk)
		if err := writeSSEChunk(c, chunk); err != nil {
			continue
		}
	}

	if err := <-streamResult.Err; err != nil {
		writeSSEError(c, err)
	} else {
		if accumulator.HasData() {
			response := accumulator.Response()
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

type streamResponseAccumulator struct {
	id        string
	createdAt int64
	model     string
	usage     *types.Usage
	text      strings.Builder
	toolCalls map[int]*types.ResponseItem
	indexes   map[int]struct{}
}

func newStreamResponseAccumulator() *streamResponseAccumulator {
	return &streamResponseAccumulator{
		toolCalls: make(map[int]*types.ResponseItem),
		indexes:   make(map[int]struct{}),
	}
}

func (a *streamResponseAccumulator) Add(chunk *types.SSEChunk) {
	if chunk == nil {
		return
	}

	if chunk.ID != "" {
		a.id = chunk.ID
	}
	if chunk.Created != 0 {
		a.createdAt = chunk.Created
	}
	if chunk.Model != "" {
		a.model = chunk.Model
	}
	if chunk.Usage != nil {
		a.usage = chunk.Usage
	}

	if len(chunk.Choices) == 0 {
		return
	}

	choice := chunk.Choices[0]
	if choice.Delta.Content != nil {
		a.text.WriteString(*choice.Delta.Content)
	}

	for _, tc := range choice.Delta.ToolCalls {
		item, ok := a.toolCalls[tc.Index]
		if !ok {
			callID := tc.ID
			if callID == "" {
				callID = "call_" + a.id
			}
			item = &types.ResponseItem{
				ID:     fmt.Sprintf("fc_%s_%d", a.id, tc.Index),
				Type:   "function_call",
				CallID: callID,
				Status: "completed",
			}
			a.toolCalls[tc.Index] = item
			a.indexes[tc.Index] = struct{}{}
		}

		if tc.ID != "" {
			item.CallID = tc.ID
		}

		if tc.Function != nil {
			if tc.Function.Name != nil {
				item.Name += *tc.Function.Name
			}
			if tc.Function.Arguments != nil {
				item.Arguments += *tc.Function.Arguments
			}
		}
	}
}

func (a *streamResponseAccumulator) HasData() bool {
	return a.id != "" || a.text.Len() > 0 || len(a.toolCalls) > 0
}

func (a *streamResponseAccumulator) Response() *types.Response {
	output := make([]types.ResponseItem, 0, 1+len(a.toolCalls))
	outputText := a.text.String()

	if outputText != "" {
		output = append(output, types.ResponseItem{
			ID:     a.id,
			Type:   "message",
			Role:   "assistant",
			Status: "completed",
			Content: []types.ContentOutput{{
				Type: "output_text",
				Text: outputText,
			}},
		})
	}

	indexes := make([]int, 0, len(a.indexes))
	for index := range a.indexes {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		output = append(output, *a.toolCalls[index])
	}

	return &types.Response{
		ID:         "resp_" + a.id,
		Object:     "response",
		CreatedAt:  a.createdAt,
		Model:      a.model,
		Output:     output,
		OutputText: outputText,
		Usage:      a.usage,
		Status:     "completed",
	}
}
