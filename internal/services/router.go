package services

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/abdo-355/llm-gateway/internal/types"
)

const providerQuotaScopeModel = "__provider__"

type Router struct {
	config           types.AppConfig
	quotaService     QuotaChecker
	healthService    HealthChecker
	providerService  ProviderCaller
	classifier       FailureClassifier
	backoffStrategy  BackoffStrategy
	cooldownService  *CooldownService
	providerDisabler *ProviderDisabler
}

func NewRouter(
	quotaSvc QuotaChecker,
	healthSvc HealthChecker,
	providerSvc ProviderCaller,
) *Router {
	cfg := config.LoadConfig()
	metrics.RegisterModelInfo(cfg)
	return &Router{
		config:           cfg,
		quotaService:     quotaSvc,
		healthService:    healthSvc,
		providerService:  providerSvc,
		classifier:       NewDefaultFailureClassifier(),
		backoffStrategy:  DefaultBackoffStrategy(),
		cooldownService:  nil,
		providerDisabler: NewDefaultProviderDisabler(),
	}
}

func NewRouterWithConfig(
	cfg types.AppConfig,
	quotaSvc QuotaChecker,
	healthSvc HealthChecker,
	providerSvc ProviderCaller,
) *Router {
	metrics.RegisterModelInfo(cfg)
	return &Router{
		config:           cfg,
		quotaService:     quotaSvc,
		healthService:    healthSvc,
		providerService:  providerSvc,
		classifier:       NewDefaultFailureClassifier(),
		backoffStrategy:  DefaultBackoffStrategy(),
		cooldownService:  nil,
		providerDisabler: NewDefaultProviderDisabler(),
	}
}

// SetCooldownService sets the cooldown service (called after construction)
func (r *Router) SetCooldownService(cs *CooldownService) {
	r.cooldownService = cs
}

func (r *Router) SetProviderDisabler(disabler *ProviderDisabler) {
	r.providerDisabler = disabler
}

// DeriveRequirements normalizes the raw request fields (response_format, stream, tools, tool_choice)
// into a uniform set of requirement categories (output, streaming, tools) so that downstream stages
// can filter and score providers generically without re-inspecting every request field combination.
// Router hints can override the auto-detected values.
func (r *Router) DeriveRequirements(req types.ChatCompletionRequest, hints *types.RouterHints) types.DerivedRequirements {
	requirements := types.DerivedRequirements{
		Output:    "text",
		Streaming: "preferred",
		Tools:     "forbidden",
		Reasoning: "preferred",
	}

	// Detect structured output requirements.
	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case "json_object":
			requirements.Output = "json_object"
		case "json_schema":
			requirements.Output = "json_schema"
			if req.ResponseFormat.JSONSchema != nil &&
				req.ResponseFormat.JSONSchema.Strict != nil &&
				*req.ResponseFormat.JSONSchema.Strict {
				requirements.Output = "json_schema_strict"
			}
		}
	}

	// Detect streaming
	if req.Stream != nil {
		if *req.Stream {
			requirements.Streaming = "required"
		} else {
			requirements.Streaming = "forbidden"
		}
	}

	// Detect tools
	if len(req.Tools) > 0 {
		switch tc := req.ToolChoice.(type) {
		case string:
			switch tc {
			case "required":
				requirements.Tools = "required"
			case "none":
				requirements.Tools = "forbidden"
			default: // "auto" or any other string
				requirements.Tools = "allowed"
			}
		case map[string]any:
			// Object form like {"type": "function", "function": {"name": "..."}}
			// implies a specific tool is required
			requirements.Tools = "required"
		default:
			requirements.Tools = "allowed"
		}
	}

	// Detect reasoning: any reasoning param makes it preferred (drop on
	// unsupported targets); the required hint opts into candidate filtering.
	if resolvedReasoning := NormalizeReasoningParams(req); resolvedReasoning.Present {
		requirements.Reasoning = "preferred"
		if !resolvedReasoning.Disabled {
			requirements.ReasoningLevel = resolvedReasoning.Level
		}
	} else {
		requirements.Reasoning = "forbidden"
	}

	// Router hints override
	if hints != nil && hints.Requirements != nil {
		if hints.Requirements.Output != nil {
			requirements.Output = *hints.Requirements.Output
		}
		if hints.Requirements.Streaming != nil {
			requirements.Streaming = *hints.Requirements.Streaming
		}
		if hints.Requirements.Tools != nil {
			requirements.Tools = *hints.Requirements.Tools
		}
		if hints.Requirements.Reasoning != nil {
			requirements.Reasoning = *hints.Requirements.Reasoning
		}
	}

	return requirements
}

// Stage 2: Generate Candidates
func (r *Router) GenerateCandidates() []types.RoutingCandidate {
	var candidates []types.RoutingCandidate

	for _, provider := range r.config.Providers {
		for _, model := range provider.Models.List {
			isCertified := r.isCertifiedForStrictSchema(provider.ID, model)

			candidates = append(candidates, types.RoutingCandidate{
				Provider:                   provider,
				Model:                      model,
				IsCertifiedForStrictSchema: isCertified,
				Score:                      0,
				ScoreBreakdown:             make(map[string]float64),
			})
		}
	}

	return candidates
}

func (r *Router) GenerateCandidatesForTier(tier types.Tier) []types.RoutingCandidate {
	var candidates []types.RoutingCandidate

	tierConfig := config.GetTierConfig(tier)
	if tierConfig == nil {
		return candidates
	}

	providerMap := make(map[string]types.ProviderConfig)
	for _, p := range r.config.Providers {
		providerMap[p.ID] = p
	}

	for _, entry := range tierConfig.Entries {
		provider, ok := providerMap[entry.Provider]
		if !ok {
			logger.Warn().
				Str("type", "router").
				Str("event", "tier.provider_missing").
				Str("provider", entry.Provider).
				Msg("Provider not found for tier entry")
			continue
		}

		found := slices.Contains(provider.Models.List, entry.Model)
		if !found {
			logger.Warn().
				Str("type", "router").
				Str("event", "tier.model_missing").
				Str("provider", entry.Provider).
				Str("model", entry.Model).
				Msg("Model not in provider allowlist")
			continue
		}

		isCertified := r.isCertifiedForStrictSchema(provider.ID, entry.Model)
		candidates = append(candidates, types.RoutingCandidate{
			Provider:                   provider,
			Model:                      entry.Model,
			IsCertifiedForStrictSchema: isCertified,
			Score:                      entry.Weight,
			ScoreBreakdown:             make(map[string]float64),
		})
	}

	return candidates
}

// Stage 3: Filter Candidates
func (r *Router) FilterCandidates(
	ctx context.Context,
	candidates []types.RoutingCandidate,
	requirements types.DerivedRequirements,
	req types.ChatCompletionRequest,
	hints *types.RouterHints,
) ([]types.RoutingCandidate, map[string]string) {
	var eligible []types.RoutingCandidate
	filtered := make(map[string]string)
	estimatedTokens := r.quotaService.EstimateTokens(req)
	cloudflareBudget, hasCloudflareBudget := r.quotaService.(CloudflareBudgetManager)

	// Phase 1: Non-Redis capability checks. Collect candidates that need Redis checks.
	type redisCandidate struct {
		candidate   types.RoutingCandidate
		caps        types.ProviderCapabilities
		modelLimits types.ModelLimits
	}
	var redisCandidates []redisCandidate

	for _, candidate := range candidates {
		provider := candidate.Provider
		model := candidate.Model
		caps := r.resolveCapabilities(provider, model)

		// Check allow/deny lists
		if hints != nil && hints.Providers != nil {
			if len(hints.Providers.Allow) > 0 {
				found := slices.Contains(hints.Providers.Allow, provider.ID)
				if !found {
					filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "provider_not_in_allowlist"
					continue
				}
			}

			if slices.Contains(hints.Providers.Deny, provider.ID) {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "provider_in_denylist"
				continue
			}
		}

		if !r.providerAvailable(provider) {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "provider_unavailable"
			continue
		}

		if !supportsStructuredOutput(requirements, caps, candidate.IsCertifiedForStrictSchema) {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = structuredOutputFilterReason(requirements, caps, candidate.IsCertifiedForStrictSchema)
			continue
		}

		if requirements.Streaming == "required" && !caps.Streaming {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "streaming_not_supported"
			continue
		}

		if len(req.Tools) > 0 {
			if !caps.Tools {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "tools_not_supported"
				continue
			}
			if caps.ToolSchema != "" && caps.ToolSchema != "json_schema" {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "tool_schema_dialect_not_supported"
				continue
			}
		}

		if requirements.Tools == "required" && !caps.Tools {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "tools_not_supported"
			continue
		}

		if requirements.Reasoning == "required" {
			if !SupportsReasoningLevel(caps, requirements.ReasoningLevel) {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "reasoning_not_supported"
				continue
			}
		}

		if req.Logprobs != nil && *req.Logprobs && !caps.Logprobs {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "logprobs_not_supported"
			continue
		}

		if req.N != nil && *req.N > 1 && !caps.MultipleChoices {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "multiple_choices_not_supported"
			continue
		}

		redisCandidates = append(redisCandidates, redisCandidate{
			candidate:   candidate,
			caps:        caps,
			modelLimits: effectiveModelLimits(provider, model),
		})
	}

	// Phase 2: Batch Redis checks for surviving candidates.
	pairs := make([]ProviderModelPair, len(redisCandidates))
	for i, rc := range redisCandidates {
		pairs[i] = ProviderModelPair{ProviderID: rc.candidate.Provider.ID, Model: rc.candidate.Model}
	}

	// Batch circuit state check.
	var circuitStates map[string]CircuitState
	if hs, ok := r.healthService.(interface {
		BatchCircuitStates(context.Context, []ProviderModelPair) map[string]CircuitState
	}); ok {
		circuitStates = hs.BatchCircuitStates(ctx, pairs)
	}

	// Batch cooldown check.
	var cooldownStates map[string]bool
	if r.cooldownService != nil {
		cooldownStates = r.cooldownService.BatchIsOnCooldown(ctx, pairs)
	}

	// Phase 3: Use cached Redis results and perform remaining checks.
	for _, rc := range redisCandidates {
		candidate := rc.candidate
		provider := candidate.Provider
		model := candidate.Model
		key := provider.ID + "/" + model

		// Circuit breaker: use batch result as fast path for CLOSED,
		// fall back to CanExecute for non-CLOSED (handles OPEN→HALF_OPEN recovery).
		if circuitStates != nil {
			if circuitStates[key] != StateClosed {
				if !r.healthService.CanExecute(ctx, provider.ID, model) {
					filtered[key] = "circuit_breaker_open"
					continue
				}
			}
		} else if !r.healthService.CanExecute(ctx, provider.ID, model) {
			filtered[key] = "circuit_breaker_open"
			continue
		}

		// Cooldown (try cache, fall back to individual call)
		if cooldownStates != nil {
			if cooldownStates[key] {
				reason := r.cooldownService.GetCooldownReason(ctx, provider.ID, model)
				remaining := r.cooldownService.GetCooldownRemaining(ctx, provider.ID, model)
				filtered[key] = fmt.Sprintf("provider_cooldown_active:%s:%.0fs", reason, remaining.Seconds())
				continue
			}
		} else if r.cooldownService != nil && r.cooldownService.IsOnCooldown(ctx, provider.ID, model) {
			reason := r.cooldownService.GetCooldownReason(ctx, provider.ID, model)
			remaining := r.cooldownService.GetCooldownRemaining(ctx, provider.ID, model)
			filtered[key] = fmt.Sprintf("provider_cooldown_active:%s:%.0fs", reason, remaining.Seconds())
			continue
		}

		// Check per-model quota
		modelLimits := rc.modelLimits

		if provider.ID == cloudflareProviderID && hasCloudflareBudget {
			estimatedNeurons := cloudflareBudget.EstimateCloudflareRequestNeurons(model, req)
			if err := cloudflareBudget.CheckCloudflareDailyNeuronBudget(ctx, model, estimatedNeurons); err != nil {
				if quotaErr, ok := err.(*errors.ModelQuotaExceededError); ok {
					filtered[key] = fmt.Sprintf("quota_exceeded_%s", quotaErr.LimitType)
				} else {
					filtered[key] = "cloudflare_budget_check_failed"
				}
				continue
			}
		}

		if err := r.quotaService.CheckModelQuota(ctx, provider.ID, model, modelLimits, estimatedTokens); err != nil {
			if quotaErr, ok := err.(*errors.ModelQuotaExceededError); ok {
				filtered[key] = fmt.Sprintf("quota_exceeded_%s", quotaErr.LimitType)
			} else {
				filtered[key] = "quota_check_failed"
			}
			continue
		}

		if providerLimits := providerLevelModelLimits(provider.Limits); hasModelLimits(providerLimits) {
			if err := r.quotaService.CheckModelQuota(ctx, provider.ID, providerQuotaScopeModel, providerLimits, estimatedTokens); err != nil {
				if quotaErr, ok := err.(*errors.ModelQuotaExceededError); ok {
					filtered[key] = fmt.Sprintf("quota_exceeded_provider_%s", quotaErr.LimitType)
				} else {
					filtered[key] = "provider_quota_check_failed"
				}
				continue
			}
		}

		if modelLimits.MaxConcurrent != nil && *modelLimits.MaxConcurrent > 0 {
			if !r.quotaService.CheckConcurrencyLimit(ctx, provider.ID, model, *modelLimits.MaxConcurrent) {
				filtered[key] = "concurrency_limit_exceeded"
				continue
			}
		}

		eligible = append(eligible, candidate)
	}

	return eligible, filtered
}

