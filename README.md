# LLM Gateway

A secure, production-ready LLM Gateway that exposes an OpenAI-compatible API and routes requests to multiple upstream providers (Groq, OpenRouter, Cerebras, Mistral, Vertex) with automatic failover, quota management, and health monitoring.

## Features

- **OpenAI-Compatible API** - Drop-in replacement for OpenAI's `/v1/chat/completions`
- **Multi-Provider Support** - groq, openrouter, cerebras, mistral, vertex
- **Smart Routing** - Automatic provider selection based on requirements
- **Automatic Failover** - Retries on errors, circuit breakers for unhealthy providers
- **Quota Management** - Redis-backed RPM/TPM/daily limits with persistence
- **Rate Limiting** - Per-IP rate limiting (configurable)
- **Mandatory Authentication** - Bearer token required for all requests
- **Streaming Support** - Full Server-Sent Events (SSE) support
- **Structured Outputs** - Strict JSON Schema validation
- **Docker Deployment** - Production-ready Docker Compose setup
- **Observability** - Prometheus metrics, structured logging

## Quick Start (Docker)

### 1. Clone and Configure

```bash
git clone <your-repo>
cd llm-gateway

# Copy and edit environment variables
cp .env.example .env
# Edit .env with your API keys
```

### 2. Configure Providers

Edit `config/config.json` to add/remove providers:

```json
{
  "providers": [
    {
      "id": "groq",
      "baseUrl": "https://api.groq.com/openai/v1",
      "auth": { "type": "bearer", "env": "GROQ_API_KEY" },
      "models": {
        "mode": "allowlist",
        "list": ["llama-3.3-70b-versatile"]
      },
      "capabilities": {
        "streaming": true,
        "tools": true,
        "structuredOutputs": "model_dependent"
      },
      "limits": { "rpm": 30, "dailyRequests": 1000 }
    },
    {
      "id": "cerebras",
      "baseUrl": "https://api.cerebras.ai/v1",
      "auth": { "type": "bearer", "env": "CEREBRAS_API_KEY" },
      "models": {
        "mode": "allowlist",
        "list": ["llama-4-scout-17b-16e-instruct", "llama-3.3-70b", "llama3.1-8b"]
      },
      "capabilities": {
        "streaming": true,
        "tools": true,
        "structuredOutputs": "model_dependent"
      },
      "limits": { "rpm": 60, "dailyRequests": 10000 }
    },
    {
      "id": "mistral",
      "baseUrl": "https://api.mistral.ai/v1",
      "auth": { "type": "bearer", "env": "MISTRAL_API_KEY" },
      "models": {
        "mode": "allowlist",
        "list": ["mistral-large-latest", "mistral-medium-latest", "codestral-latest"]
      },
      "capabilities": {
        "streaming": true,
        "tools": true,
        "structuredOutputs": "model_dependent"
      },
      "limits": { "rpm": 50, "dailyRequests": 5000 }
    },
    {
      "id": "vertex",
      "baseUrl": "https://us-central1-aiplatform.googleapis.com/v1",
      "auth": { "type": "bearer", "env": "VERTEX_API_KEY" },
      "models": {
        "mode": "allowlist",
        "list": ["gemini-3-pro-preview", "gemini-3-flash-preview", "gemini-1.5-pro", "gemini-1.5-flash"]
      },
      "capabilities": {
        "streaming": true,
        "tools": true,
        "structuredOutputs": "model_dependent"
      },
      "limits": { "rpm": 100, "dailyRequests": 20000 }
    }
  ],
  "certifications": [
    { "provider": "groq", "model": "llama-3.3-70b-versatile", "strictSchema": true },
    { "provider": "cerebras", "model": "llama-3.3-70b", "strictSchema": true },
    { "provider": "mistral", "model": "mistral-large-latest", "strictSchema": true }
  ]
}
```

### 3. Start Services

```bash
# Build and start
docker-compose up -d

# View logs
docker-compose logs -f app

# Check health
curl http://localhost:8080/health
```

