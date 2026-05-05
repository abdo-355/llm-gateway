# LLM Gateway

A production-ready API gateway for routing LLM requests across multiple providers with intelligent failover, rate limiting, and observability.

---

## What Is This?

LLM Gateway is a unified API interface that sits between your application and LLM providers. Instead of calling providers directly, you call the gateway and it:

- **Routes requests intelligently** based on model requirements, provider health, and your preferences
- **Handles failures automatically** - if one provider has issues, it falls back to another
- **Tracks usage and limits** per-model, per-provider with Redis-backed quota management
- **Provides a single API** that works like OpenAI's API, so you don't need to change your code when switching providers

### Why Use It?

| Problem | Solution |
|---------|----------|
| Provider goes down | Automatic failover to backup providers |
| Rate limits hit | Routes to providers with remaining quota |
| Different APIs | Unified OpenAI-compatible API |
| Cost management | Per-model quotas and token tracking |
| Debugging | Structured logging, health endpoints |

### Supported Providers

- **Groq** - Fast inference for Llama models
- **Cerebras** - High-throughput Llama and Qwen models
- **Mistral** - Mistral, Ministral, and Pixtral models
- **NVIDIA NIM** - High-performance LLMs via NVIDIA's API
- **Ollama** - Self-hosted and cloud Ollama models
- **Kilo** - Diverse models via Kilo's gateway
- **Cloudflare Workers AI** - Native Workers AI chat models with daily neuron-budget tracking
- **OpenCode Zen** - Free and experimental models via OpenCode's Zen gateway

---

## Key Features

### Intelligent Routing
The gateway examines each request to determine what it needs (streaming, JSON output, tools, etc.) and routes to providers that can handle those requirements. It considers:
- Model capabilities (which providers support the requested model)
- Provider health (success rates, latency)
- Your preferences (preferred providers, deny lists)
- Provider preference and health-based ranking within each tier

### Automatic Failover
If a provider fails (timeout, error, rate limit), the gateway automatically tries the next available provider. This happens transparently - your code sees a successful response or a final error.

### Circuit Breaker
When a provider experiences repeated failures, the circuit breaker opens and temporarily stops sending requests. After a cooldown period, it allows probe requests again. This prevents hammering a struggling provider.

### Quota Management
Per-model, per-provider limits are tracked in Redis:
- Requests per minute/hour/day
- Tokens per minute/hour/day/month

When a limit is reached, that model/provider is filtered out and other options are tried.

### Unified API
The gateway implements OpenAI's chat completions API. Your existing code calling OpenAI can switch to the gateway by changing the base URL.

### Tier-Based Models
Select relative capability tiers directly:
- `default` - Balanced quality, speed, and cost
- `pro` - Higher capability for more complex tasks
- `max` - Highest general capability models

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      HTTP Layer                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │   Handlers   │  │  Middleware  │  │   Rate Limiting  │   │
│  │  (Gin)       │  │  (Auth/CORS) │  │   (Redis)        │   │
│  └──────────────┘  └──────────────┘  └──────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Routing Service                           │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Stage 1: Derive Requirements                          │ │
│  │  Stage 2: Generate Candidates                          │ │
│  │  Stage 3: Filter (Capabilities/Quota/Circuit Breaker)  │ │
│  │  Stage 4: Score & Sort (Preference + Health)           │ │
│  │  Stage 5: Compile Execution Plan                       │ │
│  │  Stage 6: Execute with Fallback                        │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Core Services                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │    Quota     │  │    Health    │  │    Provider      │   │
│  │   (Redis)    │  │  (Circuit)   │  │   (HTTP)         │   │
│  └──────────────┘  └──────────────┘  └──────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
              ┌─────────────────────────┐
              │   LLM Providers         │
              │   (Groq/Cerebras/       │
              │    Mistral/NIM/         │
              │    Ollama/Kilo/         │
              │    Gemini)              │
              └─────────────────────────┘
```

### How Requests Flow

1. **Request arrives** - You call `/v1/chat/completions` with your API key
2. **Authentication** - Gateway validates your API key
3. **Rate limiting** - Checks per-IP limits to prevent abuse
4. **Routing** - 6-stage pipeline picks the best provider based on model, health, and your preferences
5. **Execution** - Calls the provider with automatic fallback on failure
6. **Response** - Returns the LLM response in OpenAI format
7. **Metrics** - Records success/failure for health tracking and quota updates

---

## Quick Start

### Prerequisites

- Go 1.25 or later
- Redis 7.x
- At least one LLM provider API key

### Run Locally

```bash
git clone https://github.com/abdo-355/llm-gateway.git
cd llm-gateway

