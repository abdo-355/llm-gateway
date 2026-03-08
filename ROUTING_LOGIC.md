# 6-Stage Routing Pipeline

This document describes how the router selects which LLM provider to use for each request.

```
Request → Stage 1 → Stage 2 → Stage 3 → Stage 4 → Stage 5 → Stage 6 → Provider
           ↓         ↓         ↓         ↓         ↓         ↓
        Derive   Generate   Filter    Score    Compile   Execute
       Req's     Candidates           Candidates  Plan
```

## Data Structures

### DerivedRequirements

What the request needs (detected from request fields, can be overridden by hints).

```go
type DerivedRequirements struct {
    Output    string // text, json_schema_strict
    Streaming string // required, preferred, forbidden
    Tools     string // required, allowed, forbidden
}
```

### RouterHints

Optional overrides from the client.

```go
type RouterHints struct {
    Profile      *string              // e.g., "cheap_fast", "reliable_structured"
    Requirements *RouterRequirements  // override derived requirements
    Budget       *BudgetConfig        // e.g., free_only mode
    SLO          *SLOConfig           // timeout settings
    Providers    *ProviderPreferences  // allow, deny, prefer
    Fallback     *FallbackConfig      // retry behavior
    Trace        *TraceConfig         // request tracing
}

type ProviderPreferences struct {
    Allow  []string // only use these providers
    Deny   []string // don't use these providers
    Prefer []string // in this order (first is highest priority)
}

type FallbackConfig struct {
    MaxAttempts *int  // max providers to try (default 3)
    On429       *bool // retry on rate limit
    OnTimeout   *bool // retry on timeout
    On5xx       *bool // retry on server errors
}
```

### RoutingCandidate

A provider/model pair that could handle the request.

```go
type RoutingCandidate struct {
    Provider                   ProviderConfig
    Model                      string
    IsCertifiedForStrictSchema bool // strict JSON certification
    Score                      float64
    ScoreBreakdown             map[string]float64 // for debugging
}
```

### RoutingPlan

The execution plan with retry policy.

```go
type RoutingPlan struct {
    Attempts       []RoutingAttempt
    MaxAttempts    int
    HardTimeoutMs  *int  // total time allowed for entire request
    RetryOn429     bool
    RetryOnTimeout bool
    RetryOn5xx     bool
}

type RoutingAttempt struct {
    ProviderID   string
    Model        string
    BaseURL      string
    APIKey       string
    Score        float64
    TimeoutMs    int
    ProviderType string // "openai" or "vertex"
    Auth         ProviderAuth
}
```

---

## Stage 1: Derive Requirements

**Purpose**: Figure out what the request needs by examining request fields.

### Detection Logic

**Output Format (JSON):**

```go
if req.ResponseFormat != nil &&
   req.ResponseFormat.Type == "json_schema" &&
   req.ResponseFormat.JSONSchema != nil &&
   req.ResponseFormat.JSONSchema.Strict != nil &&
   *req.ResponseFormat.JSONSchema.Strict {
    requirements.Output = "json_schema_strict"
}
```

**Streaming:**

```go
if req.Stream != nil {
    if *req.Stream {
        requirements.Streaming = "required"
    } else {
        requirements.Streaming = "forbidden"
    }
}
```

**Tools/Functions:**

```go
if len(req.Tools) > 0 {
    switch req.ToolChoice {
    case "required": requirements.Tools = "required"
    case "none":      requirements.Tools = "forbidden"
    default:          requirements.Tools = "allowed"
    }
}
```

### RouterHints Override

Hints can override any derived value:

```go
if hints != nil && hints.Requirements != nil {
    if hints.Requirements.Output != nil {
        requirements.Output = *hints.Requirements.Output
    }
    // ... same for Streaming and Tools
}
```

### Output

`DerivedRequirements` struct with Output, Streaming, and Tools fields.

---

## Stage 2: Generate Candidates

**Purpose**: Build a list of provider/model pairs that could potentially handle the request.

There are two ways to generate candidates:

### Path A: From Logical Model (Most Common)

Uses the logical model configuration (e.g., "chat-pro") to find candidates.