// Stage 4: Score Candidates
func (r *Router) ScoreCandidates(ctx context.Context, candidates []types.RoutingCandidate, requirements types.DerivedRequirements, hints *types.RouterHints) []types.RoutingCandidate {
	// Batch-read health metrics for all candidates in one pipeline.
	type batchHealthResult struct {
		healthScore       float64
		successRatioScore float64
	}
	batchResults := make([]batchHealthResult, len(candidates))

	if hs, ok := r.healthService.(interface {
		BatchGetHealthMetrics(context.Context, []ProviderModelPair) []HealthMetrics
	}); ok {
		pairs := make([]ProviderModelPair, len(candidates))
		for i, c := range candidates {
			pairs[i] = ProviderModelPair{ProviderID: c.Provider.ID, Model: c.Model}
		}
		metricsList := hs.BatchGetHealthMetrics(ctx, pairs)
		for i, m := range metricsList {
			batchResults[i] = batchHealthResult{
				healthScore:       m.HealthScore,
				successRatioScore: calculateSuccessRatioScore(m.SuccessCount, m.FailureCount),
			}
		}
	} else {
		for i, c := range candidates {
			m := r.healthService.GetHealthMetrics(ctx, c.Provider.ID, c.Model)
			batchResults[i] = batchHealthResult{
				healthScore:       m.HealthScore,
				successRatioScore: calculateSuccessRatioScore(m.SuccessCount, m.FailureCount),
			}
		}
	}

	for i := range candidates {
		candidate := &candidates[i]
		baseScore := 1.0
		if candidate.ScoreBreakdown == nil {
			candidate.ScoreBreakdown = make(map[string]float64)
		}

		// Preference bonus
		if hints != nil && hints.Providers != nil {
			for j, pref := range hints.Providers.Prefer {
				if pref == candidate.Provider.ID {
					bonus := 0.5 * (1.0 - float64(j)/float64(len(hints.Providers.Prefer)))
					baseScore += bonus
					candidate.ScoreBreakdown["preference_bonus"] = bonus
					break
				}
			}
		}

		// Health score (from batch)
		healthScore := batchResults[i].healthScore
		successRatioScore := batchResults[i].successRatioScore
		candidate.ScoreBreakdown["health_score"] = healthScore
		candidate.ScoreBreakdown["success_ratio"] = successRatioScore
		concurrencyPenalty := r.concurrencyLoadPenalty(ctx, *candidate)
		if concurrencyPenalty > 0 {
			candidate.ScoreBreakdown["concurrency_load_penalty"] = concurrencyPenalty
		}
		structuredOutputAdjustment := strictStructuredOutputScoreAdjustment(requirements, *candidate, r.resolveCapabilities(candidate.Provider, candidate.Model))
		if structuredOutputAdjustment > 0 {
			candidate.ScoreBreakdown["strict_structured_output_bonus"] = structuredOutputAdjustment
		} else if structuredOutputAdjustment < 0 {
			candidate.ScoreBreakdown["strict_structured_output_penalty"] = -structuredOutputAdjustment
		}

		reasoningBonus := reasoningScoreAdjustment(requirements, r.resolveCapabilities(candidate.Provider, candidate.Model))
		if reasoningBonus > 0 {
			candidate.ScoreBreakdown["reasoning_bonus"] = reasoningBonus
		}

		// Combine scores
		candidate.Score = baseScore*0.5 + healthScore*0.5 + successRatioScore + candidate.Score + structuredOutputAdjustment + reasoningBonus - concurrencyPenalty
	}

	slices.SortFunc(candidates, func(a, b types.RoutingCandidate) int {
		if a.Score > b.Score {
			return -1
		}
		if a.Score < b.Score {
			return 1
		}
		return 0
	})

	return candidates
}

func strictStructuredOutputScoreAdjustment(requirements types.DerivedRequirements, candidate types.RoutingCandidate, caps types.ProviderCapabilities) float64 {
	if requirements.Output != "json_schema_strict" {
		return 0
	}
	if candidate.IsCertifiedForStrictSchema || caps.StructuredOutputs == "json_schema_strict" {
		return 3.00
	}
	if caps.StructuredOutputs == "json_schema" {
		return 2.00
	}
	if caps.StructuredOutputs == "model_dependent" {
		return 1.00
	}
	if caps.StructuredOutputs == "json_object" {
		return -2.00
	}
	return 0
}

// reasoningScoreAdjustment gives a small preference to candidates that can
// honor an explicit reasoning ask. It never applies when the requirement is
// forbidden or the request carries no reasoning params.
func reasoningScoreAdjustment(requirements types.DerivedRequirements, caps types.ProviderCapabilities) float64 {
	if requirements.Reasoning == "forbidden" {
		return 0
	}
	if requirements.Reasoning == "required" || requirements.ReasoningLevel != "" {
		if SupportsReasoningLevel(caps, requirements.ReasoningLevel) {
			return 0.25
		}
	}
	return 0
}

func (r *Router) concurrencyLoadPenalty(ctx context.Context, candidate types.RoutingCandidate) float64 {
	reader, ok := r.quotaService.(ConcurrencyUsageReader)
	if !ok {
		return 0
	}

	limits := effectiveModelLimits(candidate.Provider, candidate.Model)
	if limits.MaxConcurrent == nil || *limits.MaxConcurrent <= 0 {
		return 0
	}

	current, err := reader.GetConcurrencyUsage(ctx, candidate.Provider.ID, candidate.Model)
	if err != nil || current <= 0 {
		return 0
	}

	utilization := float64(current) / float64(*limits.MaxConcurrent)
	if utilization > 1 {
		utilization = 1
	}
	return utilization * 0.75
}

func calculateSuccessRatioScore(successCount, failureCount int) float64 {
	total := successCount + failureCount
	if total <= 0 {
		return 1.0
	}
	return float64(successCount) / float64(total)
}

// Stage 5: Compile Plan
func (r *Router) CompilePlan(
	candidates []types.RoutingCandidate,
	hints *types.RouterHints,
	tierSLO *types.TierSLO,
) types.RoutingPlan {
	// Determine max attempts. Client hints can cap failover breadth, otherwise
	// all eligible candidates are tried until the request timeout is reached.
	maxAttempts := len(candidates)
	if hints != nil && hints.Fallback != nil && hints.Fallback.MaxAttempts != nil {
		maxAttempts = *hints.Fallback.MaxAttempts
	}

	if maxAttempts > len(candidates) {
		maxAttempts = len(candidates)
	}

	// Determine timeout
	timeoutMs := defaultRequestTimeoutMs
	if hints != nil && hints.SLO != nil && hints.SLO.MaxLatencyMs != nil {
		timeoutMs = *hints.SLO.MaxLatencyMs
	} else if tierSLO != nil && tierSLO.MaxLatencyMs != nil {
		timeoutMs = *tierSLO.MaxLatencyMs
	}

	// Determine hard timeout
	var hardTimeoutMs *int
	if hints != nil && hints.SLO != nil && hints.SLO.HardTimeoutMs != nil {
		hardTimeoutMs = hints.SLO.HardTimeoutMs
	}

	// Build attempts
	var attempts []types.RoutingAttempt
	for i := 0; i < maxAttempts && i < len(candidates); i++ {
		candidate := candidates[i]
		apiKey := r.resolveProviderAPIKey(candidate.Provider.Auth)
		caps := r.resolveCapabilities(candidate.Provider, candidate.Model)

		// Per-model timeout overrides let slow-but-reliable models outlive the
		// tier SLO (nous ox-alpha legitimately finishes at 19-27s).
		attemptTimeoutMs := timeoutMs
		if override := candidate.Provider.Models.Limits[candidate.Model].TimeoutMs; override != nil && *override > 0 {
			attemptTimeoutMs = *override
		}

		attempts = append(attempts, types.RoutingAttempt{
			ProviderID:   candidate.Provider.ID,
			Model:        candidate.Model,
			BaseURL:      candidate.Provider.BaseURL,
			APIKey:       apiKey,
			Score:        candidate.Score,
			TimeoutMs:    attemptTimeoutMs,
			ProviderType: candidate.Provider.ProviderType,
			Auth:         candidate.Provider.Auth,
			Capabilities: caps,
		})
	}

	// Determine retry policy
	retryOn429 := true
	retryOnTimeout := true
	retryOn5xx := true

	if hints != nil && hints.Fallback != nil {
		if hints.Fallback.On429 != nil {
			retryOn429 = *hints.Fallback.On429
		}
		if hints.Fallback.OnTimeout != nil {
			retryOnTimeout = *hints.Fallback.OnTimeout
		}
		if hints.Fallback.On5xx != nil {
			retryOn5xx = *hints.Fallback.On5xx
		}
	}

	return types.RoutingPlan{
		Attempts:       attempts,
		MaxAttempts:    maxAttempts,
		HardTimeoutMs:  hardTimeoutMs,
		RetryOn429:     retryOn429,
		RetryOnTimeout: retryOnTimeout,
		RetryOn5xx:     retryOn5xx,
		RetryPolicySet: true,
	}
}

