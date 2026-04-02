package services

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
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
}

func NewRouter(
	quotaSvc QuotaChecker,
	healthSvc HealthChecker,
	providerSvc ProviderCaller,
) *Router {
	return &Router{
		config:          config.LoadConfig(),
		quotaService:    quotaSvc,
		healthService:   healthSvc,
		providerService: providerSvc,
	}
}

func NewRouterWithConfig(
	cfg types.AppConfig,
	quotaSvc QuotaChecker,
	healthSvc HealthChecker,
	providerSvc ProviderCaller,
) *Router {
	return &Router{
		config:          cfg,
		quotaService:    quotaSvc,
		healthService:   healthSvc,
		providerService: providerSvc,
	}
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
			logger.Warn().
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
			logger.Warn().
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

			if slices.Contains(hints.Providers.Deny, provider.ID) {
				filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "provider_in_denylist"
				continue
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
		if !r.healthService.CanExecute(ctx, provider.ID, model) {
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
		metrics := r.healthService.GetHealthMetrics(ctx, candidate.Provider.ID, candidate.Model)
		healthScore := metrics.HealthScore
		candidate.ScoreBreakdown["health_score"] = healthScore

		// Combine scores
		candidate.Score = baseScore*0.5 + healthScore*0.5 + candidate.Score
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
	timeoutMs := defaultRequestTimeoutMs
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
		case "GEMINI_API_KEY":
			apiKey = config.GetEnv().GeminiAPIKey
		}

		// Build the base URL, replacing placeholders for Vertex
		baseURL := candidate.Provider.BaseURL
		if candidate.Provider.ID == "vertex" {
			baseURL = strings.ReplaceAll(baseURL, "PROJECT_ID", config.GetEnv().GoogleVertexProjectID)
		}

		attempts = append(attempts, types.RoutingAttempt{
			ProviderID:   candidate.Provider.ID,
			Model:        candidate.Model,
			BaseURL:      baseURL,
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

	logicalModel := metrics.GetLogicalModel(ctx)
	routerProfile := metrics.GetRouterProfile(ctx)

	var previousProvider string
	var hadFailure bool

	for i, attempt := range plan.Attempts {
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
		)

		cancel()

		latencyMs := time.Since(startTime).Milliseconds()

		if err == nil {
			r.healthService.RecordSuccess(ctx, attempt.ProviderID, attempt.Model, int(latencyMs))

			tokensUsed := 0
			if resp.Usage != nil {
				tokensUsed = resp.Usage.TotalTokens
			} else {
				tokensUsed = r.quotaService.EstimateTokens(req)
			}
			r.quotaService.RecordModelUsage(ctx, attempt.ProviderID, attempt.Model, tokensUsed)

			if hadFailure {
				metrics.RetrySuccessTotal.WithLabelValues(logicalModel).Inc()
			}

			metrics.ProviderRequestsTotal.WithLabelValues(
				attempt.ProviderID, attempt.Model, "success",
				logicalModel, routerProfile, "",
			).Inc()
			metrics.ProviderLatencySeconds.WithLabelValues(
				attempt.ProviderID, attempt.Model,
				logicalModel, routerProfile,
			).Observe(float64(latencyMs) / 1000.0)
			if resp.Usage != nil {
				metrics.ProviderTokensTotal.WithLabelValues(
					attempt.ProviderID, attempt.Model, "prompt", logicalModel, routerProfile,
				).Add(float64(resp.Usage.PromptTokens))
				metrics.ProviderTokensTotal.WithLabelValues(
					attempt.ProviderID, attempt.Model, "completion", logicalModel, routerProfile,
				).Add(float64(resp.Usage.CompletionTokens))
				metrics.ProviderTokensTotal.WithLabelValues(
					attempt.ProviderID, attempt.Model, "total", logicalModel, routerProfile,
				).Add(float64(resp.Usage.TotalTokens))
			}
			metrics.RoutingAttemptsTotal.WithLabelValues(
				logicalModel, routerProfile,
			).Observe(float64(i + 1))

			logger.Info().
				Str("type", "router").
				Str("event", "attempt.success").
				Str("request_id", requestID).
				Str("provider", attempt.ProviderID).
				Str("model", attempt.Model).
				Int64("latency_ms", latencyMs).
				Int("tokens", tokensUsed).
				Int("attempts", i+1).
				Msg("Request completed")

			return &types.ExecutionResult{
				Response:   *resp,
				Attempts:   i + 1,
				ProviderID: attempt.ProviderID,
				Model:      attempt.Model,
				LatencyMs:  latencyMs,
			}, nil
		}

		r.healthService.RecordFailure(ctx, attempt.ProviderID, attempt.Model)

		if previousProvider != "" {
			metrics.FailoverEventsTotal.WithLabelValues(
				previousProvider, attempt.ProviderID, logicalModel,
			).Inc()
		}
		previousProvider = attempt.ProviderID
		hadFailure = true

		var status string
		var errorType string
		switch err.(type) {
		case *errors.RateLimitError:
			status = "rate_limited"
			errorType = "rate_limit"
		case *errors.TimeoutError:
			status = "timeout"
			errorType = "timeout"
		case *errors.CircuitBreakerError:
			status = "circuit_breaker"
			errorType = "circuit_breaker"
		case *errors.ModelQuotaExceededError:
			status = "quota_exceeded"
			errorType = "quota_exceeded"
		case *errors.PaymentRequiredError:
			status = "payment_required"
			errorType = "payment_required"
		case *errors.ValidationError:
			status = "validation"
			errorType = "validation"
		default:
			status = "error"
			errorType = "unknown"
		}
		metrics.ProviderRequestsTotal.WithLabelValues(
			attempt.ProviderID, attempt.Model, status,
			logicalModel, routerProfile, errorType,
		).Inc()

		logger.Warn().
			Str("type", "router").
			Str("event", "attempt.failed").
			Str("request_id", requestID).
			Str("provider", attempt.ProviderID).
			Str("model", attempt.Model).
			Err(err).
			Msg("Provider attempt failed")

		if !r.ShouldRetry(err, plan, i) {
			return nil, r.CreateGatewayError(err, i+1, requestID)
		}

		if rateLimitErr, ok := err.(*errors.RateLimitError); ok {
			r.quotaService.HandleProviderRateLimit(ctx, attempt.ProviderID, attempt.Model, &http.Response{
				StatusCode: 429,
				Header:     http.Header{"Retry-After": []string{fmt.Sprintf("%d", rateLimitErr.RetryAfter)}},
			})
		}
	}

	metrics.RoutingAttemptsTotal.WithLabelValues(
		logicalModel, routerProfile,
	).Observe(float64(len(plan.Attempts)))

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
		var ttfbRecorded bool

		logicalModel := metrics.GetLogicalModel(ctx)
		routerProfile := metrics.GetRouterProfile(ctx)

		var previousProvider string
		var hadFailure bool
		var chunksSent bool
		var outputTokenCount int

		for i, attempt := range plan.Attempts {
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

			outputTokenCount = 0
			for chunk := range result.Chunks {
				if !ttfbRecorded {
					metrics.StreamTTFBSeconds.WithLabelValues(
						attempt.ProviderID, attempt.Model, logicalModel, routerProfile,
					).Observe(time.Since(startTime).Seconds())
					ttfbRecorded = true
				}
				if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != nil {
					outputTokenCount++
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
				tokensUsed := r.quotaService.EstimateTokens(req)
				r.quotaService.RecordModelUsage(ctx, attempt.ProviderID, attempt.Model, tokensUsed)

				if hadFailure {
					metrics.RetrySuccessTotal.WithLabelValues(logicalModel).Inc()
				}

				metrics.ProviderRequestsTotal.WithLabelValues(
					attempt.ProviderID, attempt.Model, "success",
					logicalModel, routerProfile, "",
				).Inc()
				metrics.ProviderLatencySeconds.WithLabelValues(
					attempt.ProviderID, attempt.Model,
					logicalModel, routerProfile,
				).Observe(float64(latencyMs) / 1000.0)
				metrics.StreamDurationSeconds.WithLabelValues(
					attempt.ProviderID, attempt.Model, logicalModel, routerProfile,
				).Observe(float64(latencyMs) / 1000.0)
				metrics.StreamOutputTokensTotal.WithLabelValues(
					attempt.ProviderID, attempt.Model, logicalModel, routerProfile,
				).Add(float64(outputTokenCount))
				metrics.RoutingAttemptsTotal.WithLabelValues(
					logicalModel, routerProfile,
				).Observe(float64(i + 1))
				errChan <- nil
				return
			}

			r.healthService.RecordFailure(ctx, attempt.ProviderID, attempt.Model)

			if previousProvider != "" {
				metrics.FailoverEventsTotal.WithLabelValues(
					previousProvider, attempt.ProviderID, logicalModel,
				).Inc()
			}
			previousProvider = attempt.ProviderID
			hadFailure = true

			var status string
			var errorType string
			switch err.Code {
			case "RATE_LIMITED":
				status = "rate_limited"
				errorType = "rate_limit"
			case "TIMEOUT", "HARD_TIMEOUT":
				status = "timeout"
				errorType = "timeout"
			default:
				status = "error"
				errorType = "unknown"
			}
			metrics.ProviderRequestsTotal.WithLabelValues(
				attempt.ProviderID, attempt.Model, status,
				logicalModel, routerProfile, errorType,
			).Inc()
			ttfbRecorded = false

			if err.Code == "RATE_LIMITED" {
				r.quotaService.HandleProviderRateLimit(ctx, attempt.ProviderID, attempt.Model, &http.Response{
					StatusCode: 429,
					Header:     http.Header{"Retry-After": []string{fmt.Sprintf("%d", 1)}},
				})
			}

			// Don't retry if we already sent chunks to the client
			if chunksSent {
				errChan <- r.CreateGatewayError(r.gatewayErrorToTypedError(err), i+1, requestID)
				return
			}

			typedErr := r.gatewayErrorToTypedError(err)
			if !r.ShouldRetry(typedErr, plan, i) {
				errChan <- r.CreateGatewayError(typedErr, i+1, requestID)
				return
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

// gatewayErrorToTypedError converts a GatewayError to the appropriate typed error for ShouldRetry
func (r *Router) gatewayErrorToTypedError(ge *types.GatewayError) error {
	if ge == nil {
		return nil
	}

	switch ge.Code {
	case "RATE_LIMITED":
		retryAfter := 60
		if details, ok := ge.Details["retry_after"].(int); ok {
			retryAfter = details
		}
		return errors.NewRateLimitError(ge.Message, retryAfter, "rpm")
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
