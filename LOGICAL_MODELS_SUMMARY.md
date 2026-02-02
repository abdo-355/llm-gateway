# Logical Model Registry - Implementation Summary

## Overview

The Logical Model Registry has been successfully implemented, providing a simple OpenAI-compatible API that maps logical model names to intelligent routing policies.

## What Was Implemented

### 1. Type Definitions (`src/types/index.ts`)
- `LogicalModelId`: Type for logical model identifiers
- `TaskType`: Union type for task categories ('chat', 'analysis', 'json_extraction', 'code', 'tool_orchestration')
- `LogicalModelCandidate`: Interface for candidate (provider, model) pairs with weights
- `LogicalModelSLO`: Service Level Objectives (latency, attempts)
- `LogicalModelConfig`: Complete configuration for a logical model
- `LogicalModelRegistry`: Registry type mapping IDs to configurations

### 2. Registry Configuration (`src/config/logicalModels.ts`)
Created 9 logical models with kebab-case IDs:

| Logical Model | Task Type | Primary Use Case |
|--------------|-----------|------------------|
| `chat-lite` | chat | Fast, cheap responses |
| `chat-pro` | chat | Balanced quality/speed |
| `chat-max` | chat | Maximum capability |
| `analysis-pro` | analysis | Deep reasoning |
| `json-fast` | json_extraction | Quick parsing |
| `json-safe` | json_extraction | Reliable structured output |
| `code-fast` | code | Quick code generation |
| `code-pro` | code | Production code |
| `tools-pro` | tool_orchestration | Function calling |

Each logical model defines:
- 2-4 candidate (provider, model) pairs with preference weights
- Default SLOs (max latency, max attempts)
- Capability requirements (strict JSON, tools)

### 3. Router Integration (`src/services/router.ts`)
- `generateCandidatesFromLogicalModel()`: New method to generate candidates from logical model config
- Updated `scoreCandidates()`: Preserves logical model weights in scoring
- Updated `compilePlan()`: Accepts logical model SLO as defaults
- All existing routing logic (quotas, health, failover) works unchanged

### 4. Completions Route Updates (`src/routes/completions.ts`)
- Detects if requested model is a logical model
- Uses logical model's candidate list instead of all models
- Applies logical model SLO as defaults (overridable by explicit hints)
- Adds response headers for observability:
  - `X-Gateway-Provider`: Actual provider used
  - `X-Gateway-Model`: Actual model used
  - `X-Gateway-Logical-Model`: Original logical model requested
  - `X-Gateway-Attempts`: Number of attempts made

## API Usage

### Simple Request (Recommended)
```bash
curl -X POST http://gateway/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "model": "chat-pro",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### With Response Headers
```bash
curl -i -X POST http://gateway/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "json-safe",
    "messages": [{"role": "user", "content": "Extract name"}],
    "response_format": {"type": "json_schema", ...}
  }'
```

**Response Headers:**
```
X-Gateway-Provider: mistral
X-Gateway-Model: mistral-large-latest
X-Gateway-Logical-Model: json-safe
X-Gateway-Attempts: 1
```

### With Explicit Overrides (Power User)
```bash
curl -X POST http://gateway/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "chat-pro",
    "messages": [...],
    "router": {
      "slo": {"max_latency_ms": 5000},
      "fallback": {"max_attempts": 5}
    }
  }'
```

## How It Works

1. **Request arrives** with logical model ID (e.g., "chat-pro")
2. **Lookup** logical model configuration in registry
3. **Generate candidates** only from logical model's candidate list
4. **Apply hard filters** (capabilities, quotas, circuit breaker)
5. **Score candidates** using weights from logical model + health + quota
6. **Compile plan** using SLO from logical model (overridable by hints)
7. **Execute with failover** across the candidate list
8. **Return response** with headers showing actual provider/model used

## Backward Compatibility

✅ **Fully backward compatible:**
- Existing requests with provider model names (e.g., "llama-3.3-70b") continue to work
- Router hints continue to work and can override logical model defaults
- All existing quota/health/failover logic unchanged
- All 245 existing tests pass

## Benefits

1. **Simple API**: Users only need to know ~9 logical model names
2. **Intelligent Routing**: System automatically picks best available provider based on quota/health
3. **Resilient**: Built-in failover across multiple providers per logical model
4. **Observable**: Response headers show which provider actually served the request
5. **Maintainable**: Change backend providers without affecting API
6. **Flexible**: Power users can still override with explicit hints

## Testing

All 245 tests pass, including:
- Existing routing tests
- Quota management tests  
- Error handling tests
- Provider adapter tests

The logical model system integrates seamlessly with existing infrastructure.