// Stage 6: Execute
func (r *Router) Execute(
	ctx context.Context,
	plan types.RoutingPlan,
	req types.ChatCompletionRequest,
	requestID string,
) (*types.ExecutionResult, error) {
	startTime := time.Now()

	tier := metrics.GetTier(ctx)
	strategy := metrics.GetStrategy(ctx)

	var previousProvider string
	var hadFailure bool
	var attemptChain []map[string]any

	for i, attempt := range plan.Attempts {
		if r.providerDisabled(attempt.ProviderID) {
			attemptChain = append(attemptChain, disabledProviderAttempt(attempt))
			continue
		}

		if quotaErr := r.checkCloudflareAttemptBudget(ctx, attempt, req); quotaErr != nil {
			logger.Warn().
				Str("type", "router").
				Str("event", "attempt.skipped").
				Str("request_id", requestID).
				Str("provider", attempt.ProviderID).
				Str("model", attempt.Model).
				Err(quotaErr).
				Msg("Skipping Cloudflare attempt because daily neuron budget is exhausted")
			continue
		}

		attemptTimeoutMs, hardTimeoutErr := boundedAttemptTimeoutMs(plan, startTime, attempt.TimeoutMs)
		if hardTimeoutErr != nil {
			return nil, hardTimeoutErr
		}

		logger.Info().
			Str("type", "router").
			Str("event", "attempt.start").
			Str("request_id", requestID).
			Int("attempt", i+1).
			Str("provider", attempt.ProviderID).
			Str("model", attempt.Model).
			Int("timeout_ms", attemptTimeoutMs).
			Float64("score", attempt.Score).
			Msg("Trying provider")

		releaseConcurrency, deniedScope, concurrencyOK := r.acquireAttemptConcurrency(ctx, attempt.ProviderID, attempt.Model)
		if !concurrencyOK {
			logger.Warn().
				Str("type", "router").
				Str("event", "attempt.concurrency_denied").
				Str("request_id", requestID).
				Str("provider", attempt.ProviderID).
				Str("model", attempt.Model).
				Str("scope", deniedScope).
				Msg("Concurrency slot unavailable")
			attemptChain = append(attemptChain, map[string]any{
				"provider":       attempt.ProviderID,
				"model":          attempt.Model,
				"failure_kind":   "concurrency_denied",
				"failure_action": string(types.ActionFailover),
				"failure_reason": "concurrency slot unavailable, trying different provider",
			})
			continue
		}

		reservation, reservationErr := r.reserveAttemptQuota(ctx, attempt, req)
		if reservationErr != nil {
			if releaseConcurrency != nil {
				releaseConcurrency()
			}
			attemptChain = append(attemptChain, quotaReservationFailureAttempt(attempt, reservationErr))
			continue
		}

		attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(attemptTimeoutMs)*time.Millisecond)

		attemptReq := req
		attemptReq.ProviderCapabilities = attempt.Capabilities

		attemptStart := time.Now()
		resp, err := r.providerService.CallProvider(
			attempt.BaseURL,
			attempt.APIKey,
			attempt.Model,
			attemptReq,
			attemptTimeoutMs,
			attemptCtx,
			attempt.ProviderType,
			attempt.Auth,
			requestID,
		)

		attemptLatencyMs := time.Since(attemptStart).Milliseconds()
		parentContextErr := ctx.Err()
		attemptContextErr := attemptCtx.Err()
		cancel()

		if releaseConcurrency != nil {
			releaseConcurrency()
		}

		totalLatencyMs := time.Since(startTime).Milliseconds()

		if err == nil {
			if validationErr := validateStructuredOutputResponse(req, resp, attempt.ProviderID, attempt.Model); validationErr != nil {
				err = validationErr
			}
		}
		if err != nil {
			r.releaseTokenReservation(ctx, reservation)
		}

		if err == nil {
			r.healthService.RecordSuccess(ctx, attempt.ProviderID, attempt.Model, int(totalLatencyMs))

			if cooldownMs := r.lookupModelCooldownMs(attempt.ProviderID, attempt.Model); cooldownMs > 0 && r.cooldownService != nil {
				cooldownSec := cooldownMs / 1000
				if cooldownSec < 1 {
					cooldownSec = 1
				}
				r.cooldownService.ApplyCooldownForReason(ctx, attempt.ProviderID, attempt.Model, "success", cooldownSec)
			}

			tokensUsed := 0
			var cloudflareStats *CloudflareUsageStats
			if resp.Usage != nil {
				tokensUsed = resp.Usage.TotalTokens
			} else {
				tokensUsed = r.quotaService.EstimateTokens(req)
			}
			if reservation != nil {
				r.recordReservedQuotaUsage(ctx, reservation, tokensUsed)
			} else {
				r.recordQuotaUsage(ctx, attempt.ProviderID, attempt.Model, tokensUsed)
			}
			if attempt.ProviderID == cloudflareProviderID {
				if budgetMgr, ok := r.quotaService.(CloudflareBudgetManager); ok && resp.Usage != nil {
					stats, quotaErr := budgetMgr.RecordCloudflareNeuronUsage(ctx, attempt.Model, resp.Usage)
					if quotaErr != nil {
						logger.Warn().
							Str("type", "router").
							Str("event", "cloudflare.neuron_record_failed").
							Str("request_id", requestID).
							Str("model", attempt.Model).
							Err(quotaErr).
							Msg("Failed to record Cloudflare neuron usage")
					} else {
						cloudflareStats = &stats
						metrics.ProviderNeuronsTotal.WithLabelValues(
							attempt.ProviderID, attempt.Model, tier, strategy,
						).Add(float64(stats.Neurons))
						metrics.ProviderEstimatedCostUSDTotal.WithLabelValues(
							attempt.ProviderID, attempt.Model, tier, strategy,
						).Add(stats.EstimatedUSDIfPaid)
					}
				}
			}

			if hadFailure {
				metrics.RetrySuccessTotal.WithLabelValues(tier).Inc()
			}

			metrics.ProviderRequestsTotal.WithLabelValues(
				attempt.ProviderID, attempt.Model, "success",
				tier, strategy, "",
			).Inc()
			metrics.ProviderLatencySeconds.WithLabelValues(
				attempt.ProviderID, attempt.Model,
				tier, strategy,
			).Observe(float64(totalLatencyMs) / 1000.0)
			if resp.Usage != nil {
				metrics.ProviderTokensTotal.WithLabelValues(
					attempt.ProviderID, attempt.Model, "prompt", tier, strategy,
				).Add(float64(resp.Usage.PromptTokens))
				metrics.ProviderTokensTotal.WithLabelValues(
					attempt.ProviderID, attempt.Model, "completion", tier, strategy,
				).Add(float64(resp.Usage.CompletionTokens))
				metrics.ProviderTokensTotal.WithLabelValues(
					attempt.ProviderID, attempt.Model, "total", tier, strategy,
				).Add(float64(resp.Usage.TotalTokens))
			}
			metrics.RoutingAttemptsTotal.WithLabelValues(
				tier, strategy,
			).Observe(float64(i + 1))

			logEvent := logger.Info().
				Str("type", "router").
				Str("event", "attempt.success").
				Str("request_id", requestID).
				Str("provider", attempt.ProviderID).
				Str("model", attempt.Model).
				Int64("latency_ms", totalLatencyMs).
				Int64("attempt_latency_ms", attemptLatencyMs).
				Int("timeout_ms", attemptTimeoutMs).
				Int("tokens", tokensUsed).
				Int("attempts", i+1)
			if resp.Usage != nil {
				logEvent = logEvent.
					Int("input_tokens", resp.Usage.PromptTokens).
					Int("output_tokens", resp.Usage.CompletionTokens)
				if resp.Usage.PromptTokensDetails != nil && resp.Usage.PromptTokensDetails.CachedTokens > 0 {
					logEvent = logEvent.Int("cached_tokens", resp.Usage.PromptTokensDetails.CachedTokens)
				}
			}
			if cloudflareStats != nil {
				logEvent = logEvent.
					Int("cloudflare_cached_input_tokens", cloudflareStats.CachedInputTokens).
					Int("cloudflare_non_cached_input_tokens", cloudflareStats.NonCachedInputTokens).
					Int("cloudflare_neurons", cloudflareStats.Neurons).
					Float64("cloudflare_estimated_usd_if_paid", cloudflareStats.EstimatedUSDIfPaid).
					Int("cloudflare_remaining_daily_neurons", cloudflareStats.RemainingDailyNeurons)
			}
			logEvent.Msg("Request completed")

			return &types.ExecutionResult{
				Response:   *resp,
				Attempts:   i + 1,
				ProviderID: attempt.ProviderID,
				Model:      attempt.Model,
				LatencyMs:  totalLatencyMs,
			}, nil
		}

		if previousProvider != "" {
			metrics.FailoverEventsTotal.WithLabelValues(
				previousProvider, attempt.ProviderID, tier,
			).Inc()
		}
		previousProvider = attempt.ProviderID
		hadFailure = true

		failureCtx := types.FailureContext{
			AttemptIndex:       i,
			MaxAttempts:        plan.MaxAttempts,
			ProviderID:         attempt.ProviderID,
			Model:              attempt.Model,
			HasRemainingBudget: true,
		}
		decision := r.classifier.Classify(err, failureCtx)

		if decision.ShouldRecordFailure {
			r.healthService.RecordFailure(ctx, attempt.ProviderID, attempt.Model)
		}

		metrics.FailureClassifiedTotal.WithLabelValues(
			attempt.ProviderID, attempt.Model,
			string(decision.Category), string(decision.Action),
		).Inc()

		attemptFailure := map[string]any{
			"provider":           attempt.ProviderID,
			"model":              attempt.Model,
			"failure_kind":       string(decision.Category),
			"failure_action":     string(decision.Action),
			"failure_reason":     decision.Reason,
			"attempt_latency_ms": attemptLatencyMs,
			"total_latency_ms":   totalLatencyMs,
			"timeout_ms":         attemptTimeoutMs,
		}
		if source := cancellationSource(parentContextErr, attemptContextErr); source != "" {
			attemptFailure["cancellation_source"] = source
			attemptFailure["parent_context_error"] = contextErrorString(parentContextErr)
			attemptFailure["attempt_context_error"] = contextErrorString(attemptContextErr)
		}
		enrichAttemptFailureDetails(attemptFailure, err)
		attemptChain = append(attemptChain, attemptFailure)

		var status string
		var errorType string
		switch decision.Category {
		case types.CategoryRateLimit:
			status = "rate_limited"
			errorType = "rate_limit"
		case types.CategoryTimeout:
			status = "timeout"
			errorType = "timeout"
		case types.CategoryCircuitBreaker:
			status = "circuit_breaker"
			errorType = "circuit_breaker"
		case types.CategoryQuota:
			status = "quota_exceeded"
			errorType = "quota_exceeded"
		case types.CategoryPayment:
			status = "payment_required"
			errorType = "payment_required"
		case types.CategoryValidation:
			status = "validation"
			errorType = "validation"
		case types.CategoryNetwork:
			status = "network_error"
			errorType = "network"
		case types.CategoryProvider4xx:
			status = "provider_4xx"
			errorType = "provider_4xx"
		case types.CategoryProvider5xx:
			status = "provider_5xx"
			errorType = "provider_5xx"
		case types.CategoryParse:
			status = "parse_error"
			errorType = "parse"
		case types.CategoryEmpty:
			status = "empty_response"
			errorType = "empty_response"
		default:
			status = "error"
			errorType = "unknown"
		}
		metrics.ProviderRequestsTotal.WithLabelValues(
			attempt.ProviderID, attempt.Model, status,
			tier, strategy, errorType,
		).Inc()

		logger.Warn().
			Str("type", "router").
			Str("event", "attempt.failed").
			Str("request_id", requestID).
			Str("provider", attempt.ProviderID).
			Str("model", attempt.Model).
			Int64("attempt_latency_ms", attemptLatencyMs).
			Int64("total_latency_ms", totalLatencyMs).
			Int("timeout_ms", attemptTimeoutMs).
			Str("cancellation_source", cancellationSource(parentContextErr, attemptContextErr)).
			Str("parent_context_error", contextErrorString(parentContextErr)).
			Str("attempt_context_error", contextErrorString(attemptContextErr)).
			Str("failure_category", string(decision.Category)).
			Str("failure_action", string(decision.Action)).
			Str("failure_reason", decision.Reason).
			Err(err).
			Msg("Provider attempt failed")

		r.handleRateLimitFailure(ctx, attempt.ProviderID, attempt.Model, err)
		r.handleAuthFailure(ctx, attempt.ProviderID, attempt.Model, err)
		r.handlePaymentFailure(ctx, attempt.ProviderID, attempt.Model, err)
		r.handleStructuredOutputFailure(ctx, attempt.ProviderID, attempt.Model, req, err)

		if !r.failureAllowedByPlan(err, plan, i) {
			return nil, r.CreateGatewayError(err, i+1, requestID)
		}

		switch decision.Action {
		case types.ActionAbort:
			return nil, r.CreateGatewayError(err, i+1, requestID)
		case types.ActionRetry, types.ActionRetryWithBackoff:
			if decision.BackoffMs > 0 {
				backoffDuration := r.backoffStrategy.CalculateBackoff(i)
				metrics.BackoffSeconds.WithLabelValues(attempt.ProviderID, attempt.Model).Observe(backoffDuration.Seconds())
				logger.Info().
					Str("type", "router").
					Str("event", "attempt.backoff").
					Str("request_id", requestID).
					Dur("backoff", backoffDuration).
					Msg("Applying backoff before retry")
				if err := waitWithHardTimeout(ctx, startTime, plan, backoffDuration); err != nil {
					return nil, err
				}
			}
		case types.ActionFailover, types.ActionFailoverWithBackoff:
			if decision.BackoffMs > 0 {
				backoffDuration := r.backoffStrategy.CalculateBackoff(i)
				if err := waitWithHardTimeout(ctx, startTime, plan, backoffDuration); err != nil {
					return nil, err
				}
			}
		case types.ActionCooldown:
			if decision.CooldownSeconds > 0 && r.cooldownService != nil {
				if decision.Category == types.CategoryRateLimit {
					break
				}
				reason := r.failureCategoryToCooldownReason(decision.Category)
				retryAfter := 0
				if rateLimitErr, ok := err.(*errors.RateLimitError); ok {
					retryAfter = rateLimitErr.RetryAfter
				}
				r.cooldownService.ApplyCooldownForReason(ctx, attempt.ProviderID, attempt.Model, reason, retryAfter)
			}
		}

	}

	metrics.RoutingAttemptsTotal.WithLabelValues(
		tier, strategy,
	).Observe(float64(len(plan.Attempts)))

	if gatewayErr := aggregateRateLimitError(attemptChain); gatewayErr != nil {
		return nil, gatewayErr
	}
	if allAttemptsPaymentRequired(attemptChain) {
		return nil, &types.GatewayError{
			Type:    "payment_error",
			Code:    "PAYMENT_REQUIRED",
			Message: "All provider attempts failed due to billing issues",
			Details: map[string]any{"attempts": attemptChain},
		}
	}

	logger.Warn().
		Str("type", "router").
		Str("event", "attempts.exhausted").
		Str("request_id", requestID).
		Int("attempt_count", len(attemptChain)).
		Int64("total_latency_ms", time.Since(startTime).Milliseconds()).
		Interface("attempt_chain", attemptChain).
		Msg("All provider attempts failed")

	return nil, &types.GatewayError{
		Type:    "gateway_error",
		Code:    "ALL_ATTEMPTS_FAILED",
		Message: allAttemptsFailedMessage(attemptChain),
		Details: map[string]any{
			"attempts": attemptChain,
		},
	}
}

