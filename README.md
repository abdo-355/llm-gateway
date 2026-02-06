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

The LLM Gateway is a high-performance API gateway for Large Language Model services. Built with Go 1.25 and the Gin web framework, it provides intelligent request routing across multiple LLM providers with automatic failover, circuit breaker protection, and comprehensive observability.

### Key Capabilities

- **Multi-Provider Support**: Routes to Groq, Cerebras, Mistral, and Google Vertex AI
- **Intelligent Routing**: 6-stage routing pipeline with preference-based selection
- **High Availability**: Circuit breaker pattern with automatic provider fallback
- **Quota Management**: Per-model rate limiting and usage tracking
- **Observability**: Structured logging, request tracing, and health monitoring

### Performance Characteristics

| Metric | Value |
|--------|-------|
| Routing Latency | <1ms |
| Throughput | 10,000+ RPS |
| Memory Footprint | <100MB |
| Docker Image Size | 82.6MB |
| Cold Start | <100ms |

---

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

### Data Flow

1. **Request Ingestion**: Client sends OpenAI-compatible request
2. **Authentication**: API key validation via middleware
3. **Rate Limiting**: Per-IP request throttling
4. **Routing Decision**: 6-stage pipeline selects optimal provider
5. **Provider Call**: HTTP request to selected LLM provider
6. **Response Streaming**: Real-time streaming or batched response
7. **Metrics Recording**: Success/failure tracking for circuit breaker

---

## Quick Start

### Prerequisites

- Go 1.25 or later
- Redis 7.x
- At least one LLM provider API key

### Local Development

```bash
# Clone repository
git clone https://github.com/abdo-355/llm-gateway.git
cd llm-gateway

# Install dependencies
go mod download

# Configure environment
cp .env.example .env
# Edit .env with your API keys

# Run application
go run ./cmd/gateway
```

### Docker Deployment

```bash
# Using Docker Compose (recommended)
docker-compose up -d

# Verify deployment
curl http://localhost:8080/health
```

### First API Call

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

## Configuration Reference

### Environment Variables

#### Required

| Variable | Type | Description |
|----------|------|-------------|
| `GATEWAY_API_KEY` | string | Authentication key for gateway API (min 32 chars) |

At least one provider key:

| Variable | Provider | Description |
|----------|----------|-------------|
| `GROQ_API_KEY` | Groq | API key for Groq inference |
| `CEREBRAS_API_KEY` | Cerebras | API key for Cerebras |
| `MISTRAL_API_KEY` | Mistral | API key for Mistral AI |
| `GOOGLE_VERTEX_API_KEY` | Google Vertex | API key for Vertex AI |

#### Optional

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | HTTP server port |
| `ENV` | production | Environment (development/production) |
| `LOG_LEVEL` | info | Logging level (debug/info/warn/error) |
| `REDIS_URL` | redis://localhost:6379 | Redis connection URL |
| `REDIS_KEY_PREFIX` | llm_gateway | Key prefix for Redis |
| `RATE_LIMIT_PER_IP` | 100 | Max requests per IP per window |
| `RATE_LIMIT_WINDOW_MS` | 60000 | Rate limit window in milliseconds |
| `CORS_ORIGINS` | * | Comma-separated allowed origins |

### Logical Model Configuration

Logical models abstract provider-specific model names into semantic categories.

Configuration location: `internal/config/logical_models.go`

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

Available logical models:

| Model | Purpose | SLO |
|-------|---------|-----|
| `chat-lite` | Quick responses | 15s timeout |
| `chat-pro` | General purpose | 30s timeout |
| `chat-max` | Complex tasks | 60s timeout |
| `analysis-pro` | Reasoning/analysis | 60s timeout |
| `json-fast` | Quick JSON extraction | 15s timeout |
| `json-safe` | Strict schema output | 30s timeout |
| `code-fast` | Rapid prototyping | 15s timeout |
| `code-pro` | Production code | 30s timeout |
| `tools-pro` | Function calling | 30s timeout |

### Provider Configuration

Configuration location: `internal/config/providers.go`