```go
func (r *Router) GenerateCandidatesFromLogicalModel(logicalModel *LogicalModelConfig) []RoutingCandidate {
    var candidates []RoutingCandidate

    for _, candidate := range logicalModel.Candidates {
        // Find provider config by ID
        provider := findProvider(candidate.Provider)

        // Check if model exists in provider
        if !slices.Contains(provider.Models.List, candidate.Model) {
            log.Warn("Model not found in provider")
            continue
        }

        // Check if certified for strict JSON
        isCertified := r.isCertifiedForStrictSchema(candidate.Provider, candidate.Model)

        candidates = append(candidates, RoutingCandidate{
            Provider:                   *provider,
            Model:                      candidate.Model,
            IsCertifiedForStrictSchema: isCertified,
            Score:                      candidate.Weight, // from logical model config
            ScoreBreakdown: map[string]float64{
                "logical_model_weight": candidate.Weight,
            },
        })
    }

    return candidates
}
```

### Path B: All Available Models

Generates candidates for every model in every provider (used when no logical model is specified).

```go
func (r *Router) GenerateCandidates() []RoutingCandidate {
    var candidates []RoutingCandidate

    for _, provider := range r.config.Providers {
        for _, model := range provider.Models.List {
            isCertified := r.isCertifiedForStrictSchema(provider.ID, model)

            candidates = append(candidates, RoutingCandidate{
                Provider:                   provider,
                Model:                      model,
                IsCertifiedForStrictSchema: isCertified,
                Score:                      0, // will be set in scoring
                ScoreBreakdown:             make(map[string]float64),
            })
        }
    }

    return candidates
}
```

### Output

List of `RoutingCandidate` structs, one per provider/model combination.

---

## Stage 3: Filter Candidates

**Purpose**: Remove candidates that can't handle the request.

### Filters (in order)

**1. Allow/Deny Lists:**

```go
if hints != nil && hints.Providers != nil {
    if len(hints.Providers.Allow) > 0 {
        if !slices.Contains(hints.Providers.Allow, provider.ID) {
            filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "provider_not_in_allowlist"
            continue
        }
    }
    if slices.Contains(hints.Providers.Deny, provider.ID) {
        filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "provider_in_denylist"
        continue
    }
}
```

**2. Strict JSON Requirement:**

```go
if requirements.Output == "json_schema_strict" {
    // First check: is this specific model certified for strict JSON?
    if !candidate.IsCertifiedForStrictSchema {
        // Second check: does provider guarantee strict JSON capability?
        if provider.Capabilities.StructuredOutputs != "json_schema_strict" {
            filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "not_certified_for_strict_json"
            continue
        }
    }
}
```

**3. Streaming Support:**

```go
if requirements.Streaming == "required" && !provider.Capabilities.Streaming {
    filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "streaming_not_supported"
    continue
}
```

**4. Tools Support:**

```go
if requirements.Tools == "required" && !provider.Capabilities.Tools {
    filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "tools_not_supported"
    continue
}
```

**5. Circuit Breaker:**

```go
if !r.healthService.CanExecute(ctx, provider.ID, model) {
    filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "circuit_breaker_open"
    continue
}
```

**6. Quota Check:**

```go
modelLimits := provider.Models.Limits[model]
estimatedTokens := r.quotaService.EstimateTokens(req)

if err := r.quotaService.CheckModelQuota(ctx, provider.ID, model, modelLimits, estimatedTokens); err != nil {
    if quotaErr, ok := err.(*ModelQuotaExceededError); ok {
        filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = fmt.Sprintf("quota_exceeded_%s", quotaErr.LimitType)
    } else {
        filtered[fmt.Sprintf("%s/%s", provider.ID, model)] = "quota_check_failed"
    }
    continue
}
```

### Output

- `eligible`: List of candidates that can handle the request
- `filtered`: Map of `provider/model` → `reason` for observability

---

## Stage 4: Score Candidates

**Purpose**: Rank remaining candidates by preference and health.

### Scoring Formula

```go
score = (baseScore * 0.5) + (healthScore * 0.5) + logicalModelWeight
```

**Preference Bonus:**