// ExecuteStream executes a streaming request
func (r *Router) ExecuteStream(
	ctx context.Context,
	plan types.RoutingPlan,
	req types.ChatCompletionRequest,
	requestID string,
) types.StreamResult {
	chunks := make(chan *types.SSEChunk)
	errChan := make(chan *types.GatewayError, 1)

	go func() {
		defer close(chunks)
		defer close(errChan)

		startTime := time.Now()
		var ttfbRecorded bool

		tier := metrics.GetTier(ctx)
		strategy := metrics.GetStrategy(ctx)

		var previousProvider string
		var hadFailure bool
		var chunksSent bool
		var outputTokenCount int
		var attemptChain []map[string]any
		var streamUsage *types.Usage

		for i, attempt := range plan.Attempts {
			if r.providerDisabled(attempt.ProviderID) {
				attemptChain = append(attemptChain, disabledProviderAttempt(attempt))
				continue
			}

			if quotaErr := r.checkCloudflareAttemptBudget(ctx, attempt, req); quotaErr != nil {
				logger.Warn().
					Str("type", "router").
					Str("event", "attempt.skipped").
					Str("request_id", requestID).
					Str("provider", attempt.ProviderID).
					Str("model", attempt.Model).
					Err(quotaErr).
					Msg("Skipping Cloudflare attempt because daily neuron budget is exhausted")
				continue
			}

			attemptTimeoutMs, hardTimeoutErr := boundedAttemptTimeoutMs(plan, startTime, attempt.TimeoutMs)
			if hardTimeoutErr != nil {
				errChan <- hardTimeoutErr
				return
			}

			attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(attemptTimeoutMs)*time.Millisecond)

			releaseConcurrency, deniedScope, concurrencyOK := r.acquireAttemptConcurrency(ctx, attempt.ProviderID, attempt.Model)
			if !concurrencyOK {
				logger.Warn().
					Str("type", "router").
					Str("event", "attempt.concurrency_denied").
					Str("request_id", requestID).
					Str("provider", attempt.ProviderID).
					Str("model", attempt.Model).
					Str("scope", deniedScope).
					Msg("Concurrency slot unavailable")
				attemptChain = append(attemptChain, map[string]any{
					"provider":       attempt.ProviderID,
					"model":          attempt.Model,
					"failure_kind":   "concurrency_denied",
					"failure_action": string(types.ActionFailover),
					"failure_reason": "concurrency slot unavailable, trying different provider",
				})
				cancel()
				continue
			}

			reservation, reservationErr := r.reserveAttemptQuota(ctx, attempt, req)
			if reservationErr != nil {
				if releaseConcurrency != nil {
					releaseConcurrency()
				}
				attemptChain = append(attemptChain, quotaReservationFailureAttempt(attempt, reservationErr))
				cancel()
				continue
			}

			attemptReq := req
			attemptReq.ProviderCapabilities = attempt.Capabilities

			result := r.providerService.StreamProviderChannel(
				attempt.BaseURL,
				attempt.APIKey,
				attempt.Model,
				attemptReq,
				attemptTimeoutMs,
				attemptCtx,
				attempt.ProviderType,
				attempt.Auth,
				requestID,
			)

			outputTokenCount = 0
			streamUsage = nil
			normalizer := newStreamNormalizer(attemptReq)

			for chunk := range result.Chunks {
				if !ttfbRecorded {
					metrics.StreamTTFBSeconds.WithLabelValues(
						attempt.ProviderID, attempt.Model, tier, strategy,
					).Observe(time.Since(startTime).Seconds())
					ttfbRecorded = true
				}
				if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != nil {
					outputTokenCount++
				}
				if chunk.Usage != nil {
					streamUsage = chunk.Usage
				}
				if !normalizer.Process(chunk) {
					// Suppressed usage artifact (include_usage not requested).
					continue
				}
				// Lock failover only once real payload reached the client;
				// role preambles alone don't pin this attempt (litellm rule:
				// splice only while nothing content-bearing was emitted).
				chunksSent = chunksSent || chunkHasPayload(chunk)
				select {
				case chunks <- chunk:
				case <-ctx.Done():
					cancel()
					if releaseConcurrency != nil {
						releaseConcurrency()
					}
					r.releaseTokenReservation(ctx, reservation)
					return
				}
			}

			err := <-result.Err
			cancel()
			if releaseConcurrency != nil {
				releaseConcurrency()
			}
			if err != nil {
				r.releaseTokenReservation(ctx, reservation)
			}

			latencyMs := time.Since(startTime).Milliseconds()

			if err == nil {
				if terminal := normalizer.TerminalChunk(); terminal != nil {
					select {
					case chunks <- terminal:
					case <-ctx.Done():
						r.releaseTokenReservation(ctx, reservation)
						return
					}
				}

				r.healthService.RecordSuccess(ctx, attempt.ProviderID, attempt.Model, int(latencyMs))

				if cooldownMs := r.lookupModelCooldownMs(attempt.ProviderID, attempt.Model); cooldownMs > 0 && r.cooldownService != nil {
					cooldownSec := cooldownMs / 1000
					if cooldownSec < 1 {
						cooldownSec = 1
					}
					r.cooldownService.ApplyCooldownForReason(ctx, attempt.ProviderID, attempt.Model, "success", cooldownSec)
				}

				tokensUsed := 0
				if streamUsage != nil && streamUsage.TotalTokens > 0 {
					tokensUsed = streamUsage.TotalTokens
				} else {
					tokensUsed = r.quotaService.EstimateTokens(req)
				}
				if reservation != nil {
					r.recordReservedQuotaUsage(ctx, reservation, tokensUsed)
				} else {
					r.recordQuotaUsage(ctx, attempt.ProviderID, attempt.Model, tokensUsed)
				}

				if attempt.ProviderID == cloudflareProviderID && streamUsage != nil {
					if budgetMgr, ok := r.quotaService.(CloudflareBudgetManager); ok {
						stats, quotaErr := budgetMgr.RecordCloudflareNeuronUsage(ctx, attempt.Model, streamUsage)
						if quotaErr != nil {
							logger.Warn().
								Str("type", "router").
								Str("event", "cloudflare.neuron_record_failed").
								Str("request_id", requestID).
								Str("model", attempt.Model).
								Err(quotaErr).
								Msg("Failed to record Cloudflare neuron usage")
						}
						_ = stats
					}
				}

				if hadFailure {
					metrics.RetrySuccessTotal.WithLabelValues(tier).Inc()
				}

				metrics.ProviderRequestsTotal.WithLabelValues(
					attempt.ProviderID, attempt.Model, "success",
					tier, strategy, "",
				).Inc()
				metrics.ProviderLatencySeconds.WithLabelValues(
					attempt.ProviderID, attempt.Model,
					tier, strategy,
				).Observe(float64(latencyMs) / 1000.0)
				metrics.StreamDurationSeconds.WithLabelValues(
					attempt.ProviderID, attempt.Model, tier, strategy,
				).Observe(float64(latencyMs) / 1000.0)
				if streamUsage != nil {
					metrics.ProviderTokensTotal.WithLabelValues(
						attempt.ProviderID, attempt.Model, "prompt", tier, strategy,
					).Add(float64(streamUsage.PromptTokens))
					metrics.ProviderTokensTotal.WithLabelValues(
						attempt.ProviderID, attempt.Model, "completion", tier, strategy,
					).Add(float64(streamUsage.CompletionTokens))
					metrics.ProviderTokensTotal.WithLabelValues(
						attempt.ProviderID, attempt.Model, "total", tier, strategy,
					).Add(float64(streamUsage.TotalTokens))
					metrics.StreamOutputTokensTotal.WithLabelValues(
						attempt.ProviderID, attempt.Model, tier, strategy,
					).Add(float64(streamUsage.CompletionTokens))
				} else {
					metrics.StreamOutputTokensTotal.WithLabelValues(
						attempt.ProviderID, attempt.Model, tier, strategy,
					).Add(float64(outputTokenCount))
				}
				metrics.RoutingAttemptsTotal.WithLabelValues(
					tier, strategy,
				).Observe(float64(i + 1))

				logEvent := logger.Info().
					Str("type", "router").
					Str("event", "attempt.success").
					Str("request_id", requestID).
					Str("provider", attempt.ProviderID).
					Str("model", attempt.Model).
					Int64("latency_ms", latencyMs).
					Int("tokens", tokensUsed).
					Int("attempts", i+1)
				if streamUsage != nil {
					logEvent = logEvent.
						Int("input_tokens", streamUsage.PromptTokens).
						Int("output_tokens", streamUsage.CompletionTokens)
					if streamUsage.PromptTokensDetails != nil && streamUsage.PromptTokensDetails.CachedTokens > 0 {
						logEvent = logEvent.Int("cached_tokens", streamUsage.PromptTokensDetails.CachedTokens)
					}
				}
				logEvent.Msg("Request completed")

				errChan <- nil
				return
			}

			if previousProvider != "" {
				metrics.FailoverEventsTotal.WithLabelValues(
					previousProvider, attempt.ProviderID, tier,
				).Inc()
			}
			previousProvider = attempt.ProviderID
			hadFailure = true

			typedErr := r.gatewayErrorToTypedError(err)
			failureCtx := types.FailureContext{
				AttemptIndex:       i,
				MaxAttempts:        plan.MaxAttempts,
				ProviderID:         attempt.ProviderID,
				Model:              attempt.Model,
				HasRemainingBudget: true,
			}
			decision := r.classifier.Classify(typedErr, failureCtx)

			if decision.ShouldRecordFailure {
				r.healthService.RecordFailure(ctx, attempt.ProviderID, attempt.Model)
			}

			var status string
			var errorType string
			switch decision.Category {
			case types.CategoryRateLimit:
				status = "rate_limited"
				errorType = "rate_limit"
			case types.CategoryTimeout:
				status = "timeout"
				errorType = "timeout"
			case types.CategoryProvider4xx:
				status = "provider_4xx"
				errorType = "provider_4xx"
			case types.CategoryProvider5xx:
				status = "provider_5xx"
				errorType = "provider_5xx"
			default:
				status = "error"
				errorType = "unknown"
			}
			metrics.ProviderRequestsTotal.WithLabelValues(
				attempt.ProviderID, attempt.Model, status,
				tier, strategy, errorType,
			).Inc()

			logger.Warn().
				Str("type", "router").
				Str("event", "attempt.failed").
				Str("request_id", requestID).
				Str("provider", attempt.ProviderID).
				Str("model", attempt.Model).
				Str("failure_category", string(decision.Category)).
				Str("failure_action", string(decision.Action)).
				Str("failure_reason", decision.Reason).
				Err(err).
				Msg("Provider attempt failed")

			r.handleRateLimitFailure(ctx, attempt.ProviderID, attempt.Model, typedErr)
			r.handleAuthFailure(ctx, attempt.ProviderID, attempt.Model, typedErr)
			r.handlePaymentFailure(ctx, attempt.ProviderID, attempt.Model, typedErr)
			r.handleStructuredOutputFailure(ctx, attempt.ProviderID, attempt.Model, req, typedErr)

			attemptFailure := map[string]any{
				"provider":       attempt.ProviderID,
				"model":          attempt.Model,
				"failure_kind":   string(decision.Category),
				"failure_action": string(decision.Action),
				"failure_reason": decision.Reason,
			}
			enrichAttemptFailureDetails(attemptFailure, typedErr)
			attemptChain = append(attemptChain, attemptFailure)

			ttfbRecorded = false

			// Don't retry if we already sent chunks to the client
			if chunksSent {
				errChan <- r.CreateGatewayError(typedErr, i+1, requestID)
				return
			}

			if !r.failureAllowedByPlan(typedErr, plan, i) {
				errChan <- r.CreateGatewayError(typedErr, i+1, requestID)
				return
			}

			switch decision.Action {
			case types.ActionAbort:
				errChan <- r.CreateGatewayError(typedErr, i+1, requestID)
				return
			case types.ActionRetry, types.ActionRetryWithBackoff:
				if decision.BackoffMs > 0 {
					backoffDuration := r.backoffStrategy.CalculateBackoff(i)
					metrics.BackoffSeconds.WithLabelValues(attempt.ProviderID, attempt.Model).Observe(backoffDuration.Seconds())
					logger.Info().
						Str("type", "router").
						Str("event", "attempt.backoff").
						Str("request_id", requestID).
						Dur("backoff", backoffDuration).
						Msg("Backing off before retry")
					if err := waitWithHardTimeout(ctx, startTime, plan, backoffDuration); err != nil {
						errChan <- gatewayErrorFromError(err)
						return
					}
				}
			case types.ActionFailover, types.ActionFailoverWithBackoff:
				if decision.BackoffMs > 0 {
					backoffDuration := r.backoffStrategy.CalculateBackoff(i)
					metrics.BackoffSeconds.WithLabelValues(attempt.ProviderID, attempt.Model).Observe(backoffDuration.Seconds())
					logger.Info().
						Str("type", "router").
						Str("event", "attempt.backoff").
						Str("request_id", requestID).
						Dur("backoff", backoffDuration).
						Msg("Backing off before failover")
					if err := waitWithHardTimeout(ctx, startTime, plan, backoffDuration); err != nil {
						errChan <- gatewayErrorFromError(err)
						return
					}
				}
			case types.ActionCooldown:
				if decision.CooldownSeconds > 0 && r.cooldownService != nil {
					if decision.Category == types.CategoryRateLimit {
						break
					}
					reason := r.failureCategoryToCooldownReason(decision.Category)
					retryAfter := 0
					if rateLimitErr, ok := typedErr.(*errors.RateLimitError); ok {
						retryAfter = rateLimitErr.RetryAfter
					}
					r.cooldownService.ApplyCooldownForReason(ctx, attempt.ProviderID, attempt.Model, reason, retryAfter)
				}
			}
		}

		if gatewayErr := aggregateRateLimitError(attemptChain); gatewayErr != nil {
			errChan <- gatewayErr
		} else if allAttemptsPaymentRequired(attemptChain) {
			errChan <- &types.GatewayError{
				Type:    "payment_error",
				Code:    "PAYMENT_REQUIRED",
				Message: "All provider attempts failed due to billing issues",
				Details: map[string]any{"attempts": attemptChain},
			}
		} else {
			errChan <- &types.GatewayError{
				Type:    "gateway_error",
				Code:    "ALL_ATTEMPTS_FAILED",
				Message: "All provider attempts failed",
				Details: map[string]any{
					"attempts": attemptChain,
				},
			}
		}
	}()

	return types.StreamResult{
		Chunks: chunks,
		Err:    errChan,
	}
}

