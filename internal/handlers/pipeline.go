package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-gonic/gin"
)

type Pipeline struct {
	router services.RouterHandler
}

func NewPipeline(router services.RouterHandler) *Pipeline {
	return &Pipeline{router: router}
}

type RouteResult struct {
	LogicalModel   *types.LogicalModelConfig
	LogicalModelID string
	Plan           types.RoutingPlan
	Requirements   types.DerivedRequirements
}

func (p *Pipeline) Route(ctx context.Context, model string, hints *types.RouterHints, req types.ChatCompletionRequest) (*RouteResult, error) {
	var logicalModel *types.LogicalModelConfig
	var logicalModelID string
	if model != "" {
		logicalModel, logicalModelID = resolveLogicalModel(model)
	}

	ctx = metrics.SetLogicalModel(ctx, logicalModelID)
	if hints != nil && hints.Profile != nil {
		ctx = metrics.SetRouterProfile(ctx, *hints.Profile)
	} else {
		ctx = metrics.SetRouterProfile(ctx, "default")
	}

	requirements := p.router.DeriveRequirements(req, hints)

	var candidates []types.RoutingCandidate
	if logicalModel != nil {
		candidates = p.router.GenerateCandidatesFromLogicalModel(logicalModel)
	} else {
		candidates = p.router.GenerateCandidates()
	}

	eligible, filtered := p.router.FilterCandidates(ctx, candidates, requirements, req, hints)
	if len(eligible) == 0 {
		return nil, &types.GatewayError{
			Type:    "gateway_error",
			Code:    "NO_ELIGIBLE_PROVIDER",
			Message: "No eligible provider found",
			Details: map[string]any{
				"requirements":       requirements,
				"filtered_providers": filtered,
			},
		}
	}

	scored := p.router.ScoreCandidates(ctx, eligible, hints)

	var slo *types.LogicalModelSLO
	if logicalModel != nil {
		slo = logicalModel.SLO
	}

	plan := p.router.CompilePlan(scored, hints, slo)

	return &RouteResult{
		LogicalModel:   logicalModel,
		LogicalModelID: logicalModelID,
		Plan:           plan,
		Requirements:   requirements,
	}, nil
}

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

func writeStreamHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()
}

func writeSSEChunk(c *gin.Context, chunk *types.SSEChunk) error {
	chunkJSON, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.Writer, "data: %s\n\n", chunkJSON)
	c.Writer.Flush()
	return nil
}

func writeSSEError(c *gin.Context, err *types.GatewayError) {
	errJSON, _ := json.Marshal(err)
	fmt.Fprintf(c.Writer, "data: %s\n\n", errJSON)
	c.Writer.Flush()
}

func writeSSEDone(c *gin.Context) {
	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()
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
