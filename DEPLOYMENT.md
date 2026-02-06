# LLM Gateway Deployment Guide

## Quick Start

### Prerequisites
- Docker & Docker Compose
- Redis (for rate limiting and quota tracking)
- API keys for at least one LLM provider

### Environment Setup

1. Copy the environment template:
```bash
cp .env.example .env
```

2. Edit `.env` with your API keys:
```bash
# Required
GATEWAY_API_KEY=your-secure-gateway-key-min-32-chars

# At least one provider
GROQ_API_KEY=your-groq-key
CEREBRAS_API_KEY=your-cerebras-key
MISTRAL_API_KEY=your-mistral-key
GOOGLE_VERTEX_API_KEY=your-vertex-key
```

### Docker Compose (Recommended)

```bash
# Build and start
docker-compose up -d

# View logs
docker-compose logs -f app

# Stop
docker-compose down
```

### Manual Docker Build

```bash
# Build image
docker build -t llm-gateway .

# Run container
docker run -d \
  -p 8080:8080 \
  -e GATEWAY_API_KEY=your-key \
  -e GROQ_API_KEY=your-groq-key \
  -e REDIS_URL=redis://host:6379 \
  llm-gateway
```

## Configuration

### Required Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `GATEWAY_API_KEY` | API key for gateway authentication (min 32 chars) | Required |
| `GROQ_API_KEY` | Groq provider API key | Required |
| `CEREBRAS_API_KEY` | Cerebras provider API key | Required |
| `MISTRAL_API_KEY` | Mistral provider API key | Required |
| `GOOGLE_VERTEX_API_KEY` | Google Vertex AI API key | Required |

### Optional Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | 8080 |
| `ENV` | Environment (development/production) | production |
| `LOG_LEVEL` | Log level (debug/info/warn/error) | info |
| `REDIS_URL` | Redis connection URL | redis://localhost:6379 |
| `REDIS_KEY_PREFIX` | Redis key prefix | llm_gateway |
| `RATE_LIMIT_PER_IP` | Requests per IP per window | 100 |
| `RATE_LIMIT_WINDOW_MS` | Rate limit window in ms | 60000 |
| `CORS_ORIGINS` | Allowed CORS origins (comma-separated) | * (all) |

## Health Checks

The container includes health checks:
- **HTTP**: `GET /health` endpoint
- **Docker**: Built-in healthcheck every 30s
- **Docker Compose**: Depends on Redis health

## Security

- Runs as non-root user (UID 1000)
- Minimal Alpine Linux base image
- No shell access in production image
- HTTPS support for provider connections

## Monitoring

### Endpoints
- `GET /health` - Health status
- `POST /v1/chat/completions` - OpenAI-compatible API

### Logs
Structured JSON logs with request IDs for tracing.

## Troubleshooting

### Container won't start
- Check all required API keys are set
- Verify GATEWAY_API_KEY is at least 32 characters
- Ensure Redis is accessible

### High memory usage
- Redis memory limit: 256mb (configured in docker-compose.yml)
- Circuit breaker timeout: 60s for inactivity

### Rate limiting
- Default: 100 requests per IP per minute
- Configure with `RATE_LIMIT_PER_IP` and `RATE_LIMIT_WINDOW_MS`