// gatewayErrorToTypedError converts a GatewayError to the appropriate typed error for ShouldRetry
func (r *Router) gatewayErrorToTypedError(ge *types.GatewayError) error {
	if ge == nil {
		return nil
	}

	switch ge.Code {
	case "RATE_LIMITED", "QUOTA_EXHAUSTED", "PROVIDER_OVERLOADED":
		retryAfter := 60
		limitType := "rpm"
		limitSubtype := "rate_limit"
		if ge.Code == "QUOTA_EXHAUSTED" {
			limitSubtype = "quota_exhausted"
		}
		if ge.Code == "PROVIDER_OVERLOADED" {
			limitSubtype = "overload"
		}
		headers := map[string]string{}
		if details, ok := ge.Details["retry_after"].(int); ok {
			retryAfter = details
		}
		if details, ok := ge.Details["limit_type"].(string); ok && details != "" {
			limitType = details
		}
		if details, ok := ge.Details["limit_subtype"].(string); ok && details != "" {
			limitSubtype = details
		}
		if details, ok := ge.Details["headers"].(map[string]any); ok {
			for key, value := range details {
				headers[key] = fmt.Sprintf("%v", value)
			}
		}
		if details, ok := ge.Details["headers"].(map[string]string); ok {
			headers = details
		}
		err := errors.NewRateLimitErrorWithSubtype(ge.Message, retryAfter, limitType, limitSubtype, headers)
		err.Headers = headers
		return err
	case "TIMEOUT", "HARD_TIMEOUT":
		timeoutType := "request"
		if ge.Code == "HARD_TIMEOUT" {
			timeoutType = "hard"
		}
		return errors.NewTimeoutError(ge.Message, timeoutType)
	case "PAYMENT_REQUIRED":
		return errors.NewPaymentRequiredError(ge.Message)
	case "VALIDATION_FAILED", "VALIDATION_ERROR":
		return errors.NewValidationError(ge.Message, nil)
	case "CIRCUIT_BREAKER_OPEN":
		providerID := ""
		if details, ok := ge.Details["provider_id"].(string); ok {
			providerID = details
		}
		return errors.NewCircuitBreakerError(ge.Message, providerID, "OPEN")
	case "QUOTA_EXCEEDED":
		providerID := ""
		model := ""
		limitType := ""
		if details, ok := ge.Details["provider_id"].(string); ok {
			providerID = details
		}
		if details, ok := ge.Details["model"].(string); ok {
			model = details
		}
		if details, ok := ge.Details["limit_type"].(string); ok {
			limitType = details
		}
		return errors.NewModelQuotaExceededError(ge.Message, providerID, model, limitType)
	case "PROVIDER_ERROR":
		statusCode := 500
		if details, ok := ge.Details["status_code"].(int); ok && details > 0 {
			statusCode = details
		}
		if details, ok := ge.Details["status_code"].(float64); ok && details > 0 {
			statusCode = int(details)
		}
		return &errors.ProviderError{
			Message:     upstreamProviderFailureMessage,
			StatusCode:  statusCode,
			IsRetryable: statusCode >= 500,
		}
	default:
		// Provider errors - check if retryable based on code
		isRetryable := ge.Code == "REQUEST_FAILED" || ge.Code == "STREAM_PARSE_FAILED"
		return &errors.ProviderError{
			Message:     ge.Message,
			StatusCode:  500,
			IsRetryable: isRetryable,
		}
	}
}

// ShouldRetry determines if an error should trigger a retry
func (r *Router) ShouldRetry(err error, plan types.RoutingPlan, attemptIndex int) bool {
	if attemptIndex >= len(plan.Attempts)-1 {
		return false // No more attempts
	}

	switch e := err.(type) {
	case *errors.RateLimitError:
		return plan.RetryOn429
	case *errors.TimeoutError:
		return plan.RetryOnTimeout
	case *errors.ProviderError:
		return e.IsRetryable && plan.RetryOn5xx
	case *errors.CircuitBreakerError:
		return true // Try different provider
	case *errors.ModelQuotaExceededError:
		return true // Try different provider
	case *errors.PaymentRequiredError:
		return false
	case *errors.ValidationError:
		return false
	case *errors.NetworkError:
		// Network errors are usually transient, so retry
		return true
	case *errors.EmptyResponseError:
		// Empty responses can be transient
		return true
	case *errors.ParseError:
		// Parse errors are usually not retryable (same bad response)
		return false
	default:
		return false
	}
}

