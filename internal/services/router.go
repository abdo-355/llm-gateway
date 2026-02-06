package services

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/rs/zerolog/log"
)

type Router struct {
	config          types.AppConfig
	quotaService    *QuotaService
	healthService   *HealthService
	providerService *ProviderService
}

func NewRouter(
	quotaSvc *QuotaService,
	healthSvc *HealthService,
	providerSvc *ProviderService,
) *Router {
	return &Router{
		config:          config.LoadConfig(),
		quotaService:    quotaSvc,
		healthService:   healthSvc,
		providerService: providerSvc,
	}
}

// Stage 1: Derive Requirements
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
		switch req.ToolChoice {
		case "required":
			requirements.Tools = "required"
		case "none":
			requirements.Tools = "forbidden"
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

func (r *Router) GenerateCandidatesFromLogicalModel(logicalModel *types.LogicalModelConfig) []types.RoutingCandidate {
	var candidates []types.RoutingCandidate

	for _, candidate := range logicalModel.Candidates {
		// Find provider config
		var provider *types.ProviderConfig
		for _, p := range r.config.Providers {
			if p.ID == candidate.Provider {
				provider = &p
				break
			}
		}

		if provider == nil {
			log.Warn().
				Str("type", "router").
				Str("event", "logical_model.provider_not_found").
				Str("provider", candidate.Provider).
				Str("logical_model", logicalModel.ID).
				Msg("Provider not found for logical model candidate")
			continue
		}

		// Check if model exists in provider
		found := slices.Contains(provider.Models.List, candidate.Model)

		if !found {
			log.Warn().
				Str("type", "router").
				Str("event", "logical_model.model_not_found").
				Str("provider", candidate.Provider).
				Str("model", candidate.Model).
				Msg("Model not found in provider for logical model candidate")
			continue
		}

		isCertified := r.isCertifiedForStrictSchema(candidate.Provider, candidate.Model)

		candidates = append(candidates, types.RoutingCandidate{
			Provider:                   *provider,
			Model:                      candidate.Model,
			IsCertifiedForStrictSchema: isCertified,
			Score:                      candidate.Weight,
			ScoreBreakdown: map[string]float64{
				"logical_model_weight": candidate.Weight,
			},
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

	for _, candidate := range candidates {
		provider := candidate.Provider
		model := candidate.Model

		// Check allow/deny lists
		if hints != nil && hints.Providers != nil {
			if len(hints.Providers.Allow) > 0 {
				found := slices.Contains(hints.Providers.Allow, provider.ID)
				if !found {
					filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "provider_not_in_allowlist"
					continue
				}
			}

			for _, p := range hints.Providers.Deny {
				if p == provider.ID {
					filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "provider_in_denylist"
					continue
				}
			}
		}

		// Check strict JSON requirement
		if requirements.Output == "json_schema_strict" {
			if !candidate.IsCertifiedForStrictSchema {
				// Check if provider guarantees strict JSON
				if provider.Capabilities.StructuredOutputs != "json_schema_strict" {
					filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "not_certified_for_strict_json"
					continue
				}
			}
		}

		// Check streaming requirement
		if requirements.Streaming == "required" && !provider.Capabilities.Streaming {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "streaming_not_supported"
			continue
		}

		// Check tools requirement
		if requirements.Tools == "required" && !provider.Capabilities.Tools {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "tools_not_supported"
			continue
		}

		// Check circuit breaker
		if !r.healthService.CanExecute(ctx, provider.ID) {
			filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "circuit_breaker_open"
			continue
		}

		// Check per-model quota
		modelLimits := provider.Models.Limits[model]
		estimatedTokens := r.quotaService.EstimateTokens(req)

		if err := r.quotaService.CheckModelQuota(ctx, provider.ID, model, modelLimits, estimatedTokens); err != nil {
			if quotaErr, ok := err.(*errors.ModelQuotaExceededError); ok {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = fmt.Sprintf("quota_exceeded_%s", quotaErr.LimitType)
			} else {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "quota_check_failed"
			}
			continue
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
		metrics := r.healthService.GetHealthMetrics(ctx, candidate.Provider.ID)
		healthScore := metrics.HealthScore
		candidate.ScoreBreakdown["health_score"] = healthScore

		// Combine scores
		candidate.Score = baseScore*0.5 + healthScore*0.5 + candidate.Score
	}

	// Sort by score (bubble sort for simplicity)
	for i := range candidates {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Score > candidates[i].Score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	return candidates
}

// Stage 5: Compile Plan
func (r *Router) CompilePlan(
	candidates []types.RoutingCandidate,
	hints *types.RouterHints,
	logicalModelSLO *types.LogicalModelSLO,
) types.RoutingPlan {
	// Determine max attempts
	maxAttempts := 3
	if hints != nil && hints.Fallback != nil && hints.Fallback.MaxAttempts != nil {
		maxAttempts = *hints.Fallback.MaxAttempts
	} else if logicalModelSLO != nil && logicalModelSLO.MaxAttempts != nil {
		maxAttempts = *logicalModelSLO.MaxAttempts
	}

	if maxAttempts > len(candidates) {
		maxAttempts = len(candidates)
	}

	// Determine timeout
	timeoutMs := 30000
	if hints != nil && hints.SLO != nil && hints.SLO.MaxLatencyMs != nil {
		timeoutMs = *hints.SLO.MaxLatencyMs
	} else if logicalModelSLO != nil && logicalModelSLO.MaxLatencyMs != nil {
		timeoutMs = *logicalModelSLO.MaxLatencyMs
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

		// Get API key from environment
		apiKey := ""
		switch candidate.Provider.Auth.Env {
		case "GROQ_API_KEY":
			apiKey = config.GetEnv().GroqAPIKey
		case "CEREBRAS_API_KEY":
			apiKey = config.GetEnv().CerebrasAPIKey
		case "MISTRAL_API_KEY":
			apiKey = config.GetEnv().MistralAPIKey
		case "GOOGLE_VERTEX_API_KEY":
			apiKey = config.GetEnv().GoogleVertexAPIKey
		}

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

	for i, attempt := range plan.Attempts {
		// Check hard timeout
		if plan.HardTimeoutMs != nil {
			if int(time.Since(startTime).Milliseconds()) > *plan.HardTimeoutMs {
				return nil, errors.NewTimeoutError("Hard timeout exceeded", "request")
			}
		}

		// Create context with timeout
		attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(attempt.TimeoutMs)*time.Millisecond)

		// Make request
		resp, err := r.providerService.CallProvider(
			attempt.BaseURL,
			attempt.APIKey,
			attempt.Model,
			req,
			attempt.TimeoutMs,
			attemptCtx,
			attempt.ProviderType,
			attempt.Auth,
		)

		cancel()

		latencyMs := time.Since(startTime).Milliseconds()

		if err == nil {
			// Success
			r.healthService.RecordSuccess(ctx, attempt.ProviderID, int(latencyMs))

			// Record token usage (estimate if not provided)
			tokensUsed := 0
			if resp.Usage != nil {
				tokensUsed = resp.Usage.TotalTokens
			} else {
				tokensUsed = r.quotaService.EstimateTokens(req)
			}
			r.quotaService.RecordModelUsage(ctx, attempt.ProviderID, attempt.Model, tokensUsed)

			return &types.ExecutionResult{
				Response:   *resp,
				Attempts:   i + 1,
				ProviderID: attempt.ProviderID,
				Model:      attempt.Model,
				LatencyMs:  latencyMs,
			}, nil
		}

		// Handle error
		r.healthService.RecordFailure(ctx, attempt.ProviderID)

		// Check if should retry
		if !r.ShouldRetry(err, plan, i) {
			return nil, r.CreateGatewayError(err, i+1, requestID)
		}

		// Handle rate limit
		if rateLimitErr, ok := err.(*errors.RateLimitError); ok {
			r.quotaService.HandleProviderRateLimit(ctx, attempt.ProviderID, attempt.Model, &http.Response{
				StatusCode: 429,
				Header:     http.Header{"Retry-After": []string{fmt.Sprintf("%d", rateLimitErr.RetryAfter)}},
			})
		}
	}

	return nil, &types.GatewayError{
		Type:    "gateway_error",
		Code:    "ALL_ATTEMPTS_FAILED",
		Message: "All provider attempts failed",
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

		for _, attempt := range plan.Attempts {
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

			result := r.providerService.StreamProviderChannel(
				attempt.BaseURL,
				attempt.APIKey,
				attempt.Model,
				req,
				attempt.TimeoutMs,
				attemptCtx,
				attempt.ProviderType,
				attempt.Auth,
			)

			for chunk := range result.Chunks {
				select {
				case chunks <- chunk:
				default:
				}
			}

			err := <-result.Err
			cancel()

			latencyMs := time.Since(startTime).Milliseconds()

			if err == nil {
				r.healthService.RecordSuccess(ctx, attempt.ProviderID, int(latencyMs))
				tokensUsed := r.quotaService.EstimateTokens(req)
				r.quotaService.RecordModelUsage(ctx, attempt.ProviderID, attempt.Model, tokensUsed)
				return
			}

			r.healthService.RecordFailure(ctx, attempt.ProviderID)

			if err.Code == "RATE_LIMITED" {
				r.quotaService.HandleProviderRateLimit(ctx, attempt.ProviderID, attempt.Model, &http.Response{
					StatusCode: 429,
					Header:     http.Header{"Retry-After": []string{fmt.Sprintf("%d", 1)}},
				})
			}
		}

		errChan <- &types.GatewayError{
			Type:    "gateway_error",
			Code:    "ALL_ATTEMPTS_FAILED",
			Message: "All provider attempts failed",
		}
	}()

	return types.StreamResult{
		Chunks: chunks,
		Err:    errChan,
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
	default:
		return false
	}
}

// CreateGatewayError creates a gateway error from a provider error
func (r *Router) CreateGatewayError(err error, attempts int, requestID string) *types.GatewayError {
	switch e := err.(type) {
	case *errors.RateLimitError:
		return &types.GatewayError{
			Type:    "rate_limit_error",
			Code:    "RATE_LIMITED",
			Message: e.Error(),
			Details: map[string]any{
				"retry_after": e.RetryAfter,
				"limit_type":  e.LimitType,
				"attempts":    attempts,
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

func (r *Router) isCertifiedForStrictSchema(providerID, model string) bool {
	for _, cert := range r.config.Certifications {
		if cert.Provider == providerID && cert.Model == model && cert.StrictSchema {
			return true
		}
	}
	return false
}