```go
if hints != nil && hints.Providers != nil {
    for j, pref := range hints.Providers.Prefer {
        if pref == candidate.Provider.ID {
            // First preference gets 0.5 bonus, decays with each position
            bonus := 0.5 * (1.0 - float64(j)/float64(len(hints.Providers.Prefer)))
            baseScore += bonus
            candidate.ScoreBreakdown["preference_bonus"] = bonus
            break
        }
    }
}
```

**Health Score:**

```go
metrics := r.healthService.GetHealthMetrics(ctx, candidate.Provider.ID, candidate.Model)
healthScore := metrics.HealthScore // 0.0 to 1.0
candidate.ScoreBreakdown["health_score"] = healthScore
```

**Sorting:**

```go
slices.SortFunc(candidates, func(a, b RoutingCandidate) int {
    if a.Score > b.Score {
        return -1 // a comes first
    }
    if a.Score < b.Score {
        return 1 // b comes first
    }
    return 0
})
```

### Output

Candidates sorted by score (highest first).

---

## Stage 5: Compile Plan

**Purpose**: Create an execution plan with retry policy and timeouts.

### Steps

**1. Determine Max Attempts:**

```go
maxAttempts := 3 // default
if hints != nil && hints.Fallback != nil && hints.Fallback.MaxAttempts != nil {
    maxAttempts = *hints.Fallback.MaxAttempts
} else if logicalModelSLO != nil && logicalModelSLO.MaxAttempts != nil {
    maxAttempts = *logicalModelSLO.MaxAttempts
}
```

**2. Determine Timeout:**

```go
timeoutMs := 30000 // 30 seconds default
if hints != nil && hints.SLO != nil && hints.SLO.MaxLatencyMs != nil {
    timeoutMs = *hints.SLO.MaxLatencyMs
} else if logicalModelSLO != nil && logicalModelSLO.MaxLatencyMs != nil {
    timeoutMs = *logicalModelSLO.MaxLatencyMs
}
```

**3. Build Attempts List:**

```go
var attempts []RoutingAttempt
for i := 0; i < maxAttempts && i < len(candidates); i++ {
    candidate := candidates[i]

    // Get API key from environment based on provider
    apiKey := ""
    switch candidate.Provider.Auth.Env {
    case "GROQ_API_KEY":         apiKey = config.GetEnv().GroqAPIKey
    case "CEREBRAS_API_KEY":     apiKey = config.GetEnv().CerebrasAPIKey
    case "MISTRAL_API_KEY":      apiKey = config.GetEnv().MistralAPIKey
    case "GEMINI_API_KEY":       apiKey = config.GetEnv().GeminiAPIKey
    case "GOOGLE_VERTEX_API_KEY": apiKey = config.GetEnv().GoogleVertexAPIKey
    }

    attempts = append(attempts, RoutingAttempt{
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
```

**4. Configure Retry Policy:**

```go
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
```

### Output

`RoutingPlan` with attempts list, timeouts, and retry policy.

---

## Stage 6: Execute

**Purpose**: Try providers in order until one succeeds or all fail

### Algorithm

```go
func (r *Router) Execute(ctx, plan, req, requestID) (*ExecutionResult, error) {
    startTime := time.Now()

    for i, attempt := range plan.Attempts {
        // Check hard timeout (total request time)
        if plan.HardTimeoutMs != nil {
            if int(time.Since(startTime).Milliseconds()) > *plan.HardTimeoutMs {
                return nil, NewTimeoutError("Hard timeout exceeded", "request")
            }
        }

        log.Info().
            Str("request_id", requestID).
            Int("attempt", i+1).
            Str("provider", attempt.ProviderID).
            Str("model", attempt.Model).
            Float64("score", attempt.Score).
            Msg("Trying provider")

        // Call provider with timeout
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
            // Success
            r.healthService.RecordSuccess(ctx, attempt.ProviderID, attempt.Model, int(latencyMs))
            tokensUsed := countTokens(resp) // or estimate from request
            r.quotaService.RecordModelUsage(ctx, attempt.ProviderID, attempt.Model, tokensUsed)

            log.Info().
                Str("request_id", requestID).
                Str("provider", attempt.ProviderID).
                Str("model", attempt.Model).
                Int64("latency_ms", latencyMs).
                Int("tokens", tokensUsed).
                Int("attempts", i+1).
                Msg("Request completed")

            return &ExecutionResult{
                Response:   *resp,
                Attempts:   i + 1,
                ProviderID: attempt.ProviderID,
                Model:      attempt.Model,
                LatencyMs:  latencyMs,
            }, nil
        }

        // Failure - record and decide whether to retry
        r.healthService.RecordFailure(ctx, attempt.ProviderID, attempt.Model)

        if !r.ShouldRetry(err, plan, i) {
            return nil, r.CreateGatewayError(err, i+1, requestID)
        }

        // Handle rate limit - update quota tracking
        if rateLimitErr, ok := err.(*RateLimitError); ok {
            r.quotaService.HandleProviderRateLimit(ctx, attempt.ProviderID, attempt.Model, &http.Response{
                StatusCode: 429,
                Header:     http.Header{"Retry-After": []string{fmt.Sprintf("%d", rateLimitErr.RetryAfter)}},
            })
        }
    }

    return nil, &GatewayError{
        Type:    "gateway_error",
        Code:    "ALL_ATTEMPTS_FAILED",
        Message: "All provider attempts failed",
    }
}
```