func (r *Router) failureAllowedByPlan(err error, plan types.RoutingPlan, attemptIndex int) bool {
	if !plan.RetryPolicySet {
		return true
	}

	switch e := err.(type) {
	case *errors.RateLimitError:
		return plan.RetryOn429
	case *errors.TimeoutError:
		return plan.RetryOnTimeout
	case *errors.ProviderError:
		if e.StatusCode >= 500 || e.IsRetryable {
			return plan.RetryOn5xx
		}
	}

	return true
}

func boundedAttemptTimeoutMs(plan types.RoutingPlan, start time.Time, attemptTimeoutMs int) (int, *types.GatewayError) {
	if plan.HardTimeoutMs == nil {
		return attemptTimeoutMs, nil
	}

	elapsedMs := int(time.Since(start).Milliseconds())
	remainingMs := *plan.HardTimeoutMs - elapsedMs
	if remainingMs <= 0 {
		return 0, hardTimeoutGatewayError()
	}
	if attemptTimeoutMs <= 0 || attemptTimeoutMs > remainingMs {
		return remainingMs, nil
	}
	return attemptTimeoutMs, nil
}

func waitWithHardTimeout(ctx context.Context, start time.Time, plan types.RoutingPlan, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}

	if plan.HardTimeoutMs != nil {
		remaining := time.Duration(*plan.HardTimeoutMs)*time.Millisecond - time.Since(start)
		if remaining <= 0 {
			return hardTimeoutGatewayError()
		}
		if duration > remaining {
			duration = remaining
		}
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		if plan.HardTimeoutMs != nil && time.Since(start) >= time.Duration(*plan.HardTimeoutMs)*time.Millisecond {
			return hardTimeoutGatewayError()
		}
		return nil
	case <-ctx.Done():
		return errors.NewTimeoutError("Context cancelled during backoff", "request")
	}
}

func cancellationSource(parentErr, attemptErr error) string {
	if parentErr != nil {
		return "parent"
	}
	if attemptErr == context.DeadlineExceeded {
		return "attempt_deadline"
	}
	if attemptErr == context.Canceled {
		return "attempt_canceled"
	}
	return ""
}

func contextErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func hardTimeoutGatewayError() *types.GatewayError {
	return &types.GatewayError{
		Type:    "timeout_error",
		Code:    "HARD_TIMEOUT",
		Message: "Hard timeout exceeded",
	}
}

func gatewayErrorFromError(err error) *types.GatewayError {
	if gatewayErr, ok := err.(*types.GatewayError); ok {
		return gatewayErr
	}
	if timeoutErr, ok := err.(*errors.TimeoutError); ok {
		return &types.GatewayError{Type: "timeout_error", Code: "TIMEOUT", Message: timeoutErr.Message}
	}
	return &types.GatewayError{Type: "gateway_error", Code: "EXECUTION_ERROR", Message: err.Error()}
}

func (r *Router) providerAvailable(provider types.ProviderConfig) bool {
	if r.providerDisabled(provider.ID) {
		return false
	}

	if provider.ProviderType == cloudflareProviderType && os.Getenv(cloudflareAccountIDEnv) == "" {
		return false
	}

	switch provider.Auth.Type {
	case "bearer", "header":
		if provider.Auth.Env == "" || provider.Auth.Optional {
			return true
		}
		return r.resolveProviderAPIKey(provider.Auth) != ""
	default:
		return true
	}
}

func (r *Router) resolveCapabilities(provider types.ProviderConfig, model string) types.ProviderCapabilities {
	resolved := provider.Capabilities
	overrides, ok := provider.Models.Capabilities[model]
	if !ok {
		return resolved
	}

	if overrides.Streaming != nil {
		resolved.Streaming = *overrides.Streaming
	}
	if overrides.Tools != nil {
		resolved.Tools = *overrides.Tools
	}
	if overrides.StructuredOutputs != nil {
		resolved.StructuredOutputs = *overrides.StructuredOutputs
	}
	if overrides.Logprobs != nil {
		resolved.Logprobs = *overrides.Logprobs
	}
	if overrides.Metadata != nil {
		resolved.Metadata = *overrides.Metadata
	}
	if overrides.Seed != nil {
		resolved.Seed = *overrides.Seed
	}
	if overrides.User != nil {
		resolved.User = *overrides.User
	}
	if overrides.FrequencyPenalty != nil {
		resolved.FrequencyPenalty = *overrides.FrequencyPenalty
	}
	if overrides.PresencePenalty != nil {
		resolved.PresencePenalty = *overrides.PresencePenalty
	}
	if overrides.MaxTokens != nil {
		resolved.MaxTokens = *overrides.MaxTokens
	}
	if overrides.MaxCompletionTokens != nil {
		resolved.MaxCompletionTokens = *overrides.MaxCompletionTokens
	}
	if overrides.MultipleChoices != nil {
		resolved.MultipleChoices = *overrides.MultipleChoices
	}
	if overrides.ToolSchema != nil {
		resolved.ToolSchema = *overrides.ToolSchema
	}
	if overrides.Reasoning != nil {
		resolved.Reasoning = *overrides.Reasoning
	}
	if len(overrides.ReasoningLevels) > 0 {
		resolved.ReasoningLevels = overrides.ReasoningLevels
	}

	return resolved
}

func supportsJSONObject(caps types.ProviderCapabilities) bool {
	switch caps.StructuredOutputs {
	case "json_object", "json_schema", "json_schema_strict", "model_dependent":
		return true
	default:
		return false
	}
}

func supportsStructuredOutput(requirements types.DerivedRequirements, caps types.ProviderCapabilities, strictCertified bool) bool {
	switch requirements.Output {
	case "", "text":
		return true
	case "json_object":
		return supportsJSONObject(caps) || strictCertified
	case "json_schema":
		return supportsJSONSchema(caps) || strictCertified
	case "json_schema_strict":
		if requirements.Streaming == "required" && caps.StructuredOutputs == "json_object" && !strictCertified {
			return false
		}
		return supportsJSONObject(caps) || strictCertified
	default:
		return false
	}
}

func structuredOutputFilterReason(requirements types.DerivedRequirements, caps types.ProviderCapabilities, strictCertified bool) string {
	switch requirements.Output {
	case "json_object":
		return "json_object_not_supported"
	case "json_schema":
		return "json_schema_not_supported"
	case "json_schema_strict":
		if requirements.Streaming == "required" && caps.StructuredOutputs == "json_object" && !strictCertified {
			return "strict_streaming_requires_schema_support"
		}
		return "not_certified_for_strict_json"
	default:
		return "structured_output_not_supported"
	}
}

func supportsJSONSchema(caps types.ProviderCapabilities) bool {
	switch caps.StructuredOutputs {
	case "json_schema", "json_schema_strict", "model_dependent":
		return true
	default:
		return false
	}
}

func (r *Router) resolveProviderAPIKey(auth types.ProviderAuth) string {
	if auth.Env == "" {
		return ""
	}

	return os.Getenv(auth.Env)
}

// CreateGatewayError creates a gateway error from a provider error
func (r *Router) CreateGatewayError(err error, attempts int, requestID string) *types.GatewayError {
	switch e := err.(type) {
	case *errors.RateLimitError:
		code := "RATE_LIMITED"
		if e.LimitSubtype == "quota_exhausted" {
			code = "QUOTA_EXHAUSTED"
		} else if e.LimitSubtype == "overload" {
			code = "PROVIDER_OVERLOADED"
		}
		return &types.GatewayError{
			Type:    "rate_limit_error",
			Code:    code,
			Message: e.Error(),
			Details: map[string]any{
				"retry_after":   e.RetryAfter,
				"limit_type":    e.LimitType,
				"limit_subtype": e.LimitSubtype,
				"headers":       e.Headers,
				"attempts":      attempts,
			},
		}
	case *errors.CircuitBreakerError:
		return &types.GatewayError{
			Type:    "circuit_breaker_error",
			Code:    "CIRCUIT_BREAKER_OPEN",
			Message: e.Error(),
			Details: map[string]any{
				"provider_id": e.ProviderID,
				"state":       e.State,
				"attempts":    attempts,
			},
		}
	case *errors.TimeoutError:
		return &types.GatewayError{
			Type:    "timeout_error",
			Code:    "TIMEOUT",
			Message: e.Error(),
			Details: map[string]any{
				"timeout_type": e.TimeoutType,
				"attempts":     attempts,
			},
		}
	case *errors.ModelQuotaExceededError:
		return &types.GatewayError{
			Type:    "quota_error",
			Code:    "QUOTA_EXCEEDED",
			Message: e.Error(),
			Details: map[string]any{
				"provider_id": e.ProviderID,
				"model":       e.Model,
				"limit_type":  e.LimitType,
				"attempts":    attempts,
			},
		}
	case *errors.PaymentRequiredError:
		return &types.GatewayError{
			Type:    "payment_error",
			Code:    "PAYMENT_REQUIRED",
			Message: e.Error(),
			Details: map[string]any{
				"attempts": attempts,
			},
		}
	case *errors.ValidationError:
		return &types.GatewayError{
			Type:    "validation_error",
			Code:    "VALIDATION_ERROR",
			Message: e.Error(),
			Details: map[string]any{
				"attempts": attempts,
			},
		}
	case *errors.ProviderError:
		return &types.GatewayError{
			Type:    "upstream_error",
			Code:    "PROVIDER_ERROR",
			Message: upstreamProviderFailureMessage,
			Details: map[string]any{
				"attempts":    attempts,
				"status_code": e.StatusCode,
			},
		}
	default:
		return &types.GatewayError{
			Type:    "upstream_error",
			Code:    "PROVIDER_ERROR",
			Message: upstreamProviderFailureMessage,
			Details: map[string]any{
				"attempts": attempts,
			},
		}
	}
}

func buildRateLimitResponse(rateLimitErr *errors.RateLimitError) *http.Response {
	header := http.Header{}
	if rateLimitErr == nil {
		return &http.Response{StatusCode: 429, Header: header}
	}
	if rateLimitErr.RetryAfter > 0 {
		header.Set("Retry-After", fmt.Sprintf("%d", rateLimitErr.RetryAfter))
	}
	for key, value := range rateLimitErr.Headers {
		header.Set(key, value)
	}
	return &http.Response{StatusCode: 429, Header: header}
}

func (r *Router) checkCloudflareAttemptBudget(ctx context.Context, attempt types.RoutingAttempt, req types.ChatCompletionRequest) error {
	if attempt.ProviderID != cloudflareProviderID {
		return nil
	}

	budgetMgr, ok := r.quotaService.(CloudflareBudgetManager)
	if !ok {
		return nil
	}

	estimatedNeurons := budgetMgr.EstimateCloudflareRequestNeurons(attempt.Model, req)
	return budgetMgr.CheckCloudflareDailyNeuronBudget(ctx, attempt.Model, estimatedNeurons)
}

func (r *Router) maybeMarkCloudflareDailyBudgetExhausted(ctx context.Context, providerID string, rateLimitErr *errors.RateLimitError) {
	if providerID != cloudflareProviderID || rateLimitErr == nil || rateLimitErr.LimitSubtype != "quota_exhausted" {
		return
	}

	budgetMgr, ok := r.quotaService.(CloudflareBudgetManager)
	if !ok {
		return
	}

	if err := budgetMgr.MarkCloudflareDailyBudgetExhausted(ctx); err != nil {
		logger.Warn().
			Str("type", "router").
			Str("event", "cloudflare.neuron_exhausted_mark_failed").
			Err(err).
			Msg("Failed to mark Cloudflare daily neuron budget exhausted")
	}
}