### 4. Test

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "router:auto",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GATEWAY_API_KEY` | Yes | - | API key for authenticating requests (min 32 chars) |
| `GROQ_API_KEY` | No | - | Groq API key |
| `OPENROUTER_API_KEY` | No | - | OpenRouter API key |
| `CEREBRAS_API_KEY` | No | - | Cerebras API key |
| `MISTRAL_API_KEY` | No | - | Mistral API key |
| `VERTEX_API_KEY` | No | - | Google Vertex AI API key |
| `PORT` | No | 8080 | Server port |
| `RATE_LIMIT_PER_IP` | No | 100 | Max requests per IP per window |
| `RATE_LIMIT_WINDOW_MS` | No | 60000 | Rate limit window in ms |
| `CORS_ORIGINS` | No | - | Comma-separated allowed origins |
| `REDIS_URL` | No | redis://redis:6379 | Redis connection URL |
| `LOG_LEVEL` | No | info | Log level (debug, info, warn, error) |

## API Usage

### Authentication

All requests require Bearer token authentication:

```bash
curl -H "Authorization: Bearer $GATEWAY_API_KEY" ...
```

### Basic Request

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "router:auto",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### With Router Hints

Control routing behavior:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "router:auto",
    "messages": [{"role": "user", "content": "Hello!"}],
    "router": {
      "providers": {
        "allow": ["groq"],
        "prefer": ["groq"]
      },
      "slo": {
        "max_latency_ms": 5000
      },
      "fallback": {
        "max_attempts": 3
      }
    }
  }'
```

### Strict JSON Schema

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "router:auto",
    "messages": [{"role": "user", "content": "Extract name"}],
    "response_format": {
      "type": "json_schema",
      "json_schema": {
        "name": "person",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": { "name": {"type": "string"} },
          "required": ["name"]
        }
      }
    }
  }'
```

### Streaming

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "router:auto",
    "messages": [{"role": "user", "content": "Count to 5"}],
    "stream": true
  }'
```

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/chat/completions` | POST | Main chat completions endpoint |
| `/health` | GET | Health check with provider status |
| `/metrics` | GET | Prometheus metrics |

## Router Hints Reference

| Field | Type | Description |
|-------|------|-------------|
| `profile` | string | Routing profile: `cheap_fast`, `reliable_structured`, `balanced` |
| `requirements.output` | string | `text`, `json_schema_strict` |
| `requirements.streaming` | string | `required`, `preferred`, `forbidden` |
| `requirements.tools` | string | `required`, `allowed`, `forbidden` |
| `slo.max_latency_ms` | number | Per-attempt timeout |
| `slo.hard_timeout_ms` | number | Total timeout across attempts |
| `providers.allow` | string[] | Whitelist provider IDs |
| `providers.deny` | string[] | Blacklist provider IDs |
| `providers.prefer` | string[] | Preferred provider order |
| `fallback.max_attempts` | number | Max retry attempts (1-5) |
| `fallback.on_429` | boolean | Retry on rate limit |
| `fallback.on_timeout` | boolean | Retry on timeout |
| `fallback.on_5xx` | boolean | Retry on server error |

## Architecture

```
Request
  ↓
[Auth Middleware] - Bearer token validation
  ↓
[Rate Limit] - Per-IP Redis sliding window
  ↓
[Router] - Derive requirements, filter, score
  ↓
[Provider Selection] - Health checks, quota checks
  ↓
[Execution] - Retry logic, circuit breakers
  ↓
[Provider] - OpenAI-compatible HTTP client
  ↓
Response
```

## Development

```bash
# Install dependencies
npm install

# Run in development mode (requires local Redis)
npm run dev

# Type check
npm run typecheck

# Build
npm run build
```

## Security

- **Mandatory Bearer Authentication** - All requests require valid API key
- **Non-root Container** - Runs as UID 1001
- **Rate Limiting** - Per-IP limits with Redis
- **Helmet Headers** - Security headers (CSP, HSTS, etc.)
- **CORS** - Configurable origin whitelist
- **Internal Network** - Redis isolated in backend network
- **No Secrets in Code** - All credentials via environment variables

## License

MIT
