# LLM Gateway Documentation

## Table of Contents

1. [Introduction](#introduction)
2. [Architecture](#architecture)
3. [Quick Start](#quick-start)
4. [Configuration Reference](#configuration-reference)
5. [API Reference](#api-reference)
6. [Routing Logic](#routing-logic)
7. [Deployment](#deployment)
8. [Monitoring](#monitoring)
9. [Troubleshooting](#troubleshooting)

---

## Introduction

LLM Gateway is an API gateway for Large Language Models. It routes requests to different LLM providers (Groq, Cerebras, Mistral, and Google Vertex AI) and handles failover when providers have issues.

## Architecture

### System Components

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
              │    Mistral/Vertex)      │
              └─────────────────────────┘
```

### How Requests Flow

1. Client sends a request to the gateway
2. Gateway checks the API key
3. Gateway checks rate limits
4. Routing pipeline picks the best provider
5. Request is sent to the LLM provider
6. Response is returned to the client
7. Success/failure is recorded for monitoring

---

## Quick Start

### What You Need

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
    "model": "chat-pro",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

---

## Configuration

### Required Settings

| Variable | Description |
|----------|-------------|
| `GATEWAY_API_KEY` | Your API key (minimum 32 characters) |

You also need at least one provider key:

| Variable | Provider |
|----------|----------|
| `GROQ_API_KEY` | Groq |
| `CEREBRAS_API_KEY` | Cerebras |
| `MISTRAL_API_KEY` | Mistral |
| `GOOGLE_VERTEX_API_KEY` | Google Vertex |

### Optional Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | Server port |
| `ENV` | production | development or production |
| `LOG_LEVEL` | info | debug, info, warn, error |
| `REDIS_URL` | redis://localhost:6379 | Redis connection |
| `RATE_LIMIT_PER_IP` | 100 | Max requests per IP |
| `RATE_LIMIT_WINDOW_MS` | 60000 | Rate limit window (ms) |
| `CORS_ORIGINS` | * | Allowed CORS origins |

### Logical Models

Logical models are abstract names that map to specific provider models.

```go
var LogicalModels = map[string]LogicalModelConfig{
    "chat-pro": {
        Description: "High-quality conversational model",
        SLO: SLOConfig{
            TimeoutMs:    30000,
            MaxRetries:   3,
            HardTimeoutMs: intPtr(60000),
        },
        Routing: RoutingConfig{
            Strategy: "preference",
            Candidates: []string{
                "groq/llama-3.3-70b-versatile",
                "cerebras/llama-3.1-70b",
                "mistral/mistral-large",
            },
        },
    },
}
```

### Available Models

| Model | Use Case |
|-------|----------|
| `chat-lite` | Fast, simple responses |
| `chat-pro` | General purpose |
| `chat-max` | Complex, long tasks |
| `analysis-pro` | Reasoning and analysis |
| `json-fast` | Quick JSON output |
| `json-safe` | Strict JSON schema |
| `code-fast` | Quick code generation |
| `code-pro` | Production code |
| `tools-pro` | Function calling |

### Provider Configuration

See `internal/config/providers.go` for details.

---

## API Reference

### Health Check

```http
GET /health
```

Returns system health and provider status.

### Chat Completions

```http
POST /v1/chat/completions
```

Headers:
- `Authorization: Bearer {GATEWAY_API_KEY}`
- `Content-Type: application/json`

Request:

```json
{
  "model": "chat-pro",
  "messages": [{"role": "user", "content": "Hello!"}],
  "temperature": 0.7
}
```

Response:

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

Response Headers:
- `X-Gateway-Provider`: groq
- `X-Gateway-Model`: llama-3.3-70b-versatile

### Streaming

Set `stream: true` in your request. Response will be sent as Server-Sent Events:

```
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}

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

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2026-02-06T22:45:00Z",
  "providers": {
    "groq": { "circuit_state": "CLOSED", "health_score": 1.0 },
    "cerebras": { "circuit_state": "CLOSED", "health_score": 0.95 }
  },
  "quota_status": {
    "groq/llama-3.3-70b": { "rpm_remaining": 25, "tpm_remaining": 5000 }
  }
}
```

#### Chat Completions

```http
POST /v1/chat/completions
```

**Headers:**
- `Authorization: Bearer {GATEWAY_API_KEY}`
- `Content-Type: application/json`

**Request Body:**
```json
{
  "model": "chat-pro",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Explain quantum computing."}
  ],
  "temperature": 0.7,
  "max_tokens": 256,
  "stream": false
}
```

**Response:**
```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1700000000,
  "model": "llama-3.3-70b-versatile",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "Quantum computing uses quantum bits..."
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 15,
    "completion_tokens": 50,
    "total_tokens": 65
  }
}
```

**Response Headers:**
- `X-Gateway-Provider`: groq
- `X-Gateway-Model`: llama-3.3-70b-versatile
- `X-Gateway-Attempts`: 1

### Streaming

Set `stream: true` in your request. Response will be sent as Server-Sent Events:

```
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}

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

---

## Routing Logic

See [ROUTING_LOGIC.md](./ROUTING_LOGIC.md) for details.

### The 6 Routing Steps

1. **Derive Requirements** - Figure out what the request needs
2. **Generate Candidates** - Find available provider/model options
3. **Filter** - Remove options that can't handle the request
4. **Score & Sort** - Rank remaining options by preference and health
5. **Compile Plan** - Create a fallback plan
6. **Execute** - Call the provider, fall back on errors

---

## Deployment

See [DEPLOYMENT.md](./DEPLOYMENT.md) for full instructions.

### Build Docker Image

```bash
docker build -t llm-gateway:latest .
```

### Run Docker Container

```bash
docker run -d \
  -p 8080:8080 \
  -e GATEWAY_API_KEY=${GATEWAY_API_KEY} \
  -e GROQ_API_KEY=${GROQ_API_KEY} \
  -e REDIS_URL=redis://host:6379 \
  llm-gateway:latest
```

### Docker Compose

```yaml
version: "3.8"
services:
  gateway:
    build: .
    ports:
      - "8080:8080"
    environment:
      - GATEWAY_API_KEY=${GATEWAY_API_KEY}
      - REDIS_URL=redis://redis:6379
    depends_on:
      - redis
  
  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data
```

## Monitoring

### Health Endpoint

```bash
curl http://localhost:8080/health
```

Returns system health and provider status.

### Logging

Logs include request ID, provider, latency, and token usage:

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

---

## Troubleshooting

### Gateway Won't Start

**Symptom:** Container exits immediately

**Check:**
1. All required API keys are set
2. GATEWAY_API_KEY is at least 32 characters
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

- Check [CHANGELOG.md](./CHANGELOG.md) for known issues
- Check [ROUTING_LOGIC.md](./ROUTING_LOGIC.md) for routing details
- View logs with `docker-compose logs -f`