```go
var Providers = []ProviderConfig{
    {
        ID:      "groq",
        BaseURL: "https://api.groq.com/openai/v1",
        Auth: ProviderAuth{
            Type:       "bearer",
            HeaderName: "Authorization",
        },
        Models: ModelsConfig{
            Allowlist: []string{
                "llama-3.3-70b-versatile",
                "mixtral-8x7b",
            },
            Limits: map[string]ModelLimits{
                "llama-3.3-70b-versatile": {
                    RPM:  intPtr(30),
                    TPM:  intPtr(6000),
                    RPH:  intPtr(1000),
                    TPH:  intPtr(200000),
                    RPD:  intPtr(14400),
                    TPD:  intPtr(200000),
                    TPMU: intPtr(6000),
                },
            },
        },
    },
}
```

---

## API Reference

### Endpoints

#### Health Check

```http
GET /health
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

#### Streaming Completions

```http
POST /v1/chat/completions
Content-Type: text/event-stream
```

**Request:**
```json
{
  "model": "chat-pro",
  "messages": [{"role": "user", "content": "Hello"}],
  "stream": true
}
```

**Response (SSE):**
```
data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"}}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":" there"}}]}

data: [DONE]
```

### Error Responses

#### Rate Limited (429)
```json
{
  "error": {
    "type": "rate_limit_error",
    "code": "RATE_LIMITED",
    "message": "Rate limit exceeded. Try again in 60s.",
    "details": {
      "retry_after": 60,
      "limit_type": "rpm"
    }
  }
}
```

#### Circuit Breaker Open (503)
```json
{
  "error": {
    "type": "circuit_breaker_error",
    "code": "CIRCUIT_BREAKER_OPEN",
    "message": "Provider temporarily unavailable",
    "details": {
      "provider_id": "groq",
      "state": "OPEN"
    }
  }
}
```

---

## Routing Logic

See [ROUTING_LOGIC.md](./ROUTING_LOGIC.md) for detailed documentation of the 6-stage routing pipeline.

### Quick Reference

1. **Derive Requirements**: Parse request features
2. **Generate Candidates**: Expand logical models
3. **Filter**: Apply capability, quota, and health filters
4. **Score & Sort**: Calculate preference + health scores
5. **Compile Plan**: Create retry policy
6. **Execute**: Call provider with automatic fallback

---

## Deployment

See [DEPLOYMENT.md](./DEPLOYMENT.md) for comprehensive deployment instructions.

### Docker Image

```bash
# Build
docker build -t llm-gateway:latest .

# Run
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

---

## Monitoring

### Health Endpoint

```bash
curl http://localhost:8080/health
```

Returns:
- Overall system health
- Per-provider circuit breaker state
- Per-model quota status

### Structured Logging

All logs are in JSON format with consistent fields:

```json
{
  "timestamp": "2026-02-06T22:45:00Z",
  "level": "info",
  "event": "request.complete",
  "request_id": "req-123",
  "provider": "groq",
  "model": "llama-3.3-70b-versatile",
  "latency_ms": 1250,
  "tokens": 65
}
```

### Metrics

Key metrics available in logs:
- `latency_ms`: Request latency
- `tokens`: Token usage
- `attempts`: Number of provider attempts
- `circuit_state`: Circuit breaker state changes
- `quota_usage`: Quota consumption

---

## Troubleshooting

### Common Issues

#### Gateway won't start

**Symptom:** Container exits immediately

**Check:**
1. All required API keys are set
2. GATEWAY_API_KEY is at least 32 characters
3. Redis is accessible

```bash
# Verify environment
docker-compose config
```

#### High latency

**Symptom:** Requests take >5 seconds

**Check:**
1. Provider health: `GET /health`
2. Circuit breaker state (should be CLOSED)
3. Quota limits not exceeded

```bash
# Check provider status
curl http://localhost:8080/health | jq '.providers'
```

#### Rate limiting errors

**Symptom:** 429 errors

**Solution:**
- Increase `RATE_LIMIT_PER_IP` in .env
- Check per-model quotas in provider config
- Review Redis quota tracking

#### Circuit breaker open

**Symptom:** 503 errors with CIRCUIT_BREAKER_OPEN

**Solution:**
- Wait 30 seconds for automatic recovery
- Check provider health endpoint
- Review provider error logs

### Debug Mode

Enable debug logging:

```bash
LOG_LEVEL=debug go run ./cmd/gateway
```

### Getting Help

- Review [CHANGELOG.md](./CHANGELOG.md) for known issues
- Check [ROUTING_LOGIC.md](./ROUTING_LOGIC.md) for routing details
- Examine logs with `docker-compose logs -f`

---

## Migration Notes

See [CHANGELOG.md](./CHANGELOG.md) for migration guide from TypeScript v1.0.

---

## License

MIT License - See LICENSE file
