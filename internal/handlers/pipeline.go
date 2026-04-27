package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

type Pipeline struct {
	router services.RouterHandler
}

func NewPipeline(router services.RouterHandler) *Pipeline {
	return &Pipeline{router: router}
}

type RouteResult struct {
	Tier         *types.TierConfig
	Plan         types.RoutingPlan
	Requirements types.DerivedRequirements
	Ctx          context.Context
}

func (p *Pipeline) Route(ctx context.Context, model string, hints *types.RouterHints, req types.ChatCompletionRequest) (*RouteResult, error) {
	var tierConfig *types.TierConfig
	if model != "" {
		tierConfig = resolveTier(model)
	}

	ctx = metrics.SetTier(ctx, resolveMetricTier(model, tierConfig))
	if hints != nil && hints.Strategy != nil {
		ctx = metrics.SetStrategy(ctx, *hints.Strategy)
	} else {
		ctx = metrics.SetStrategy(ctx, "default")
	}

	requirements := p.router.DeriveRequirements(req, hints)

	var candidates []types.RoutingCandidate
	if tierConfig != nil {
		candidates = p.router.GenerateCandidatesForTier(tierConfig.Tier)
	} else {
		candidates = p.router.GenerateCandidates()
		if model != "" {
			candidates = filterCandidatesByModel(candidates, model)
		}
	}

	eligible, filtered := p.router.FilterCandidates(ctx, candidates, requirements, req, hints)
	if len(eligible) == 0 {
		env := config.GetEnv()
		if env.Environment != "production" {
			logger.Debug().
				Str("event", "routing.no_eligible_provider").
				Str("model", model).
				Interface("requirements", requirements).
				Interface("response_format", req.ResponseFormat).
				Interface("filtered_providers", filtered).
				Int("candidate_count", len(candidates)).
				Msg("No eligible provider found - debug details")
		}
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

	var slo *types.TierSLO
	if tierConfig != nil {
		slo = tierConfig.SLO
	}

	plan := p.router.CompilePlan(scored, hints, slo)

	return &RouteResult{
		Tier:         tierConfig,
		Plan:         plan,
		Requirements: requirements,
		Ctx:          ctx,
	}, nil
}

func resolveTier(model string) *types.TierConfig {
	if model == "" {
		return nil
	}
	tier := types.Tier(model)
	if !tier.IsValid() {
		return nil
	}
	return config.GetTierConfig(tier)
}

func resolveMetricTier(model string, tierConfig *types.TierConfig) string {
	if tierConfig != nil {
		return string(tierConfig.Tier)
	}
	if model != "" {
		return "direct"
	}
	return "unknown"
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
	if gatewayErr.RequestID == "" {
		gatewayErr.RequestID = requestid.Get(c)
	}

	status := http.StatusInternalServerError
	switch gatewayErr.Code {
	case "RATE_LIMITED":
		status = http.StatusTooManyRequests
	case "QUOTA_EXHAUSTED":
		status = http.StatusTooManyRequests
	case "NO_ELIGIBLE_PROVIDER":
		status = http.StatusUnprocessableEntity
	case "ALL_ATTEMPTS_FAILED":
		status = http.StatusBadGateway
	case "TIMEOUT":
		status = http.StatusGatewayTimeout
	case "PROVIDER_OVERLOADED":
		status = http.StatusServiceUnavailable
	case "CIRCUIT_BREAKER_OPEN":
		status = http.StatusServiceUnavailable
	case "NETWORK_ERROR":
		status = http.StatusBadGateway
	case "PARSE_ERROR", "EMPTY_RESPONSE":
		status = http.StatusBadGateway
	case "PAYMENT_REQUIRED":
		status = http.StatusPaymentRequired
	case "VALIDATION_ERROR":
		status = http.StatusBadRequest
	}

	c.JSON(status, gin.H{"error": gatewayErr})
}

func writeResultHeaders(c *gin.Context, result *types.ExecutionResult, tierConfig *types.TierConfig) {
	c.Header("X-Gateway-Provider", result.ProviderID)
	c.Header("X-Gateway-Model", result.Model)
	if tierConfig != nil {
		c.Header("X-Gateway-Tier", string(tierConfig.Tier))
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
	// First emit an OpenAI-compatible error chunk with finish_reason: "error"
	errorChunk := types.SSEChunk{
		Object: "chat.completion.chunk",
		Choices: []types.DeltaChoice{{
			Index:        0,
			Delta:        types.DeltaMessage{},
			FinishReason: ptrString("error"),
		}},
	}
	chunkJSON, _ := json.Marshal(errorChunk)
	fmt.Fprintf(c.Writer, "data: %s\n\n", chunkJSON)

	// Then emit the gateway error details as a separate event
	errJSON, _ := json.Marshal(err)
	fmt.Fprintf(c.Writer, "data: %s\n\n", errJSON)
	c.Writer.Flush()
}

func writeSSEDone(c *gin.Context) {
	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()
}

func ptrString(s string) *string {
	return &s
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
