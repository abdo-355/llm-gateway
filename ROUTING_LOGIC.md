# 6-Stage Routing Pipeline

This document describes the intelligent routing algorithm used by the LLM Gateway.

## Overview

When a request arrives, the gateway executes a 6-stage pipeline to select the optimal provider and model. This ensures high availability, performance, and intelligent fallback.

```
Request → Stage 1 → Stage 2 → Stage 3 → Stage 4 → Stage 5 → Stage 6 → Provider
           ↓         ↓         ↓         ↓         ↓         ↓
        Requirements Candidates Filtering  Scoring   Plan     Execute
```

## Stage 1: Derive Requirements

**Purpose**: Understand what the request needs.

### Input
- `ChatCompletionRequest` with messages, model, etc.
- Optional `RouterHints` for overrides

### Processing
1. **Output Type Detection**
   ```go
   if response_format.type == "json_schema" && strict == true:
       requirements.Output = "json_schema_strict"
   else if response_format.type == "json_object":
       requirements.Output = "json_object"
   else:
       requirements.Output = "text"
   ```

2. **Streaming Detection**
   ```go
   if stream == true:
       requirements.Streaming = "required"
   else if stream == false:
       requirements.Streaming = "forbidden"
   else:
       requirements.Streaming = "preferred"
   ```

3. **Tools Detection**
   ```go
   if len(tools) > 0 && tool_choice != "none":
       requirements.Tools = "required"
   else if len(tools) == 0:
       requirements.Tools = "forbidden"
   else:
       requirements.Tools = "allowed"
   ```

### Output
```go
type DerivedRequirements struct {
    Output    string // "text", "json_object", "json_schema_strict"
    Streaming string // "required", "forbidden", "preferred"
    Tools     string // "required", "forbidden", "allowed"
}
```

## Stage 2: Generate Candidates

**Purpose**: Build list of all models that could handle this request.

### Processing
1. Expand logical model (e.g., "chat-pro") to concrete models
2. Get all providers that support each model
3. Create `RoutingCandidate` for each provider+model pair

### Example
```go
Logical Model: "chat-pro"
→ Candidates: [
    {Provider: "groq", Model: "llama-3.3-70b-versatile"},
    {Provider: "cerebras", Model: "llama-3.1-70b"},
    {Provider: "mistral", Model: "mistral-large"},
]
```

## Stage 3: Filter Candidates

**Purpose**: Remove candidates that can't handle the request.

### Filters Applied

#### 1. Capability Matching
```go
if requirements.Output == "json_schema_strict" &&
   !provider.Capabilities.StructuredOutputs == "json_schema_strict":
    filter(candidate, "no_strict_json_support")
```

#### 2. Allow/Deny Lists
```go
if hints.Providers.Allow != [] && !contains(hints.Providers.Allow, provider.ID):
    filter(candidate, "provider_not_in_allowlist")

if contains(hints.Providers.Deny, provider.ID):
    filter(candidate, "provider_in_denylist")
```

#### 3. Circuit Breaker
```go
if !healthService.CanExecute(ctx, provider.ID, model):
    filter(candidate, "circuit_breaker_open")
```

#### 4. Quota Check
```go
if quotaService.CheckModelQuota(...) != nil:
    filter(candidate, "quota_exceeded")
```

### Output
Filtered candidates list + map of filtered reasons for observability.

## Stage 4: Score & Sort

**Purpose**: Rank remaining candidates by preference and health.

### Scoring Algorithm

```go
score = baseWeight(1.0)

// Preference bonus (highest to lowest rank)
if provider in hints.Providers.Prefer:
    rank = index(hints.Providers.Prefer, provider)
    bonus = 0.20 * (1.0 - float64(rank)*0.05)
    score += bonus

// Health bonus
health = healthService.GetHealthMetrics(provider.ID, model)
score += health.HealthScore * 0.25
```

### Example
```go
Candidates after scoring:
1. groq/llama-3.3-70b → Score: 1.45 (0.20 pref bonus + 1.0 health)
2. cerebras/llama-3.1 → Score: 1.30 (0.15 pref bonus + 0.9 health)
3. mistral/mistral-large → Score: 1.10 (0.0 pref bonus + 0.8 health)
```

### Output
Sorted list of candidates (highest score first).

## Stage 5: Compile Plan

**Purpose**: Create execution plan with retry policy.

### Plan Structure
```go
type RoutingPlan struct {
    Attempts       []RoutingAttempt
    RetryOn429     bool
    RetryOnTimeout bool
    RetryOn5xx     bool
    HardTimeoutMs  *int
}

type RoutingAttempt struct {
    ProviderID string
    Model      string
    BaseURL    string
    APIKey     string
    TimeoutMs  int
    Score      float64
    Auth       ProviderAuth
}
```

### Processing
1. Select top N candidates (max_attempts from hints, default 3)
2. Set timeouts (SLO timeout or default 30s)
3. Configure retry policy
   - All retries enabled by default
   - Can override via hints (e.g., `hints.Fallback.On429 = false`)
4. Set hard timeout if specified

### Example Plan
```go
RoutingPlan{
    Attempts: [
        {Provider: "groq", Model: "llama-3.3-70b", TimeoutMs: 30000},
        {Provider: "cerebras", Model: "llama-3.1-70b", TimeoutMs: 30000},
        {Provider: "mistral", Model: "mistral-large", TimeoutMs: 30000},
    ],
    RetryOn429: true,
    RetryOnTimeout: true,
    RetryOn5xx: true,
}
```

## Stage 6: Execute

**Purpose**: Execute plan with automatic fallback.

### Algorithm
```go
func Execute(plan, request):
    for i, attempt := range plan.Attempts:
        response, err = callProvider(attempt, request)
        
        if err == nil:
            recordSuccess(attempt.Provider, attempt.Model)
            return response
        
        recordFailure(attempt.Provider, attempt.Model, err)
        
        if !shouldRetry(err, plan, i):
            return GatewayError{...}
    
    return GatewayError{"All providers failed", attempts: len(plan.Attempts)}
```

### Retry Logic
```go
func shouldRetry(err, plan, attemptIndex):
    if attemptIndex >= len(plan.Attempts)-1:
        return false  // Last attempt
    
    switch err.(type):
    case RateLimitError:
        return plan.RetryOn429
    case TimeoutError:
        return plan.RetryOnTimeout
    case ProviderError:
        return err.IsRetryable && plan.RetryOn5xx
    case CircuitBreakerError:
        return true  // Try different provider
    case ModelQuotaExceededError:
        return true  // Try different provider
    default:
        return false
```

### Error Handling
Creates `GatewayError` with:
- Error type (RATE_LIMITED, TIMEOUT, UPSTREAM_ERROR, etc.)
- Message from provider
- Details: attempts count, retry info

## Observability

### Response Headers
- `X-Gateway-Provider`: Provider used
- `X-Gateway-Model`: Model used
- `X-Gateway-Attempts`: Number of attempts

### Logs
```json
{
  "event": "attempt.start",
  "provider": "groq",
  "model": "llama-3.3-70b",
  "attempt": 1,
  "score": 1.45
}
{
  "event": "attempt.failed",
  "provider": "groq",
  "error": "rate limited",
  "attempt": 1
}
{
  "event": "attempt.success",
  "provider": "cerebras",
  "model": "llama-3.1-70b",
  "attempts": 2,
  "latency_ms": 1250
}
```

## Performance

- **Routing Decision**: <1ms for 100 candidates
- **Memory**: O(n) where n = number of candidates
- **Scalability**: Stateless, scales horizontally

## Configuration

See `internal/config/logical_models.go` for SLO and routing configuration.
