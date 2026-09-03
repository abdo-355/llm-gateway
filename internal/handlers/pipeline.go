package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
)

// sseKeepAliveInterval paces ": ping" comment frames emitted while the
// upstream is silent — failover chains and reasoning models routinely pause
// for tens of seconds. Well below typical proxy/client idle timeouts.
const sseKeepAliveInterval = 10 * time.Second

func resetKeepAliveTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(sseKeepAliveInterval)
}

// pumpStreamToClient forwards chunks to the client with SSE comment pings
// during upstream silence. Single-writer by design: both chunk frames and
// pings are emitted from this goroutine only. Returns the terminal gateway
// error; nil signals a clean stream end. ensureHeaders is lazily invoked before
// the first byte (chunk or keep-alive ping) is flushed to the client.
func pumpStreamToClient(c *gin.Context, result types.StreamResult, ensureHeaders func(), onChunk func(*types.SSEChunk) error) (*types.GatewayError, error) {
	timer := time.NewTimer(sseKeepAliveInterval)
	defer timer.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return nil, c.Request.Context().Err()
		case chunk, ok := <-result.Chunks:
			if !ok {
				return <-result.Err, nil
			}
			resetKeepAliveTimer(timer)
			if ensureHeaders != nil {
				ensureHeaders()
			}
			if err := onChunk(chunk); err != nil {
				return nil, err
			}
		case <-timer.C:
			if ensureHeaders != nil {
				ensureHeaders()
			}
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return nil, err
			}
			c.Writer.Flush()
			timer.Reset(sseKeepAliveInterval)
		}
	}
}

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

func (p *Pipeline) Route(ctx context.Context, model string, hints *types.RouterHints, req types.ChatCompletionRequest, requestID string) (*RouteResult, error) {
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
				"reason_summary":     summarizeFilteredProviders(filtered),
			},
		}
	}

	scored := p.router.ScoreCandidates(ctx, eligible, requirements, hints)

	var slo *types.TierSLO
	if tierConfig != nil {
		slo = tierConfig.SLO
	}

	plan := p.router.CompilePlan(scored, hints, slo)

	tierName := "unknown"
	if tierConfig != nil {
		tierName = string(tierConfig.Tier)
	}

	filteredSummary := make(map[string]int)
	for _, reason := range filtered {
		filteredSummary[reason]++
	}
	retryPolicy := map[string]bool{
		"on_429":     plan.RetryOn429,
		"on_timeout": plan.RetryOnTimeout,
		"on_5xx":     plan.RetryOn5xx,
	}

	logger.Info().
		Str("type", "router").
		Str("event", "tier.resolved").
		Str("request_id", requestID).
		Str("model", model).
		Str("tier", tierName).
		Interface("requirements", requirements).
		Int("candidate_count", len(candidates)).
		Int("eligible_count", len(eligible)).
		Int("attempt_count", len(plan.Attempts)).
		Int("max_attempts", plan.MaxAttempts).
		Interface("retry_policy", retryPolicy).
		Interface("top_attempts", summarizePlanAttempts(plan, 5)).
		Interface("filtered_summary", filteredSummary).
		Msg("Tier resolution complete")

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
	if err == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"type":    "gateway_error",
				"code":    "EXECUTION_ERROR",
				"message": "Execution failed with unspecified error",
			},
		})
		return
	}
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
	case "TIMEOUT", "HARD_TIMEOUT":
		status = http.StatusGatewayTimeout
	case "PROVIDER_OVERLOADED":
		status = http.StatusServiceUnavailable
	case "CIRCUIT_BREAKER_OPEN":
		status = http.StatusServiceUnavailable
	case "NETWORK_ERROR":
		status = http.StatusBadGateway
	case "PROVIDER_ERROR":
		status = http.StatusBadGateway
	case "PARSE_ERROR", "EMPTY_RESPONSE":
		status = http.StatusBadGateway
	case "PAYMENT_REQUIRED":
		status = http.StatusPaymentRequired
	case "VALIDATION_ERROR":
		status = http.StatusBadRequest
	}

	if retryAfter, ok := gatewayErr.Details["retry_after"].(int); ok && retryAfter > 0 {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
	}

	logger.Warn().
		Str("type", "http").
		Str("event", "request.execution_error").
		Str("request_id", gatewayErr.RequestID).
		Str("error_code", gatewayErr.Code).
		Int("status", status).
		Str("error_message", gatewayErr.Message).
		Msg("Request execution error")

	c.JSON(status, gin.H{"error": publicGatewayError(gatewayErr)})
}

func publicGatewayError(err *types.GatewayError) *types.GatewayError {
	if err == nil {
		return nil
	}
	return &types.GatewayError{
		Type:      err.Type,
		Code:      err.Code,
		Message:   err.Message,
		RequestID: err.RequestID,
		Details:   publicGatewayErrorDetails(err),
	}
}

