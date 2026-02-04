# LLM Gateway Routing Logic - Free Tier

This document explains the routing logic used by the LLM Gateway to select the best provider and model for each request. All providers operate on free tier limits - no costs or payments are tracked.

## Table of Contents

1. [Overview](#overview)
2. [The Routing Pipeline](#the-routing-pipeline)
3. [Stage 1: Request Normalization](#stage-1-request-normalization)
4. [Stage 2: Candidate Generation](#stage-2-candidate-generation)
5. [Stage 3: Hard Filtering](#stage-3-hard-filtering)
6. [Stage 4: Soft Scoring](#stage-4-soft-scoring)
7. [Stage 5: Plan Compilation](#stage-5-plan-compilation)
8. [Stage 6: Execution with Failover](#stage-6-execution-with-failover)
9. [Per-Model Quota System](#per-model-quota-system)
10. [Error Classes](#error-classes)
11. [Example Scenarios](#example-scenarios)
12. [Edge Cases and Special Handling](#edge-cases-and-special-handling)

---

## Overview

The routing system acts as an intelligent proxy between your application and multiple LLM providers (Groq, Cerebras, Mistral, Vertex). For each incoming request, it performs a multi-stage analysis to determine the optimal provider based on:

- **Request requirements**: streaming, structured output, tools
- **Provider capabilities**: supported features, model availability
- **Health status**: circuit breaker state, error rates
- **Quota availability**: per-model rate limits (RPM, RPH, RPD, TPM, TPH, TPD, TPMU)

**Current Provider Model Counts:**

- **Groq**: 9 models (Llama 3.3/3.1/4, Kimi K2, GPT-OSS, Qwen3)
- **Cerebras**: 6 models (Llama 3.3/3.1, GPT-OSS, Qwen3, ZAI GLM)
- **Mistral**: 18 models (Large, Medium, Small, Codestral, Ministral, Open Mixtral, etc.)
- **Vertex**: 2 models (Gemini 3 Pro/Flash Preview)

---

## The Routing Pipeline

```
Stage 1: Requirement Derivation
  - Parse OpenAI request
  - Extract explicit hints
  - Derive implicit requirements
  - Estimate token count for quota checking

Stage 2: Candidate Generation
  - Enumerate all (provider, model) pairs
  - Load per-model limit configurations
  - Load certification status
  - Identify provider type (openai vs vertex)

Stage 3: Hard Filtering
  - Remove ineligible candidates
  - Check capabilities (strict schema, streaming, tools)
  - Check per-model quotas (7 time windows)
  - Check circuit breakers

Stage 4: Soft Scoring
  - Calculate score for each candidate
  - Consider: health, preference, quota headroom
  - Sort by score

Stage 5: Plan Compilation
  - Select top N candidates
  - Set per-attempt timeouts
  - Configure retry policy

Stage 6: Execution with Failover
  - Check per-model quota before request
  - Attempt primary candidate
  - Record actual token usage after success
  - If failure is retryable, try next candidate
  - Return successful response or final error
```

---

## Stage 1: Request Normalization

### Input

The gateway receives an OpenAI-compatible request with optional routing hints:

```json
{
  "model": "llama-3.3-70b",
  "messages": [{ "role": "user", "content": "Hello" }],
  "stream": true,
  "max_tokens": 1000,
  "response_format": {
    "type": "json_schema",
    "json_schema": { "strict": true }
  },
  "router": {
    "providers": { "prefer": ["groq"] }
  }
}
```

### Derivation Logic

**Output Requirement:**

- If `response_format.type` equals "json_schema" AND `json_schema.strict` is true
  - Then output requirement = "json_schema_strict"
  - Else output requirement = "text"

**Streaming Requirement:**

- If `stream` is true → "required"
- If `stream` is false → "forbidden"
- Else → "preferred"

**Tools Requirement:**

- If `tools` array is present AND `tool_choice` equals "required" → "required"
- If `tools` array is present AND `tool_choice` equals "none" → "forbidden"
- If `tools` array is present → "allowed"
- Else → "forbidden"

### Token Estimation

Rough approximation: 1 token ≈ 4 characters

- Input tokens: Sum all message content lengths ÷ 4
- Output tokens: `max_tokens` parameter (default 1000)
- Total estimate: Input + Output

---

## Stage 2: Candidate Generation

A candidate is a specific (provider, model) combination that could potentially handle the request.

### Per-Model Limit Loading

Each model has its own quota configuration:

```typescript
// Groq example - 9 models with different limits per model
{
  'llama-3.3-70b-versatile': { rpm: 30, rpd: 1000, tpm: 12000, tpd: 100000 },
  'llama-3.1-8b-instant': { rpm: 30, rpd: 14400, tpm: 6000, tpd: 500000 },
  'meta-llama/llama-4-scout-17b-16e-instruct': { rpm: 30, rpd: 1000, tpm: 30000, tpd: 500000 },
  'meta-llama/llama-4-maverick-17b-128e-instruct': { rpm: 30, rpd: 1000, tpm: 6000, tpd: 500000 },
  'moonshotai/kimi-k2-instruct': { rpm: 60, rpd: 1000, tpm: 10000, tpd: 300000 },
  'moonshotai/kimi-k2-instruct-0905': { rpm: 60, rpd: 1000, tpm: 10000, tpd: 300000 },
  'openai/gpt-oss-120b': { rpm: 30, rpd: 1000, tpm: 8000, tpd: 200000 },
  'openai/gpt-oss-20b': { rpm: 30, rpd: 1000, tpm: 8000, tpd: 200000 },
  'qwen/qwen3-32b': { rpm: 60, rpd: 1000, tpm: 6000, tpd: 500000 }
}

// Cerebras example - 6 models with hourly limits
{
  'gpt-oss-120b': { rpm: 30, rph: 900, rpd: 14400, tpm: 64000, tph: 1000000, tpd: 1000000 },
  'llama-3.3-70b': { rpm: 30, rph: 900, rpd: 14400, tpm: 64000, tph: 1000000, tpd: 1000000 },
  'llama3.1-8b': { rpm: 30, rph: 900, rpd: 14400, tpm: 60000, tph: 1000000, tpd: 1000000 },
  'qwen-3-32b': { rpm: 30, rph: 900, rpd: 14400, tpm: 64000, tph: 1000000, tpd: 1000000 },
  'qwen-3-235b-a22b-instruct-2507': { rpm: 30, rph: 900, rpd: 1440, tpm: 64000, tph: 1000000, tpd: 1000000 },
  'zai-glm-4.7': { rpm: 10, rph: 100, rpd: 100, tpm: 60000, tph: 1000000, tpd: 1000000 }
}

// Mistral example - 18 models with platform-wide + per-model limits
{
  // Platform limit (60 RPM shared across all 18 models)
  platformLimit: { rpm: 60 },
  // Per-model token limits
  'mistral-large-latest': { tpm: 500000, tpmu: 1000000000 },
  // ... 17 more models
}

// Vertex example - 2 models
{
  'gemini-3-pro-preview': { rpm: 60, tpm: 60000 },
  'gemini-3-flash-preview': { rpm: 60, tpm: 60000 }
}
```

---

## Stage 3: Hard Filtering

Hard filtering eliminates candidates that absolutely cannot handle the request. This is a binary decision: pass or fail.

### Filter Categories

#### 1. Provider Allow/Deny Lists

**Logic:**

- If router.providers.allow is set, provider must be in that list
- If router.providers.deny is set, provider must NOT be in that list

**Example:**

```json
{ "router": { "providers": { "allow": ["groq", "cerebras"] } } }
```

This removes mistral and vertex candidates.

#### 2. Strict Schema Requirement

**Logic:**

- If requirements.output equals "json_schema_strict":
  - Provider must be certified for strict schema
  - Otherwise, filter out

**Failure case:** If no certified providers exist, return HTTP 422 error.

#### 3. Streaming Requirement

**Logic:**

- If requirements.streaming equals "required", provider must have streaming=true
- If "forbidden", provider must NOT support streaming

#### 4. Tools Requirement

**Logic:**

- If requirements.tools equals "required", provider must have tools=true
- If "forbidden", provider must have tools=false

#### 5. Circuit Breaker State

**Logic:**

- If circuit breaker state is "OPEN", filter out the provider
- If "HALF_OPEN" or "CLOSED", the provider passes

#### 6. Per-Model Quota Check (Critical)

**Logic:**
For each candidate, check all applicable limits:

```typescript
// Check RPM (sliding window, 60 seconds)
if (limits.rpm && rpm >= limits.rpm) → filter out

// Check RPH (requests per hour counter)
if (limits.rph && rph >= limits.rph) → filter out

// Check RPD (requests per day counter)
if (limits.rpd && rpd >= limits.rpd) → filter out

// Check TPM (token sum in sliding window, 60 seconds)
if (limits.tpm && tpm + estimatedTokens >= limits.tpm) → filter out

// Check TPH (tokens per hour counter)
if (limits.tph && tph + estimatedTokens >= limits.tph) → filter out

// Check TPD (tokens per day counter)
if (limits.tpd && tpd + estimatedTokens >= limits.tpd) → filter out

// Check TPMU (tokens per month counter)
if (limits.tpmu && tpmu + estimatedTokens >= limits.tpmu) → filter out
```

**Redis Key Structure:**

```
quota:{provider}:{model}:rpm → Sorted set (timestamp)
quota:{provider}:{model}:rph:{YYYY-MM-DD-HH} → Counter
quota:{provider}:{model}:rpd:{YYYY-MM-DD} → Counter
quota:{provider}:{model}:tpm → Sorted set (tokenCount-timestamp)
quota:{provider}:{model}:tph:{YYYY-MM-DD-HH} → Counter
quota:{provider}:{model}:tpd:{YYYY-MM-DD} → Counter
quota:{provider}:{model}:tpmu:{YYYY-MM} → Counter
```

**Filter Reason:** `quota_exceeded_{limitType}` (e.g., quota_exceeded_rpm, quota_exceeded_tpm)

---

## Stage 4: Soft Scoring

Soft scoring ranks eligible candidates by assigning a numeric score.

### Scoring Formula

```
score = base_weight + preference_bonus + quota_headroom + health_score
```

**Component Weights:**

- base: 1.0 (all candidates start here)
- prefer: 0.5 (explicit provider preference)
- quota: 0.3 (headroom percentage)
- health: 0.5 (health score 0.0-1.0)

### Preference Bonus

```typescript
if (hints?.providers?.prefer) {
  const index = hints.providers.prefer.indexOf(candidate.provider.id);
  if (index !== -1) {
    bonus = 0.5 × (1 - index / list_length);
  }
}
```

### Quota Headroom

```typescript
let headroomScore = 1.0;
if (limits.rpm) headroomScore = Math.min(headroomScore, 1 - (rpm / limits.rpm));
if (limits.tpm) headroomScore = Math.min(headroomScore, 1 - (tpm / limits.tpm));
// ... etc for all limit types
score += 0.3 × headroomScore;
```

---

## Stage 5: Plan Compilation

The routing plan defines which candidates to try and in what order.

### Selection Logic

- Take top N candidates where N = router.fallback.max_attempts (default 3)
- timeoutMs = router.slo.max_latency_ms or default 30000ms

### Retry Policy

- retryOn429: retry on rate limit errors (default true)
- retryOnTimeout: retry on timeout errors (default true)
- retryOn5xx: retry on server errors (default true)

**Non-retryable:**

- 400 Bad Request
- 402 Payment Required
- Validation errors

---

## Stage 6: Execution with Failover

```
For each attempt in the plan:
  1. Check hard timeout exceeded → abort if yes
  2. Check circuit breaker → skip if open
  3. Check per-model quota → skip if exceeded
  4. Make HTTP request with per-attempt timeout
  5. If success:
     - Record actual token usage
     - Record success in health service
     - Return response
  6. If failure:
     - If 429: Sync local quota with provider
     - Determine if error is retryable
     - If retryable and more attempts remain → continue
     - Else → return error
```

### Token Usage Recording

```typescript
const tokensUsed = response.usage?.total_tokens || 0;
await quotaService.recordModelUsage(providerId, model, tokensUsed);
```

**Recording Logic:**

```typescript
// RPM: Add timestamp to sliding window
await redis.zadd(`quota:${provider}:${model}:rpm`, now, `${now}`);

// RPH: Increment hour counter
await redis.incr(`quota:${provider}:${model}:rph:${hour}`);

// RPD: Increment day counter
await redis.incr(`quota:${provider}:${model}:rpd:${day}`);

// TPM: Add to sliding window with token count
await redis.zadd(`quota:${provider}:${model}:tpm`, now, `${tokensUsed}-${now}`);

// TPH: Increment by token count
await redis.incrby(`quota:${provider}:${model}:tph:${hour}`, tokensUsed);

// TPD: Increment by token count
await redis.incrby(`quota:${provider}:${model}:tpd:${day}`, tokensUsed);

// TPMU: Increment by token count
await redis.incrby(`quota:${provider}:${model}:tpmu:${month}`, tokensUsed);
```

### 429 Response Handling

When a provider returns 429:

1. **Extract rate limit headers:**

```typescript
const headers = {
  retryAfter: response.headers.get("retry-after"),
  limitRequests: response.headers.get("x-ratelimit-limit-requests"),
  remainingRequests: response.headers.get("x-ratelimit-remaining-requests"),
  // ... etc
};
```

2. **Sync local quota state:**

```typescript
if (headers.remainingRequests !== undefined) {
  const used = headers.limitRequests - headers.remainingRequests;
  await redis.set(`quota:${provider}:${model}:rpd:${day}`, used);
}
```

3. **Retry with next candidate:**

```typescript
if (retryable && attemptsRemain) {
  continue; // Try next candidate
}
```

### Streaming Inactivity Timeout

**Timeout Value:** 60 seconds

**Logic:**

```typescript
const inactivityTimeoutMs = 60000;
let lastChunkTime = Date.now();

while (true) {
  if (Date.now() - lastChunkTime > inactivityTimeoutMs) {
    throw new TimeoutError("Streaming inactivity timeout", "inactivity");
  }

  const { done, value } = await reader.read();
  if (done) break;

  lastChunkTime = Date.now();
  // Process chunk...
}
```

---

## Per-Model Quota System

### Seven Time Windows

| Window   | Description         | Reset          | Redis Type                                    |
| -------- | ------------------- | -------------- | --------------------------------------------- |
| **RPM**  | Requests per minute | End of minute  | Sliding window (sorted set)                   |
| **RPH**  | Requests per hour   | End of hour    | Counter (hourly key)                          |
| **RPD**  | Requests per day    | Midnight UTC   | Counter (daily key)                           |
| **TPM**  | Tokens per minute   | End of minute  | Sliding window (sorted set with token counts) |
| **TPH**  | Tokens per hour     | End of hour    | Counter (hourly key)                          |
| **TPD**  | Tokens per day      | Midnight UTC   | Counter (daily key)                           |
| **TPMU** | Tokens per month    | Calendar month | Counter (monthly key)                         |

### Provider-Specific Limit Structures

**Groq (9 Models, Per-Model Limits):**

```typescript
{
  'llama-3.3-70b-versatile': { rpm: 30, rpd: 1000, tpm: 12000, tpd: 100000 },
  'llama-3.1-8b-instant': { rpm: 30, rpd: 14400, tpm: 6000, tpd: 500000 },
  'meta-llama/llama-4-scout-17b-16e-instruct': { rpm: 30, rpd: 1000, tpm: 30000, tpd: 500000 },
  'meta-llama/llama-4-maverick-17b-128e-instruct': { rpm: 30, rpd: 1000, tpm: 6000, tpd: 500000 },
  'moonshotai/kimi-k2-instruct': { rpm: 60, rpd: 1000, tpm: 10000, tpd: 300000 },
  'moonshotai/kimi-k2-instruct-0905': { rpm: 60, rpd: 1000, tpm: 10000, tpd: 300000 },
  'openai/gpt-oss-120b': { rpm: 30, rpd: 1000, tpm: 8000, tpd: 200000 },
  'openai/gpt-oss-20b': { rpm: 30, rpd: 1000, tpm: 8000, tpd: 200000 },
  'qwen/qwen3-32b': { rpm: 60, rpd: 1000, tpm: 6000, tpd: 500000 }
}
```

**Cerebras (6 Models, Per-Model + Hourly):**

```typescript
{
  'gpt-oss-120b': { rpm: 30, rph: 900, rpd: 14400, tpm: 64000, tph: 1000000, tpd: 1000000 },
  'llama-3.3-70b': { rpm: 30, rph: 900, rpd: 14400, tpm: 64000, tph: 1000000, tpd: 1000000 },
  'llama3.1-8b': { rpm: 30, rph: 900, rpd: 14400, tpm: 60000, tph: 1000000, tpd: 1000000 },
  'qwen-3-32b': { rpm: 30, rph: 900, rpd: 14400, tpm: 64000, tph: 1000000, tpd: 1000000 },
  'qwen-3-235b-a22b-instruct-2507': { rpm: 30, rph: 900, rpd: 1440, tpm: 64000, tph: 1000000, tpd: 1000000 },
  'zai-glm-4.7': { rpm: 10, rph: 100, rpd: 100, tpm: 60000, tph: 1000000, tpd: 1000000 }
}
```

**Mistral (18 Models, Platform-Wide + Per-Model):**

```typescript
// Platform RPM limit (60) shared across ALL 18 models
platformLimit: { rpm: 60 }

// Per-model token limits
{
  'mistral-large-latest': { tpm: 500000, tpmu: 1000000000 },
  'mistral-large-2402': { tpm: 500000, tpmu: 1000000000 },
  // ... 16 more models
}
```

**Vertex (2 Models):**

```typescript
{
  'gemini-3-pro-preview': { rpm: 60, tpm: 60000 },
  'gemini-3-flash-preview': { rpm: 60, tpm: 60000 }
}
```

---

## Error Classes

### ProviderError (Base Class)

```typescript
export class ProviderError extends Error {
  constructor(
    message: string,
    public statusCode: number,
    public isRetryable: boolean = false,
    public headers?: RateLimitHeaders
  )
}
```

### ValidationError

- **HTTP Status:** 400
- **Retryable:** No
- **When thrown:** Request validation fails

### RateLimitError

- **HTTP Status:** 429
- **Retryable:** Yes
- **When thrown:** Provider returns 429

### ModelQuotaExceededError

- **HTTP Status:** 429
- **Retryable:** Yes
- **When thrown:** Per-model quota exceeded during routing
- **Properties:** providerId, model, limitType

### CircuitBreakerError

- **HTTP Status:** 503
- **Retryable:** Yes
- **When thrown:** Circuit breaker is OPEN or HALF_OPEN

### TimeoutError

- **HTTP Status:** 504
- **Retryable:** Yes
- **Types:** 'request' | 'inactivity'

---

## Example Scenarios

### Scenario 1: Simple Text Request

**Request:**

```json
{
  "model": "llama-3.3-70b",
  "messages": [{ "role": "user", "content": "Hello" }]
}
```

**Process:**

1. All 35 models pass hard filters (no strict requirements)
2. Scoring considers health, quota, preferences
3. Check per-model quotas with estimated tokens (~250)
4. Top model selected
5. Single attempt usually sufficient

### Scenario 2: Model at Daily Limit with Automatic Fallback

**Request:**

```json
{
  "model": "llama-3.3-70b",
  "messages": [{ "role": "user", "content": "Hello" }]
}
```

**Current Quota State:**

- groq/llama-3.3-70b: 1000/1000 RPD used (100% - LIMIT REACHED)
- cerebras/llama-3.3-70b: 500/14400 RPD used (3.5%)

**Process:**

1. Generate candidates for "llama-3.3-70b" across all providers
2. Hard filter checks quotas:
   - Groq: quota_exceeded_rpd → FILTERED OUT
   - Cerebras: passes → ELIGIBLE
3. Scoring ranks Cerebras as primary
4. Plan compiled with Cerebras as attempt #1
5. Request succeeds via Cerebras

**Result:** Automatic failover from Groq to Cerebras when quota hit.

### Scenario 3: All Models of Same Type at Limit

**Request:**

```json
{
  "model": "llama-3.3-70b",
  "messages": [{ "role": "user", "content": "Hello" }]
}
```

**Current Quota State:**

- groq/llama-3.3-70b: 1000/1000 RPD (100%)
- cerebras/llama-3.3-70b: 14400/14400 RPD (100%)
- Both models at daily limit

**Process:**

1. Generate candidates
2. Both filtered out:
   - Groq: quota_exceeded_rpd
   - Cerebras: quota_exceeded_rpd
3. Eligible list is empty
4. Return HTTP 422 with filtered reasons

**Client Response:**

```json
{
  "error": {
    "type": "gateway_error",
    "code": "NO_ELIGIBLE_PROVIDER",
    "message": "No eligible provider found",
    "details": {
      "filtered_providers": [
        {
          "provider": "groq",
          "model": "llama-3.3-70b",
          "reason": "quota_exceeded_rpd"
        },
        {
          "provider": "cerebras",
          "model": "llama-3.3-70b",
          "reason": "quota_exceeded_rpd"
        }
      ]
    }
  }
}
```

### Scenario 4: Hourly Limit Recovery

**Request:**

```json
{
  "model": "llama-3.3-70b",
  "messages": [{ "role": "user", "content": "Hello" }]
}
```

**Timeline:**

- 13:59: cerebras/llama-3.3-70b at 895/900 RPH
- 14:00: Request comes in

**Process:**

1. Check RPH for current hour ("2024-01-15-14")
2. New hour started, counter reset to 0
3. Check: 0 + 1 < 900 → PASS
4. Request succeeds

**Result:** Model automatically becomes available when hour resets.

### Scenario 5: Token Limit Exceeded Mid-Request

**Request:**

```json
{
  "model": "llama-3.3-70b",
  "messages": [{ "role": "user", "content": "Very long document..." }],
  "max_tokens": 2000
}
```

**Token Estimate:** 15000 (13000 input + 2000 output)

**Current Quota:**

- groq/llama-3.3-70b: 11000/12000 TPM used

**Process:**

1. Check TPM: 11000 + 15000 = 26000 > 12000 → EXCEEDED
2. Filtered: groq/llama-3.3-70b with reason 'quota_exceeded_tpm'
3. Try next model: groq/llama-3.1-8b-instant
4. Check: 1000/6000 TPM, 1000 + 15000 = 16000 > 6000 → EXCEEDED
5. Try cerebras/llama-3.3-70b: 10000/64000 TPM, 10000 + 15000 = 25000 < 64000 → PASS
6. Request succeeds via Cerebras

**Result:** Automatically falls back to provider with higher token limits.

### Scenario 6: Mistral Platform Limit Affects All Models

**Current State:**

- Platform RPM (all 18 Mistral models): 59/60 (98.3%)

**Request:**

```json
{
  "model": "mistral-large-latest",
  "messages": [{ "role": "user", "content": "Hello" }]
}
```

**Process:**

1. Check platform RPM: 59 + 1 >= 60 → EXCEEDED
2. All 18 Mistral models filtered with 'quota_exceeded_rpm'
3. Try other providers (Groq, Cerebras, Vertex)

**Result:** All Mistral models unavailable until next minute, fallback to other providers.

---

## Edge Cases and Special Handling

### No Eligible Providers

**Trigger:** All candidates filtered out
**Response:** HTTP 422 with detailed explanation

**Example Response:**

```json
{
  "error": {
    "code": "NO_ELIGIBLE_PROVIDER",
    "details": {
      "filtered_providers": [
        {
          "provider": "groq",
          "model": "llama-3.3-70b",
          "reason": "quota_exceeded_rpd"
        },
        {
          "provider": "cerebras",
          "model": "llama-3.3-70b",
          "reason": "quota_exceeded_rph"
        },
        {
          "provider": "mistral",
          "model": "mistral-large-latest",
          "reason": "quota_exceeded_rpm"
        }
      ]
    }
  }
}
```

### All Providers Unhealthy

**Trigger:** All providers have OPEN circuit breakers
**Response:** HTTP 503 Service Unavailable
**Recovery:** Automatic after 30 seconds

### ModelQuotaExceededError Handling

When a model's quota is exceeded:

1. **During filtering:** Candidate is removed before scoring
2. **During execution:** Error is caught, logged, and next attempt tried
3. **Retry behavior:** Always retryable (try different provider/model)

**Example Flow:**

```
Attempt 1: groq/llama-3.3-70b
→ Quota exceeded (ModelQuotaExceededError)
→ Log: { event: 'model_quota_exceeded', provider: 'groq', model: 'llama-3.3-70b', limit_type: 'rpd' }
→ Mark as retryable
→ Try next...

Attempt 2: cerebras/llama-3.3-70b
→ Success!
→ Return response
```

### Streaming Failure

**Trigger:** Connection drops during SSE stream
**Behavior:** Client receives partial response with error event
**Retry:** Not automatically retried (client already received partial data)

### Streaming Inactivity Timeout

**Trigger:** No data for 60 seconds during streaming
**Behavior:** TimeoutError with timeoutType='inactivity'
**Retry:** Yes (will try next provider)

### Redis Unavailable

**Trigger:** Redis connection fails during quota check
**Behavior:** Quota check bypassed, request proceeds
**Logging:** Error logged but request allowed

### Token Estimation Inaccuracy

**Trigger:** Estimated tokens differ from actual
**Behavior:**

- Pre-request: Uses estimate (may reject valid requests)
- Post-request: Records actual (corrects for next time)

---

## Summary

The routing system is designed to be:

1. **Deterministic:** Same inputs produce same decisions (given same state)
2. **Transparent:** Clear audit trail of all decisions
3. **Resilient:** Automatic failover when quotas are hit
4. **Efficient:** Smart scoring minimizes unnecessary retries
5. **Safe:** Strict validation prevents silent failures
6. **Precise:** Per-model quota tracking respects individual limits
7. **Automatic:** Models become available again when quotas reset
8. **Free:** All providers operate on free tier limits

The 6-stage pipeline ensures that requests are matched with the most appropriate provider while respecting all free-tier quota limits. When a model reaches its limit, the gateway automatically tries alternative providers/models and only fails when no options remain.
