# LLM Gateway / Router

A personal LLM Gateway that exposes an OpenAI Chat Completions compatible HTTP endpoint and routes each request to an upstream provider/model based on policy, health, and quota.

## Features

- **OpenAI-Compatible API**: Drop-in replacement for OpenAI's `/v1/chat/completions` endpoint
- **Smart Routing**: Routes requests based on requirements (streaming, tools, strict JSON schema)
- **Multiple Providers**: Supports Groq, OpenRouter, Ollama, and OpenAI
- **Quota Management**: Tracks daily/RPM/TPM limits with rolling windows
- **Health Monitoring**: Circuit breakers and health scoring
- **Automatic Failover**: Retries on 429, timeouts, and 5xx errors
- **Streaming Support**: Full Server-Sent Events (SSE) streaming
- **Structured Outputs**: Strict JSON Schema validation and certification
- **Observability**: Prometheus metrics and structured logging

## Quick Start

### 1. Install Dependencies

```bash
npm install
```

### 2. Configure Environment

```bash
cp .env.example .env
# Edit .env with your API keys
```

### 3. Validate Configuration

```bash
npm run cli -- validate-config
```

### 4. Build and Run

```bash
npm run build
npm start
```

Or for development:

```bash
npm run dev
```

## API Usage

### Basic Request

```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $INTERNAL_API_KEY" \
  -d '{
    "model": "router:auto",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### With Router Hints

```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $INTERNAL_API_KEY" \
  -d '{
    "model": "router:auto",
    "messages": [{"role": "user", "content": "Hello!"}],
    "router": {
      "profile": "cheap_fast",
      "budget": {"mode": "free_only"},
      "providers": {
        "allow": ["groq", "ollama"],
        "prefer": ["groq"]
      },
      "slo": {
        "max_latency_ms": 2500,
        "hard_timeout_ms": 8000
      },
      "fallback": {
        "max_attempts": 3
      }
    }
  }'
```

### Strict JSON Schema

```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $INTERNAL_API_KEY" \
  -d '{
    "model": "router:auto",
    "messages": [{"role": "user", "content": "Extract name and age"}],
    "response_format": {
      "type": "json_schema",
      "json_schema": {
        "name": "person",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": {
            "name": {"type": "string"},
            "age": {"type": "integer"}
          },
          "required": ["name", "age"],
          "additionalProperties": false
        }
      }
    }
  }'
```

### Streaming

```bash
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $INTERNAL_API_KEY" \
  -d '{
    "model": "router:auto",
    "messages": [{"role": "user", "content": "Count to 5"}],
    "stream": true
  }'
```

## Configuration

### Providers (`config/providers.yaml`)

Configure your upstream LLM providers:

```yaml
providers:
  - id: "groq"
    kind: "openai_compatible"
    base_url: "https://api.groq.com/openai/v1"
    auth:
      type: "bearer_env"
      env: "GROQ_API_KEY"
    models:
      mode: "allowlist"
      allow:
        - "llama-3.3-70b-versatile"
    capabilities:
      chat_completions: true
      streaming: true
      tools: true
      structured_outputs:
        json_schema_strict: "model_dependent"
    limits:
      rpm: 30
      tpm: 6000
```

### Certifications (`config/certifications.yaml`)

Certify which (provider, model) pairs support strict JSON schema:

```yaml
certifications:
  - provider: "groq"
    model: "llama-3.3-70b-versatile"
    json_schema_strict: true
    tested_at: "2026-01-29"
```

## Router Hints Reference

| Field | Type | Description |
|-------|------|-------------|
| `profile` | string | Routing profile: `cheap_fast`, `reliable_structured`, `balanced` |
| `budget.mode` | string | Budget constraint: `free_only`, `allow_paid` |
| `requirements.output` | string | Output requirement: `text`, `json_schema_strict` |
| `requirements.streaming` | string | Streaming requirement: `required`, `preferred`, `forbidden` |
| `requirements.tools` | string | Tools requirement: `required`, `allowed`, `forbidden` |
| `slo.max_latency_ms` | number | Per-attempt timeout |
| `slo.hard_timeout_ms` | number | Total timeout across all attempts |
| `providers.allow` | string[] | Whitelist of provider IDs |
| `providers.deny` | string[] | Blacklist of provider IDs |
| `providers.prefer` | string[] | Ordered preference for provider selection |
| `fallback.max_attempts` | number | Maximum retry attempts (1-5) |
| `fallback.on_429` | boolean | Retry on rate limit |
| `fallback.on_timeout` | boolean | Retry on timeout |
| `fallback.on_5xx` | boolean | Retry on server error |
| `trace.request_id` | string | Custom request ID |
| `trace.tags` | string[] | Tags for logging/tracing |

## Endpoints

- `POST /v1/chat/completions` - Main chat completions endpoint
- `GET /health` - Health check with provider status
- `GET /health/providers` - List configured providers
- `GET /metrics` - Prometheus metrics

## Metrics

The gateway exposes Prometheus metrics at `/metrics`:

- `gateway_requests_total` - Total requests by status
- `gateway_latency_ms` - Request latency histogram
- `provider_circuit_state` - Circuit breaker state per provider
- `quota_remaining` - Remaining quota percentage

## Testing

```bash
# Run unit tests
npm test

# Run with coverage
npm run test:coverage

# Type checking
npm run typecheck

# Linting
npm run lint
```

## Architecture

```
┌─────────────────┐
│  Express Server │
│   /v1/chat/...  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Request Router  │
│ - Validate      │
│ - Derive reqs   │
│ - Filter        │
│ - Score         │
│ - Plan          │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Execution Engine│
│ - Retry logic   │
│ - Failover      │
│ - Circuit break │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Provider Clients│
│ - OpenAI compat │
│ - Undici pool   │
└─────────────────┘
```

## License

MIT