func publicGatewayErrorDetails(err *types.GatewayError) map[string]any {
	if err == nil || len(err.Details) == 0 {
		return nil
	}
	public := make(map[string]any)
	if retryAfter, ok := err.Details["retry_after"]; ok {
		public["retry_after"] = retryAfter
	}
	if err.Code == "NO_ELIGIBLE_PROVIDER" {
		if summary, ok := err.Details["reason_summary"]; ok {
			public["reason_summary"] = summary
		}
	}
	if len(public) == 0 {
		return nil
	}
	return public
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
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", chunkJSON); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func writeSSEError(c *gin.Context, err *types.GatewayError) {
	reqID := ""
	if err != nil {
		reqID = err.RequestID
	}
	if reqID == "" {
		reqID = requestid.Get(c)
	}

	// First emit an OpenAI-compatible error chunk with finish_reason: "error"
	errorChunk := types.SSEChunk{
		ID:      "chatcmpl-" + reqID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   "gateway-error",
		Choices: []types.DeltaChoice{{
			Index:        0,
			Delta:        types.DeltaMessage{},
			FinishReason: ptrString("error"),
		}},
	}
	chunkJSON, _ := json.Marshal(errorChunk)
	fmt.Fprintf(c.Writer, "data: %s\n\n", chunkJSON)

	// Then emit the gateway error details as a typed event with the same envelope
	// shape used by non-streaming responses.
	errJSON, _ := json.Marshal(gin.H{"error": publicGatewayError(err)})
	fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", errJSON)
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

type filterReasonSummary struct {
	Category          string `json:"category"`
	Reason            string `json:"reason"`
	Count             int    `json:"count"`
	Retryable         bool   `json:"retryable"`
	RetryAfterSeconds *int   `json:"retry_after_seconds,omitempty"`
}

type routingAttemptLogSummary struct {
	Attempt    int     `json:"attempt"`
	ProviderID string  `json:"provider_id"`
	Model      string  `json:"model"`
	Score      float64 `json:"score"`
	TimeoutMs  int     `json:"timeout_ms"`
}

func summarizeFilteredProviders(filtered map[string]string) []filterReasonSummary {
	if len(filtered) == 0 {
		return nil
	}

	byReason := make(map[string]filterReasonSummary)
	for _, reason := range filtered {
		category, retryable, retryAfter := classifyFilterReason(reason)
		summary := byReason[reason]
		if summary.Reason == "" {
			summary = filterReasonSummary{
				Category:          category,
				Reason:            reason,
				Retryable:         retryable,
				RetryAfterSeconds: retryAfter,
			}
		}
		summary.Count++
		byReason[reason] = summary
	}

	summaries := make([]filterReasonSummary, 0, len(byReason))
	for _, summary := range byReason {
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Category == summaries[j].Category {
			return summaries[i].Reason < summaries[j].Reason
		}
		return summaries[i].Category < summaries[j].Category
	})
	return summaries
}

func summarizePlanAttempts(plan types.RoutingPlan, limit int) []routingAttemptLogSummary {
	if len(plan.Attempts) == 0 || limit <= 0 {
		return nil
	}
	if limit > len(plan.Attempts) {
		limit = len(plan.Attempts)
	}

	summary := make([]routingAttemptLogSummary, 0, limit)
	for i := 0; i < limit; i++ {
		attempt := plan.Attempts[i]
		summary = append(summary, routingAttemptLogSummary{
			Attempt:    i + 1,
			ProviderID: attempt.ProviderID,
			Model:      attempt.Model,
			Score:      attempt.Score,
			TimeoutMs:  attempt.TimeoutMs,
		})
	}
	return summary
}

func classifyFilterReason(reason string) (string, bool, *int) {
	if strings.HasPrefix(reason, "provider_cooldown_active:") {
		parts := strings.Split(reason, ":")
		if len(parts) >= 3 {
			secondsText := strings.TrimSuffix(parts[2], "s")
			if seconds, err := strconv.Atoi(secondsText); err == nil && seconds > 0 {
				return "cooldown", true, &seconds
			}
		}
		return "cooldown", true, nil
	}

	switch {
	case strings.Contains(reason, "quota_exceeded"):
		return "quota", false, nil
	case strings.Contains(reason, "concurrency_limit"):
		return "capacity", true, nil
	case strings.Contains(reason, "circuit_breaker"):
		return "health", true, nil
	case strings.Contains(reason, "not_supported") || strings.Contains(reason, "not_certified"):
		return "capability", false, nil
	case strings.Contains(reason, "allowlist") || strings.Contains(reason, "denylist"):
		return "routing_policy", false, nil
	case strings.Contains(reason, "provider_unavailable"):
		return "configuration", false, nil
	default:
		return "other", false, nil
	}
}
