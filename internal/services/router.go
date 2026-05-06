package services

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/logger"
	"github.com/abdo-355/llm-gateway/internal/metrics"
	"github.com/abdo-355/llm-gateway/internal/types"
)

type Router struct {
	config          types.AppConfig
	quotaService    QuotaChecker
	healthService   HealthChecker
	providerService ProviderCaller
	classifier      FailureClassifier
	backoffStrategy BackoffStrategy
	cooldownService *CooldownService
}

func NewRouter(
	quotaSvc QuotaChecker,
	healthSvc HealthChecker,
	providerSvc ProviderCaller,
) *Router {
	cfg := config.LoadConfig()
	metrics.RegisterModelInfo(cfg)
	return &Router{
		config:          cfg,
		quotaService:    quotaSvc,
		healthService:   healthSvc,
		providerService: providerSvc,
		classifier:      NewDefaultFailureClassifier(),
		backoffStrategy: DefaultBackoffStrategy(),
		cooldownService: nil,
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
		config:          cfg,
		quotaService:    quotaSvc,
		healthService:   healthSvc,
		providerService: providerSvc,
		classifier:      NewDefaultFailureClassifier(),
		backoffStrategy: DefaultBackoffStrategy(),
		cooldownService: nil,
	}
}

// SetCooldownService sets the cooldown service (called after construction)
func (r *Router) SetCooldownService(cs *CooldownService) {
	r.cooldownService = cs
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
	}

	// Detect strict JSON
	if req.ResponseFormat != nil {
		if req.ResponseFormat.Type == "json_schema" &&
			req.ResponseFormat.JSONSchema != nil &&
			req.ResponseFormat.JSONSchema.Strict != nil &&
			*req.ResponseFormat.JSONSchema.Strict {
			requirements.Output = "json_schema_strict"
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

		// Check strict JSON requirement
		if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object" && !supportsJSONObject(caps) {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "json_output_not_supported"
			continue
		}

		if provider.ID == "cerebras" && req.ResponseFormat != nil && req.ResponseFormat.Type == "json_object" && req.Stream != nil && *req.Stream {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "json_object_streaming_not_supported"
			continue
		}

		if req.ResponseFormat != nil && req.ResponseFormat.Type == "json_schema" && !supportsJSONSchema(caps) {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "json_schema_not_supported"
			continue
		}

		if requirements.Output == "json_schema_strict" {
			if !candidate.IsCertifiedForStrictSchema {
				// Check if provider guarantees strict JSON
				if caps.StructuredOutputs != "json_schema_strict" {
					filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "not_certified_for_strict_json"
					continue
				}
			}
		}

		// Check streaming requirement
		if requirements.Streaming == "required" && !caps.Streaming {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "streaming_not_supported"
			continue
		}

		// Check tools requirement
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

		if req.Logprobs != nil && *req.Logprobs && !caps.Logprobs {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "logprobs_not_supported"
			continue
		}

		if req.N != nil && *req.N > 1 && !caps.MultipleChoices {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "multiple_choices_not_supported"
			continue
		}

		// Check circuit breaker
		if !r.healthService.CanExecute(ctx, provider.ID, model) {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "circuit_breaker_open"
			continue
		}

		// Check cooldown
		if r.cooldownService != nil && r.cooldownService.IsOnCooldown(ctx, provider.ID, model) {
			reason := r.cooldownService.GetCooldownReason(ctx, provider.ID, model)
			remaining := r.cooldownService.GetCooldownRemaining(ctx, provider.ID, model)
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = fmt.Sprintf("provider_cooldown_active:%s:%.0fs", reason, remaining.Seconds())
			continue
		}

		// Check per-model quota
		modelLimits := provider.Models.Limits[model]

		if provider.ID == cloudflareProviderID && hasCloudflareBudget {
			estimatedNeurons := cloudflareBudget.EstimateCloudflareRequestNeurons(model, req)
			if err := cloudflareBudget.CheckCloudflareDailyNeuronBudget(ctx, model, estimatedNeurons); err != nil {
				if quotaErr, ok := err.(*errors.ModelQuotaExceededError); ok {
					filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = fmt.Sprintf("quota_exceeded_%s", quotaErr.LimitType)
				} else {
					filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "cloudflare_budget_check_failed"
				}
				continue
			}
		}

		if err := r.quotaService.CheckModelQuota(ctx, provider.ID, model, modelLimits, estimatedTokens); err != nil {
			if quotaErr, ok := err.(*errors.ModelQuotaExceededError); ok {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = fmt.Sprintf("quota_exceeded_%s", quotaErr.LimitType)
			} else {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "quota_check_failed"
			}
			continue
		}

		if modelLimits.MaxConcurrent != nil && *modelLimits.MaxConcurrent > 0 {
			if !r.quotaService.CheckConcurrencyLimit(ctx, provider.ID, model, *modelLimits.MaxConcurrent) {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "concurrency_limit_exceeded"
				continue
			}
		}

		eligible = append(eligible, candidate)
	}

	return eligible, filtered
}