go mod download

cp .env.example .env
# Add your API keys to .env

go run ./cmd/gateway
```

### Run with Docker

```bash
docker-compose up -d

curl http://localhost:8080/health
```

### Your First Request

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer ${GATEWAY_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "default",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

---

## Manual Model Verification

Use the standalone upstream verifier to manually test every configured provider/model combination against the provider endpoints directly.

It is manual-only:
- it does not run on startup
- it does not run in CI/CD
- it uses short prompts with a configurable token cap (default `1024` via `--probe-max-tokens`)
- it prints a final stdout report with exact failures
- it does not depend on the gateway server being up

### What It Tests

For every configured provider/model combination, the verifier exercises:
- basic text generation
- grouped request-field acceptance
- streaming with usage chunks
- `json_object`
- strict `json_schema`
- tools/function calling

The verifier calls each provider directly using the configured base URL and authentication method.

Execution behavior:
- providers run concurrently
- models run concurrently
- requests for models under the same provider are start-spaced so they do not all fire at once
- scheduling uses conservative pacing based on half of the configured RPM limits
- main-pass probe attempts use a default `5m` timeout
- timeout and rate-limit failures wait `10s` and retry, up to `3` attempts total
- once a model accumulates `3` timeout/rate-limit hits, the verifier defers the rest of that model’s unfinished probes
- after the full main pass completes, each deferred model gets one final recovery check using a `2m` timeout and no retries
- if that recovery check succeeds, only the probes that were not completed earlier are replayed with the same `2m` timeout and no retries

The final report records:
- per-probe outcomes
- per-model final outcomes
- detailed attempt logs with timing, phase, HTTP status, and failure reason

### Run It

Run:

```bash
go run ./cmd/verify-upstream
```

Optional filters:

```bash
go run ./cmd/verify-upstream --provider mistral
```

Optional behavior flags:

```bash
go run ./cmd/verify-upstream --timeout 5m --fail-fast
go run ./cmd/verify-upstream --probe-max-tokens 2048
```

### Debug Raw Upstream Responses

When a provider returns `200 OK` but the visible content is empty, you can log the raw successful response body and raw SSE `data:` payloads before the gateway parses them.

```bash
LOG_RAW_PROVIDER_RESPONSES=1 \
LOG_RAW_PROVIDER_RESPONSE_FILTERS=mistral/magistral-* \
go run ./cmd/verify-upstream --provider mistral
```

Notes:
- `LOG_RAW_PROVIDER_RESPONSES=1` enables the logging
- `LOG_RAW_PROVIDER_RESPONSE_FILTERS` is optional and accepts comma-separated provider names, exact model names, or `provider/model` prefixes ending in `*`
- logs only include raw provider response payloads, not auth headers

### Failure Reporting

Failures include the exact reason when available, for example:
- missing provider auth envs like `MISTRAL_API_KEY`
- provider HTTP status and error message
- invalid JSON output when structured output was requested
- missing tool calls when tools were required
- missing stream usage chunks when streaming usage was requested

---

## Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GATEWAY_API_KEY` | Yes | Your API key (min 32 characters) |
| `GROQ_API_KEY` | No | Groq API key |
| `CEREBRAS_API_KEY` | No | Cerebras API key |
| `MISTRAL_API_KEY` | No | Mistral API key |
| `NIM_API_KEY` | No | NVIDIA NIM API key |
| `OLLAMA_API_KEY` | No | Ollama API key |
| `KILO_API_KEY` | No | Kilo API key (optional for free models) |
| `CLOUDFLARE_ACCOUNT_ID` | No | Cloudflare account ID for Workers AI |
| `CLOUDFLARE_API_TOKEN` | No | Cloudflare API token for Workers AI |
| `OPENCODE_ZEN_API_KEY` | No | OpenCode Zen API key |

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | Server port |
| `ENV` | production | development or production |
| `LOG_LEVEL` | info | debug, info, warn, error |
| `REDIS_URL` | redis://localhost:6379 | Redis connection |
| `RATE_LIMIT_PER_IP` | 100 | Max requests per IP per minute |
| `RATE_LIMIT_WINDOW_MS` | 60000 | Rate limit window in milliseconds |
| `CORS_ORIGINS` | * | Allowed CORS origins |

### Tiers

The public `model` selector maps to relative capability tiers:

| Tier | Use Case |
|------|----------|
| `default` | General purpose responses |
| `pro` | More complex tasks |
| `max` | Highest-capability general workloads |