func (r *Router) isCertifiedForStrictSchema(providerID, model string) bool {
	for _, cert := range r.config.Certifications {
		if cert.Provider == providerID && cert.Model == model && cert.StrictSchema {
			return true
		}
	}
	return false
}

func (r *Router) failureCategoryToCooldownReason(category types.FailureCategory) CooldownReason {
	switch category {
	case types.CategoryRateLimit:
		return CooldownRateLimit
	case types.CategoryQuota:
		return CooldownQuota
	case types.CategoryParse, types.CategoryEmpty:
		return CooldownStructuredOutput
	case types.CategoryPayment:
		return CooldownBilling
	case types.CategoryProvider5xx:
		return CooldownOverload
	case types.CategoryProvider4xx:
		return CooldownAuth
	default:
		return CooldownDefault
	}
}

func (r *Router) handleStructuredOutputFailure(ctx context.Context, providerID, model string, req types.ChatCompletionRequest, err error) {
	if r.cooldownService == nil || req.ResponseFormat == nil {
		return
	}
	if !isStructuredOutputFailure(err) {
		return
	}
	r.cooldownService.ApplyCooldownForReason(ctx, providerID, model, CooldownStructuredOutput, 0)
}

func isStructuredOutputFailure(err error) bool {
	switch err.(type) {
	case *errors.ParseError, *errors.EmptyResponseError:
		return true
	}
	providerErr, ok := err.(*errors.ProviderError)
	if !ok || !isValidationStatus(providerErr.StatusCode) {
		return false
	}
	lower := strings.ToLower(providerErr.Message)
	return strings.Contains(lower, "unsupported json schema feature") ||
		strings.Contains(lower, "unsupported schema feature") ||
		strings.Contains(lower, "failed_generation") ||
		(strings.Contains(lower, "failed to validate json") && strings.Contains(lower, "json"))
}

// handlePaymentFailure benches every model of a provider whose account is in
// arrears (upstream 402). The request itself still fails over to other
// providers; this cooldown just keeps the broke provider out of subsequent
// attempt chains until billing recovers.
func (r *Router) handlePaymentFailure(ctx context.Context, providerID, model string, err error) {
	if _, ok := err.(*errors.PaymentRequiredError); !ok {
		return
	}
	if r.cooldownService == nil {
		return
	}

	if provider, ok := r.lookupProvider(providerID); ok {
		for _, providerModel := range provider.Models.List {
			r.cooldownService.ApplyCooldownForReason(ctx, providerID, providerModel, CooldownBilling, 0)
		}
		return
	}
	r.cooldownService.ApplyCooldownForReason(ctx, providerID, model, CooldownBilling, 0)
}

func (r *Router) handleAuthFailure(ctx context.Context, providerID, model string, err error) {
	if !isAuthProviderError(err) {
		return
	}

	if disabled, reason := r.providerDisabler.RecordAuthFailure(providerID, model); disabled {
		logger.Warn().
			Str("type", "router").
			Str("event", "provider.disabled").
			Str("provider", providerID).
			Str("model", model).
			Str("reason", reason).
			Msg("Provider disabled after auth failure")
	}

	if r.cooldownService == nil {
		return
	}

	provider, ok := r.lookupProvider(providerID)
	if !ok {
		r.cooldownService.ApplyCooldownForReason(ctx, providerID, model, CooldownAuth, 0)
		return
	}

	for _, providerModel := range provider.Models.List {
		r.cooldownService.ApplyCooldownForReason(ctx, providerID, providerModel, CooldownAuth, 0)
	}
}

func (r *Router) providerDisabled(providerID string) bool {
	return r.providerDisabler != nil && r.providerDisabler.IsDisabled(providerID)
}

func disabledProviderAttempt(attempt types.RoutingAttempt) map[string]any {
	return map[string]any{
		"provider":       attempt.ProviderID,
		"model":          attempt.Model,
		"failure_kind":   "provider_disabled",
		"failure_action": string(types.ActionFailover),
		"failure_reason": "provider disabled after auth failure",
	}
}

func quotaReservationFailureAttempt(attempt types.RoutingAttempt, err error) map[string]any {
	entry := map[string]any{
		"provider":       attempt.ProviderID,
		"model":          attempt.Model,
		"failure_kind":   string(types.CategoryQuota),
		"failure_action": string(types.ActionFailover),
		"failure_reason": "quota reservation denied",
	}
	enrichAttemptFailureDetails(entry, err)
	return entry
}

func isAuthProviderError(err error) bool {
	providerErr, ok := err.(*errors.ProviderError)
	if !ok {
		return false
	}
	return providerErr.StatusCode == http.StatusUnauthorized || providerErr.StatusCode == http.StatusForbidden
}

func (r *Router) handleRateLimitFailure(ctx context.Context, providerID, model string, err error) {
	rateLimitErr, ok := err.(*errors.RateLimitError)
	if !ok {
		return
	}

	if r.cooldownService != nil {
		r.applyRateLimitCooldown(ctx, providerID, model, rateLimitErr)
	}
	r.maybeMarkCloudflareDailyBudgetExhausted(ctx, providerID, rateLimitErr)
	if syncer, ok := r.quotaService.(ProviderQuotaSyncer); ok && rateLimitErr.ProviderQuotaLimit > 0 {
		if err := syncer.SyncProviderQuotaLimit(ctx, providerID, model, rateLimitErr.LimitType, rateLimitErr.ProviderQuotaLimit); err != nil {
			logger.Error().
				Str("type", "router").
				Str("event", "quota.provider_sync_failed").
				Str("provider", providerID).
				Str("model", model).
				Str("limit_type", rateLimitErr.LimitType).
				Err(err).
				Msg("Failed to sync provider quota limit")
		}
	}
	r.quotaService.HandleProviderRateLimit(ctx, providerID, model, buildRateLimitResponse(rateLimitErr))
	if provider, ok := r.lookupProvider(providerID); ok && hasModelLimits(providerLevelModelLimits(provider.Limits)) {
		r.quotaService.HandleProviderRateLimit(ctx, providerID, providerQuotaScopeModel, buildRateLimitResponse(rateLimitErr))
	}
}

func (r *Router) applyRateLimitCooldown(ctx context.Context, providerID, model string, err *errors.RateLimitError) {
	reason := rateLimitCooldownReason(err)
	r.cooldownService.ApplyCooldownForReason(ctx, providerID, model, reason, r.effectiveRateLimitCooldownSeconds(err, providerID, model))

	provider, ok := r.lookupProvider(providerID)
	if !ok {
		return
	}
	// Providers that manage their own per-model limits (e.g. openrouter-alpha)
	// are exempt from roster-wide benching: one model's daily cap says nothing
	// about its siblings there.
	if isDayScaleLimit(err) && rosterBenchExemptProviders[providerID] {
		return
	}
	if !providerLimitMatchesRateLimit(provider.Limits, err) {
		return
	}

	for _, providerModel := range provider.Models.List {
		if providerModel == model {
			continue
		}
		r.cooldownService.ApplyCooldownForReason(ctx, providerID, providerModel, reason, r.effectiveRateLimitCooldownSeconds(err, providerID, providerModel))
	}
}

// rosterBenchExemptProviders lists providers whose models carry independent
// limits, so a day-scale failure on one model must not bench the rest.
var rosterBenchExemptProviders = map[string]bool{
	"openrouter-alpha": true,
}

func isDayScaleLimit(err *errors.RateLimitError) bool {
	return err.LimitType == "rpd" || err.LimitType == "tpd"
}

// effectiveRateLimitCooldownSeconds resolves how long a model should stay benched
// after a provider 429. Day-scale quotas (rpd/tpd) are handled first — a small
// retry-after is meaningless when the window resets daily. Otherwise a
// provider-supplied Retry-After wins; then a configured per-model pause window;
// then 0 falls back to the reason's default cooldown duration.
func (r *Router) effectiveRateLimitCooldownSeconds(err *errors.RateLimitError, providerID, model string) int {
	if isDayScaleLimit(err) {
		return dailyQuotaCooldownSeconds(err.ResetAtUnixMs, providerID)
	}
	if err.RetryAfterProvided && err.RetryAfter > 0 {
		return err.RetryAfter
	}
	if pauseMs := r.lookupModelRateLimitPauseMs(providerID, model); pauseMs > 0 {
		return int((time.Duration(pauseMs) * time.Millisecond).Seconds())
	}
	return 0
}

// dailyQuotaFallbackCooldown benches day-scale quota deaths for hours instead of
// seconds when no better reset signal exists (2026-08-22 incident: daily caps
// were re-walked every request on 5s cooldowns).
const dailyQuotaFallbackCooldown = 6 * time.Hour

// dailyQuotaCooldownSeconds picks the bench duration for an exhausted daily
// quota, in priority order: provider-stated absolute reset → known provider
// reset schedule (Gemini resets at midnight Pacific) → flat fallback.
func dailyQuotaCooldownSeconds(resetAtUnixMs int64, providerID string) int {
	now := time.Now()
	if resetAtUnixMs > 0 {
		until := time.UnixMilli(resetAtUnixMs).Sub(now)
		if until > 30*time.Second && until < 48*time.Hour {
			return int((until + 30*time.Second).Seconds())
		}
	}
	if providerID == "gemini" {
		if secs := secondsUntilNextMidnightPT(now); secs > 0 {
			return secs
		}
	}
	return int(dailyQuotaFallbackCooldown.Seconds())
}

func secondsUntilNextMidnightPT(now time.Time) int {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return 0
	}
	local := now.In(loc)
	nextMidnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	return int(nextMidnight.Sub(now).Seconds()) + 60 // small buffer past the reset
}

func rateLimitCooldownReason(err *errors.RateLimitError) CooldownReason {
	switch err.LimitSubtype {
	case "quota_exhausted":
		return CooldownQuota
	case "overload":
		return CooldownOverload
	default:
		return CooldownRateLimit
	}
}

func providerLimitMatchesRateLimit(limits types.ProviderLimits, err *errors.RateLimitError) bool {
	if err.LimitSubtype == "quota_exhausted" {
		return hasModelLimits(providerLevelModelLimits(limits))
	}

	switch err.LimitType {
	case "rpm":
		return limits.Rpm != nil
	case "rph":
		return limits.Rph != nil
	case "rpd":
		return limits.Rpd != nil || limits.DailyRequests != nil
	case "tpm":
		return limits.Tpm != nil
	case "tph":
		return limits.Tph != nil
	case "tpd":
		return limits.Tpd != nil
	default:
		return false
	}
}

func (r *Router) recordQuotaUsage(ctx context.Context, providerID, model string, tokensUsed int) {
	r.quotaService.RecordModelUsage(ctx, providerID, model, tokensUsed)
	provider, ok := r.lookupProvider(providerID)
	if !ok || !hasModelLimits(providerLevelModelLimits(provider.Limits)) {
		return
	}
	r.quotaService.RecordModelUsage(ctx, providerID, providerQuotaScopeModel, tokensUsed)
}

func (r *Router) lookupProvider(providerID string) (types.ProviderConfig, bool) {
	for _, provider := range r.config.Providers {
		if provider.ID == providerID {
			return provider, true
		}
	}
	return types.ProviderConfig{}, false
}