// Stage 4: Score Candidates
func (r *Router) ScoreCandidates(ctx context.Context, candidates []types.RoutingCandidate, hints *types.RouterHints) []types.RoutingCandidate {
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

		// Health score
		metrics := r.healthService.GetHealthMetrics(ctx, candidate.Provider.ID, candidate.Model)
		healthScore := metrics.HealthScore
		successRatioScore := calculateSuccessRatioScore(metrics.SuccessCount, metrics.FailureCount)
		candidate.ScoreBreakdown["health_score"] = healthScore
		candidate.ScoreBreakdown["success_ratio"] = successRatioScore

		// Combine scores
		candidate.Score = baseScore*0.5 + healthScore*0.5 + successRatioScore + candidate.Score
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
	// Determine max attempts
	maxAttempts := 3
	if hints != nil && hints.Fallback != nil && hints.Fallback.MaxAttempts != nil {
		maxAttempts = *hints.Fallback.MaxAttempts
	} else if tierSLO != nil && tierSLO.MaxAttempts != nil {
		maxAttempts = *tierSLO.MaxAttempts
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

		attempts = append(attempts, types.RoutingAttempt{
			ProviderID:   candidate.Provider.ID,
			Model:        candidate.Model,
			BaseURL:      candidate.Provider.BaseURL,
			APIKey:       apiKey,
			Score:        candidate.Score,
			TimeoutMs:    timeoutMs,
			ProviderType: candidate.Provider.ProviderType,
			Auth:         candidate.Provider.Auth,
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

		if plan.HardTimeoutMs != nil {
			if int(time.Since(startTime).Milliseconds()) > *plan.HardTimeoutMs {
				return nil, errors.NewTimeoutError("Hard timeout exceeded", "request")
			}
		}

		logger.Info().
			Str("type", "router").
			Str("event", "attempt.start").
			Str("request_id", requestID).
			Int("attempt", i+1).
			Str("provider", attempt.ProviderID).
			Str("model", attempt.Model).
			Float64("score", attempt.Score).
			Msg("Trying provider")

		concurrencyAcquired := false
		if limit := r.lookupModelConcurrencyLimit(attempt.ProviderID, attempt.Model); limit > 0 {
			if err := r.quotaService.AcquireConcurrencySlot(ctx, attempt.ProviderID, attempt.Model, limit); err != nil {
				logger.Warn().
					Str("type", "router").
					Str("event", "attempt.concurrency_denied").
					Str("request_id", requestID).
					Str("provider", attempt.ProviderID).
					Str("model", attempt.Model).
					Err(err).
					Msg("Concurrency slot unavailable")
				continue
			}
			concurrencyAcquired = true
		}

		attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(attempt.TimeoutMs)*time.Millisecond)

		resp, err := r.providerService.CallProvider(
			attempt.BaseURL,
			attempt.APIKey,
			attempt.Model,
			req,
			attempt.TimeoutMs,
			attemptCtx,
			attempt.ProviderType,
			attempt.Auth,
			requestID,
		)

		cancel()

		if concurrencyAcquired {
			r.quotaService.ReleaseConcurrencySlot(ctx, attempt.ProviderID, attempt.Model)
		}

		latencyMs := time.Since(startTime).Milliseconds()

		if err == nil {
		r.healthService.RecordSuccess(ctx, attempt.ProviderID, attempt.Model, int(latencyMs))

		if cooldownMs := r.lookupModelCooldownMs(attempt.ProviderID, attempt.Model); cooldownMs > 0 && r.cooldownService != nil {
			r.cooldownService.ApplyCooldownForReason(ctx, attempt.ProviderID, attempt.Model, "success", cooldownMs)
		}

		tokensUsed := 0
			var cloudflareStats *CloudflareUsageStats
			if resp.Usage != nil {
				tokensUsed = resp.Usage.TotalTokens
			} else {
				tokensUsed = r.quotaService.EstimateTokens(req)
			}
			r.quotaService.RecordModelUsage(ctx, attempt.ProviderID, attempt.Model, tokensUsed)
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
			).Observe(float64(latencyMs) / 1000.0)
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
				Int64("latency_ms", latencyMs).
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
			if quotaStatus := r.quotaService.GetModelQuotaStatus(ctx, attempt.ProviderID, attempt.Model, nil); quotaStatus.Rpm > 0 || quotaStatus.Tpm > 0 {
				if quotaStatus.Rpm > 0 {
					logEvent = logEvent.Int("quota_rpm", quotaStatus.Rpm)
				}
				if quotaStatus.Tpm > 0 {
					logEvent = logEvent.Int("quota_tpm", quotaStatus.Tpm)
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
				LatencyMs:  latencyMs,
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

		attemptChain = append(attemptChain, map[string]any{
			"provider":       attempt.ProviderID,
			"model":          attempt.Model,
			"failure_kind":   string(decision.Category),
			"failure_action": string(decision.Action),
			"failure_reason": decision.Reason,
		})

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
			Str("failure_category", string(decision.Category)).
			Str("failure_action", string(decision.Action)).
			Str("failure_reason", decision.Reason).
			Err(err).
			Msg("Provider attempt failed")

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
				select {
				case <-time.After(backoffDuration):
				case <-ctx.Done():
					return nil, errors.NewTimeoutError("Context cancelled during backoff", "request")
				}
			}
		case types.ActionFailover, types.ActionFailoverWithBackoff:
			if decision.BackoffMs > 0 {
				backoffDuration := r.backoffStrategy.CalculateBackoff(i)
				select {
				case <-time.After(backoffDuration):
				case <-ctx.Done():
					return nil, errors.NewTimeoutError("Context cancelled during backoff", "request")
				}
			}
		case types.ActionCooldown:
			if decision.CooldownSeconds > 0 && r.cooldownService != nil {
				reason := r.failureCategoryToCooldownReason(decision.Category)
				retryAfter := 0
				if rateLimitErr, ok := err.(*errors.RateLimitError); ok {
					retryAfter = rateLimitErr.RetryAfter
				}
				r.cooldownService.ApplyCooldownForReason(ctx, attempt.ProviderID, attempt.Model, reason, retryAfter)
			}
		}

		if rateLimitErr, ok := err.(*errors.RateLimitError); ok {
			r.maybeMarkCloudflareDailyBudgetExhausted(ctx, attempt.ProviderID, rateLimitErr)
			r.quotaService.HandleProviderRateLimit(ctx, attempt.ProviderID, attempt.Model, buildRateLimitResponse(rateLimitErr))
		}
	}

	metrics.RoutingAttemptsTotal.WithLabelValues(
		tier, strategy,
	).Observe(float64(len(plan.Attempts)))

	if allAttemptsRateLimited(attemptChain) {
		return nil, &types.GatewayError{
			Type:    "gateway_error",
			Code:    "RATE_LIMITED",
			Message: "All provider attempts failed due to rate limits or quota",
			Details: map[string]any{
				"attempts": attemptChain,
			},
		}
	}

	return nil, &types.GatewayError{
		Type:    "gateway_error",
		Code:    "ALL_ATTEMPTS_FAILED",
		Message: "All provider attempts failed",
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
	var failureKinds []types.FailureCategory
		var streamUsage *types.Usage

		for i, attempt := range plan.Attempts {
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

			if plan.HardTimeoutMs != nil {
				if int(time.Since(startTime).Milliseconds()) > *plan.HardTimeoutMs {
					errChan <- &types.GatewayError{
						Type:    "gateway_error",
						Code:    "HARD_TIMEOUT",
						Message: "Hard timeout exceeded",
					}
					return
				}
			}

		attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(attempt.TimeoutMs)*time.Millisecond)

		if limit := r.lookupModelConcurrencyLimit(attempt.ProviderID, attempt.Model); limit > 0 {
				if err := r.quotaService.AcquireConcurrencySlot(ctx, attempt.ProviderID, attempt.Model, limit); err != nil {
					logger.Warn().
						Str("type", "router").
						Str("event", "attempt.concurrency_denied").
						Str("request_id", requestID).
						Str("provider", attempt.ProviderID).
						Str("model", attempt.Model).
						Err(err).
						Msg("Concurrency slot unavailable")
				cancel()
				continue
			}
			defer func() {
				r.quotaService.ReleaseConcurrencySlot(ctx, attempt.ProviderID, attempt.Model)
			}()
		}

			result := r.providerService.StreamProviderChannel(
				attempt.BaseURL,
				attempt.APIKey,
				attempt.Model,
				req,
				attempt.TimeoutMs,
				attemptCtx,
				attempt.ProviderType,
				attempt.Auth,
				requestID,
			)

			outputTokenCount = 0
			streamUsage = nil
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
				chunksSent = true
				select {
				case chunks <- chunk:
				case <-ctx.Done():
					cancel()
					return
				}
			}

		err := <-result.Err
		cancel()

		latencyMs := time.Since(startTime).Milliseconds()

		if err == nil {
			r.healthService.RecordSuccess(ctx, attempt.ProviderID, attempt.Model, int(latencyMs))

			if cooldownMs := r.lookupModelCooldownMs(attempt.ProviderID, attempt.Model); cooldownMs > 0 && r.cooldownService != nil {
				r.cooldownService.ApplyCooldownForReason(ctx, attempt.ProviderID, attempt.Model, "success", cooldownMs)
			}

			tokensUsed := 0
				if streamUsage != nil && streamUsage.TotalTokens > 0 {
					tokensUsed = streamUsage.TotalTokens
				} else {
					tokensUsed = r.quotaService.EstimateTokens(req)
				}
				r.quotaService.RecordModelUsage(ctx, attempt.ProviderID, attempt.Model, tokensUsed)

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
				if quotaStatus := r.quotaService.GetModelQuotaStatus(ctx, attempt.ProviderID, attempt.Model, nil); quotaStatus.Rpm > 0 || quotaStatus.Tpm > 0 {
					if quotaStatus.Rpm > 0 {
						logEvent = logEvent.Int("quota_rpm", quotaStatus.Rpm)
					}
					if quotaStatus.Tpm > 0 {
						logEvent = logEvent.Int("quota_tpm", quotaStatus.Tpm)
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

	failureKinds = append(failureKinds, decision.Category)

	ttfbRecorded = false

			if err.Code == "RATE_LIMITED" {
				rateLimitErr := r.gatewayErrorToTypedError(err).(*errors.RateLimitError)
				r.maybeMarkCloudflareDailyBudgetExhausted(ctx, attempt.ProviderID, rateLimitErr)
				r.quotaService.HandleProviderRateLimit(ctx, attempt.ProviderID, attempt.Model, buildRateLimitResponse(rateLimitErr))
			}

			// Don't retry if we already sent chunks to the client
			if chunksSent {
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
					select {
					case <-time.After(backoffDuration):
					case <-ctx.Done():
						errChan <- &types.GatewayError{Type: "timeout_error", Code: "TIMEOUT", Message: "Context cancelled during backoff"}
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
					select {
					case <-time.After(backoffDuration):
					case <-ctx.Done():
						errChan <- &types.GatewayError{Type: "timeout_error", Code: "TIMEOUT", Message: "Context cancelled during backoff"}
						return
					}
				}
			case types.ActionCooldown:
				if decision.CooldownSeconds > 0 && r.cooldownService != nil {
					reason := r.failureCategoryToCooldownReason(decision.Category)
					retryAfter := 0
					if rateLimitErr, ok := typedErr.(*errors.RateLimitError); ok {
						retryAfter = rateLimitErr.RetryAfter
					}
					r.cooldownService.ApplyCooldownForReason(ctx, attempt.ProviderID, attempt.Model, reason, retryAfter)
				}
			}
	}

	if allStreamFailuresRateLimited(failureKinds) {
		errChan <- &types.GatewayError{
			Type:    "gateway_error",
			Code:    "RATE_LIMITED",
			Message: "All provider attempts failed due to rate limits or quota",
		}
	} else {
		errChan <- &types.GatewayError{
			Type:    "gateway_error",
			Code:    "ALL_ATTEMPTS_FAILED",
			Message: "All provider attempts failed",
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
	case "RATE_LIMITED":
		retryAfter := 60
		limitType := "rpm"
		headers := map[string]string{}
		if details, ok := ge.Details["retry_after"].(int); ok {
			retryAfter = details
		}
		if details, ok := ge.Details["limit_type"].(string); ok && details != "" {
			limitType = details
		}
		if details, ok := ge.Details["headers"].(map[string]any); ok {
			for key, value := range details {
				headers[key] = fmt.Sprintf("%v", value)
			}
		}
		if details, ok := ge.Details["headers"].(map[string]string); ok {
			headers = details
		}
		err := errors.NewRateLimitError(ge.Message, retryAfter, limitType)
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

func (r *Router) providerAvailable(provider types.ProviderConfig) bool {
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
	default:
		return &types.GatewayError{
			Type:    "upstream_error",
			Code:    "PROVIDER_ERROR",
			Message: err.Error(),
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
	case types.CategoryPayment:
		return CooldownBilling
	case types.CategoryProvider5xx:
		return CooldownOverload
	default:
		return CooldownDefault
	}
}

func (r *Router) lookupModelConcurrencyLimit(providerID, model string) int {
	for _, p := range r.config.Providers {
		if p.ID == providerID {
			if limits, ok := p.Models.Limits[model]; ok && limits.MaxConcurrent != nil {
				return *limits.MaxConcurrent
			}
			return 0
		}
	}
	return 0
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