Tiers are configured in `internal/config/tiers.go`, and per-provider model metadata is configured in `internal/config/providers.go`.

### Provider Configuration

Providers are configured in `internal/config/providers.go` with:
- Base URLs
- Authentication (bearer, header)
- Available models
- Rate limits per model
- Capabilities (streaming, tools, structured outputs)

---

## API Reference

### Health Check

```http
GET /health
```

Returns system health and provider status:

```json
{
  "status": "healthy",
  "providers": {
    "groq": { "circuit_state": "CLOSED", "health_score": 1.0 }
  }
}
```

### Chat Completions

```http
POST /v1/chat/completions
```

**Headers:**
- `Authorization: Bearer {GATEWAY_API_KEY}`
- `Content-Type: application/json`

**Request:**

```json
{
  "model": "default",
  "messages": [{"role": "user", "content": "Hello!"}],
  "temperature": 0.7
}
```

**Response:**

```json
{
  "id": "chatcmpl-123",
  "model": "llama-3.3-70b-versatile",
  "choices": [{
    "message": {
      "content": "Hello! How can I help?"
    }
  }],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 8,
    "total_tokens": 18
  }
}
```

**Response Headers:**
- `X-Gateway-Provider` - Provider used (e.g., groq)
- `X-Gateway-Model` - Model used (e.g., llama-3.3-70b-versatile)
- `X-Gateway-Tier` - Selected tier when tier routing is used (e.g., default)

### Streaming

Set `stream: true` in your request:

```json
{
  "model": "default",
  "messages": [{"role": "user", "content": "Tell me a story"}],
  "stream": true
}
```

Response is sent as Server-Sent Events:

```
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Once"}}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":" upon"}}]}

data: [DONE]
```

### Errors

**Rate Limited (429):**

```json
{
  "error": {
    "type": "rate_limit_error",
    "message": "Rate limit exceeded. Try again in 60s."
  }
}
```

**Provider Unavailable (503):**

```json
{
  "error": {
    "type": "circuit_breaker_error",
    "message": "Provider temporarily unavailable"
  }
}
```

**All Providers Failed:**

```json
{
  "error": {
    "type": "gateway_error",
    "code": "ALL_ATTEMPTS_FAILED",
    "message": "All provider attempts failed"
  }
}
```

---

## Routing Logic

The gateway uses a 6-stage pipeline to select the optimal provider:

1. **Derive Requirements** - Figure out what the request needs (streaming, JSON, tools)
2. **Generate Candidates** - Find available provider/model combinations
3. **Filter** - Remove candidates that can't handle the request
4. **Score & Sort** - Rank by preference and health
5. **Compile Plan** - Create execution plan with fallback order
6. **Execute** - Call provider, retry on failure

See [ROUTING_LOGIC.md](./ROUTING_LOGIC.md) for detailed documentation.

---

## Monitoring

### Health Endpoint

```bash
curl http://localhost:8080/health
```

Returns circuit breaker state and health scores per provider.

### Logging

All logs are JSON with request context:

```json
{
  "timestamp": "2026-02-06T22:45:00Z",
  "level": "info",
  "request_id": "req-123",
  "provider": "groq",
  "latency_ms": 1250,
  "tokens": 65
}
```

Key log fields:
- `request_id` - Unique request identifier
- `provider` - Provider used
- `model` - Model used
- `latency_ms` - Request latency
- `attempts` - Number of providers tried

---

## Troubleshooting

### Gateway Won't Start

**Symptom:** Container exits immediately

**Check:**
1. All required API keys are set
2. `GATEWAY_API_KEY` is at least 32 characters
3. Redis is accessible

```bash
docker-compose config
```

### Slow Responses

**Symptom:** Requests take more than 5 seconds

**Check:**
1. Provider health at `GET /health`
2. Circuit breaker state (should be CLOSED)
3. Quota limits not exceeded

```bash
curl http://localhost:8080/health | jq '.providers'
```

### Rate Limiting

**Symptom:** 429 errors

**Fix:**
- Increase `RATE_LIMIT_PER_IP` in .env
- Check provider quotas

### Provider Unavailable

**Symptom:** 503 errors

**Fix:**
- Wait 30 seconds for automatic recovery
- Check provider status

### Enable Debug Logs

```bash
LOG_LEVEL=debug go run ./cmd/gateway
```

### Get Help

- Check [ROUTING_LOGIC.md](./ROUTING_LOGIC.md) for routing details
- View logs with `docker-compose logs -f`