func (r *Router) lookupModelConcurrencyLimit(providerID, model string) int {
	for _, p := range r.config.Providers {
		if p.ID == providerID {
			limits := effectiveModelLimits(p, model)
			if limits.MaxConcurrent != nil {
				return *limits.MaxConcurrent
			}
			return 0
		}
	}
	return 0
}

// lookupProviderConcurrencyLimit returns the provider-wide (account-level)
// in-flight cap, e.g. nous enforces concurrency per account rather than per
// model. 0 means the provider has no account-wide cap.
func (r *Router) lookupProviderConcurrencyLimit(providerID string) int {
	for _, p := range r.config.Providers {
		if p.ID == providerID {
			if p.Limits.MaxConcurrent != nil {
				return *p.Limits.MaxConcurrent
			}
			return 0
		}
	}
	return 0
}

// acquireAttemptConcurrency enforces both concurrency caps for an attempt: the
// account-wide slot first (coarse gate — a denial there fails cheaply), then the
// per-model slot. Returns a release closure when any slot was taken; ok=false
// means a cap denied the attempt outright and deniedScope names which one.
func (r *Router) acquireAttemptConcurrency(ctx context.Context, providerID, model string) (release func(), deniedScope string, ok bool) {
	var acquired []string
	releaseAll := func() {
		for i := len(acquired) - 1; i >= 0; i-- {
			r.releaseConcurrencySlot(providerID, acquired[i])
		}
	}
	if limit := r.lookupProviderConcurrencyLimit(providerID); limit > 0 {
		if err := r.quotaService.AcquireConcurrencySlot(ctx, providerID, providerQuotaScopeModel, limit); err != nil {
			return nil, "provider", false
		}
		acquired = append(acquired, providerQuotaScopeModel)
	}
	if limit := r.lookupModelConcurrencyLimit(providerID, model); limit > 0 {
		if err := r.quotaService.AcquireConcurrencySlot(ctx, providerID, model, limit); err != nil {
			releaseAll()
			return nil, "model", false
		}
		acquired = append(acquired, model)
	}
	if len(acquired) == 0 {
		return nil, "", true
	}
	return releaseAll, "", true
}

func effectiveModelLimits(provider types.ProviderConfig, model string) types.ModelLimits {
	limits := types.ModelLimits{
		Rpm:              provider.Limits.Rpm,
		Rph:              provider.Limits.Rph,
		Rpd:              provider.Limits.Rpd,
		Tpm:              provider.Limits.Tpm,
		Tph:              provider.Limits.Tph,
		Tpd:              provider.Limits.Tpd,
		MaxConcurrent:    provider.Limits.MaxConcurrent,
		RateLimitPauseMs: provider.Limits.RateLimitPauseMs,
	}
	if limits.Rpd == nil {
		limits.Rpd = provider.Limits.DailyRequests
	}

	modelLimits, ok := provider.Models.Limits[model]
	if !ok {
		return limits
	}
	if modelLimits.Rpm != nil {
		limits.Rpm = modelLimits.Rpm
	}
	if modelLimits.Rph != nil {
		limits.Rph = modelLimits.Rph
	}
	if modelLimits.Rpd != nil {
		limits.Rpd = modelLimits.Rpd
	}
	if modelLimits.Tpm != nil {
		limits.Tpm = modelLimits.Tpm
	}
	if modelLimits.Tph != nil {
		limits.Tph = modelLimits.Tph
	}
	if modelLimits.Tpd != nil {
		limits.Tpd = modelLimits.Tpd
	}
	if modelLimits.Tpmu != nil {
		limits.Tpmu = modelLimits.Tpmu
	}
	if modelLimits.MaxConcurrent != nil {
		limits.MaxConcurrent = modelLimits.MaxConcurrent
	}
	if modelLimits.CooldownAfterMs != nil {
		limits.CooldownAfterMs = modelLimits.CooldownAfterMs
	}
	if modelLimits.RateLimitPauseMs != nil {
		limits.RateLimitPauseMs = modelLimits.RateLimitPauseMs
	}
	if modelLimits.TimeoutMs != nil {
		limits.TimeoutMs = modelLimits.TimeoutMs
	}
	return limits
}

func providerLevelModelLimits(limits types.ProviderLimits) types.ModelLimits {
	providerLimits := types.ModelLimits{
		Rpm:           limits.Rpm,
		Rph:           limits.Rph,
		Rpd:           limits.Rpd,
		Tpm:           limits.Tpm,
		Tph:           limits.Tph,
		Tpd:           limits.Tpd,
		MaxConcurrent: limits.MaxConcurrent,
	}
	if providerLimits.Rpd == nil {
		providerLimits.Rpd = limits.DailyRequests
	}
	return providerLimits
}

func hasModelLimits(limits types.ModelLimits) bool {
	return limits.Rpm != nil ||
		limits.Rph != nil ||
		limits.Rpd != nil ||
		limits.Tpm != nil ||
		limits.Tph != nil ||
		limits.Tpd != nil ||
		limits.Tpmu != nil
}

func (r *Router) releaseConcurrencySlot(providerID, model string) {
	releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r.quotaService.ReleaseConcurrencySlot(releaseCtx, providerID, model)
}

func (r *Router) reserveAttemptQuota(ctx context.Context, attempt types.RoutingAttempt, req types.ChatCompletionRequest) (*QuotaReservation, error) {
	reservationService, ok := r.quotaService.(QuotaReservationService)
	if !ok {
		return nil, nil
	}
	provider, ok := r.lookupProvider(attempt.ProviderID)
	if !ok {
		return nil, nil
	}
	limits := effectiveModelLimits(provider, attempt.Model)
	if !hasModelLimits(limits) {
		return nil, nil
	}
	estimatedTokens := r.quotaService.EstimateTokens(req)
	return reservationService.CheckAndReserveQuota(ctx, attempt.ProviderID, attempt.Model, limits, estimatedTokens)
}

func (r *Router) releaseQuotaReservation(ctx context.Context, reservation *QuotaReservation) {
	if reservation == nil {
		return
	}
	reservationService, ok := r.quotaService.(QuotaReservationService)
	if !ok {
		return
	}
	if err := reservationService.ReleaseQuotaReservation(ctx, reservation); err != nil {
		logger.Error().
			Str("type", "router").
			Str("event", "quota.reservation_release_failed").
			Str("provider", reservation.ProviderID).
			Str("model", reservation.Model).
			Err(err).
			Msg("Failed to release quota reservation")
	}
}

// releaseTokenReservation drops the token estimates from a failed attempt's
// reservation while keeping the request-count entries: the upstream provider
// counted the request, so RPM/RPH/RPD windows must reflect it.
func (r *Router) releaseTokenReservation(ctx context.Context, reservation *QuotaReservation) {
	if reservation == nil {
		return
	}
	reservationService, ok := r.quotaService.(QuotaReservationService)
	if !ok {
		return
	}
	if err := reservationService.ReleaseTokenReservation(ctx, reservation); err != nil {
		logger.Error().
			Str("type", "router").
			Str("event", "quota.token_reservation_release_failed").
			Str("provider", reservation.ProviderID).
			Str("model", reservation.Model).
			Err(err).
			Msg("Failed to release token reservation")
	}
}

func (r *Router) recordReservedQuotaUsage(ctx context.Context, reservation *QuotaReservation, actualTokens int) {
	reservationService, ok := r.quotaService.(QuotaReservationService)
	if !ok || reservation == nil {
		return
	}
	if err := reservationService.RecordTokenUsage(ctx, reservation, actualTokens); err != nil {
		logger.Error().
			Str("type", "router").
			Str("event", "quota.reservation_record_failed").
			Str("provider", reservation.ProviderID).
			Str("model", reservation.Model).
			Err(err).
			Msg("Failed to record reserved quota usage")
	}
}

func (r *Router) lookupModelCooldownMs(providerID, model string) int {
	for _, p := range r.config.Providers {
		if p.ID == providerID {
			if limits, ok := p.Models.Limits[model]; ok && limits.CooldownAfterMs != nil {
				return *limits.CooldownAfterMs
			}
			return 0
		}
	}
	return 0
}

func (r *Router) lookupModelRateLimitPauseMs(providerID, model string) int {
	for _, p := range r.config.Providers {
		if p.ID == providerID {
			if limits, ok := p.Models.Limits[model]; ok && limits.RateLimitPauseMs != nil {
				return *limits.RateLimitPauseMs
			}
			return 0
		}
	}
	return 0
}

func allAttemptsRateLimited(chain []map[string]any) bool {
	if len(chain) == 0 {
		return false
	}
	for _, entry := range chain {
		kind, _ := entry["failure_kind"].(string)
		if kind != "rate_limit" && kind != "quota" {
			return false
		}
	}
	return true
}

func enrichAttemptFailureDetails(entry map[string]any, err error) {
	switch e := err.(type) {
	case *errors.RateLimitError:
		entry["retry_after"] = e.RetryAfter
		entry["limit_type"] = e.LimitType
		entry["limit_subtype"] = e.LimitSubtype
	case *errors.ModelQuotaExceededError:
		entry["limit_type"] = e.LimitType
	}
}

func aggregateRateLimitError(chain []map[string]any) *types.GatewayError {
	if !allAttemptsRateLimited(chain) {
		return nil
	}

	code := "RATE_LIMITED"
	message := "All provider attempts failed due to rate limits or quota"
	allQuotaExhausted := true
	allOverloaded := true
	retryAfter := 0

	for _, entry := range chain {
		kind, _ := entry["failure_kind"].(string)
		subtype, _ := entry["limit_subtype"].(string)
		if kind != "quota" && subtype != "quota_exhausted" {
			allQuotaExhausted = false
		}
		if subtype != "overload" {
			allOverloaded = false
		}
		if value, ok := entry["retry_after"].(int); ok && value > retryAfter {
			retryAfter = value
		}
	}

	if allQuotaExhausted {
		code = "QUOTA_EXHAUSTED"
		message = "All provider attempts failed due to exhausted quota"
	} else if allOverloaded {
		code = "PROVIDER_OVERLOADED"
		message = "All provider attempts failed because providers are overloaded"
	}

	details := map[string]any{"attempts": chain}
	if retryAfter > 0 {
		details["retry_after"] = retryAfter
	}

	return &types.GatewayError{
		Type:    "gateway_error",
		Code:    code,
		Message: message,
		Details: details,
	}
}

func allAttemptsPaymentRequired(chain []map[string]any) bool {
	if len(chain) == 0 {
		return false
	}
	for _, entry := range chain {
		kind, _ := entry["failure_kind"].(string)
		if kind != string(types.CategoryPayment) {
			return false
		}
	}
	return true
}

func allAttemptsFailedMessage(chain []map[string]any) string {
	if len(chain) == 0 {
		return "All provider attempts failed"
	}

	last := chain[len(chain)-1]
	reason, _ := last["failure_reason"].(string)
	provider, _ := last["provider"].(string)
	model, _ := last["model"].(string)
	if reason == "" {
		return "All provider attempts failed"
	}
	if provider == "" || model == "" {
		return "All provider attempts failed: " + reason
	}
	return fmt.Sprintf("All provider attempts failed; last failure on %s/%s: %s", provider, model, reason)
}

func allStreamFailuresRateLimited(kinds []types.FailureCategory) bool {
	if len(kinds) == 0 {
		return false
	}
	for _, k := range kinds {
		if k != types.CategoryRateLimit && k != types.CategoryQuota {
			return false
		}
	}
	return true
}