### ShouldRetry Logic

```go
func (r *Router) ShouldRetry(err error, plan RoutingPlan, attemptIndex int) bool {
    // No more attempts left
    if attemptIndex >= len(plan.Attempts)-1 {
        return false
    }

    switch e := err.(type) {
    case *RateLimitError:
        return plan.RetryOn429
    case *TimeoutError:
        return plan.RetryOnTimeout
    case *ProviderError:
        return e.IsRetryable && plan.RetryOn5xx
    case *CircuitBreakerError:
        return true // Always try different provider
    case *ModelQuotaExceededError:
        return true // Always try different provider
    case *PaymentRequiredError:
        return false // Won't work on retry
    case *ValidationError:
        return false // Request is invalid
    default:
        return false
    }
}
```

---

## Error Types

| Error Type                | Code                 | Retry?       | Meaning                                           |
| ------------------------- | -------------------- | ------------ | ------------------------------------------------- |
| `RateLimitError`          | RATE_LIMITED         | Configurable | Hit rate limit, retry after delay                 |
| `TimeoutError`            | TIMEOUT              | Configurable | Request timed out                                 |
| `ProviderError`           | PROVIDER_ERROR       | Configurable | Provider returned error, depends on `IsRetryable` |
| `CircuitBreakerError`     | CIRCUIT_BREAKER_OPEN | Yes          | Provider temporarily blocked                      |
| `ModelQuotaExceededError` | QUOTA_EXCEEDED       | Yes          | Model-specific quota exceeded                     |
| `PaymentRequiredError`    | PAYMENT_REQUIRED     | No           | Subscription/account issue                        |
| `ValidationError`         | VALIDATION_ERROR     | No           | Invalid request format                            |

---

## Streaming Execution

For streaming requests, the router uses `ExecuteStream` which:

1. Starts a goroutine to iterate through attempts
2. Pipes SSE chunks from provider directly to response channel
3. Falls back on error without waiting for complete response
4. Returns chunks as they arrive from the successful provider

```go
func (r *Router) ExecuteStream(ctx, plan, req, requestID) StreamResult {
    chunks := make(chan *SSEChunk)
    errChan := make(chan *GatewayError, 1)

    go func() {
        startTime := time.Now()
        for _, attempt := range plan.Attempts {
            // ... similar to Execute but streams chunks
            result := r.providerService.StreamProviderChannel(...)
            for chunk := range result.Chunks {
                chunks <- chunk
            }
            // on error, continue to next attempt
        }
        errChan <- GatewayError{Code: "ALL_ATTEMPTS_FAILED"}
    }()

    return StreamResult{Chunks: chunks, Err: errChan}
}
```

---

## Response Headers

| Header               | Example                   | Meaning                   |
| -------------------- | ------------------------- | ------------------------- |
| `X-Gateway-Provider` | `groq`                    | Provider used             |
| `X-Gateway-Model`    | `llama-3.3-70b-versatile` | Model used                |
| `X-Gateway-Attempts` | `2`                       | Number of providers tried |
