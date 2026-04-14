# verify-upstream

Unified LLM gateway provider/model verifier. Tests providers and models directly (bypassing the gateway) to verify they respond correctly to low-token probes.

## Purpose

- Verify provider/model availability and correctness outside the gateway's routing logic
- Detect provider credential issues, endpoint problems, or model deprecations early
- Generate a provider capability matrix for gateway routing decisions
- Debug why certain provider/model combinations return empty or malformed responses

## Usage

```bash
# Run all probes with default 1024 token budget
go run cmd/verify-upstream/main.go

# Run specific provider only
go run cmd/verify-upstream/main.go -provider groq

# Run specific model only
go run cmd/verify-upstream/main.go -model "groq/llama-3.3-70b"

# Override token budget for probes
go run cmd/verify-upstream/main.go -probe-max-tokens 2048

# Override request timeout
go run cmd/verify-upstream/main.go -timeout 60s

# Stop on first failure
go run cmd/verify-upstream/main.go -fail-fast

# Verbose output
go run cmd/verify-upstream/main.go -v
```

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-provider` | single provider ID | all configured |
| `-model` | single model ID (provider/model format) | all in matrix |
| `-probe-max-tokens` | max_tokens for probe requests | 1024 |
| `-timeout` | per-request timeout | 30s |
| `-fail-fast` | stop on first failure | false |
| `-v` | verbose output | false |

## Environment

Loads `.env` file automatically. Requires provider API keys:

- `GROQ_API_KEY` - Groq provider
- `CEREBRAS_API_KEY` - Cerebras provider
- `MISTRAL_API_KEY` - Mistral provider
- `GOOGLE_API_KEY` - Google AI Studio (not Vertex)
- `VERTEX_PROJECT` - GCP project for Vertex tests
- `VERTEX_LOCATION` - GCP location for Vertex (default: us-central1)

Optional:
- `ENV` - environment name (development/production)
- `LOG_LEVEL` - debug, info, warn, error (default: info)

Vertex requires valid GCP credentials (`gcloud auth application-default-login` or `GOOGLE_APPLICATION_CREDENTIALS`).

## Exit Codes

- `0` - all probes passed
- `1` - one or more probes failed
- `2` - configuration/credential error

## Output

Reports each probe with:
- status: pass, fail, skip, error
- model tested
- latency in ms
- error message (if failed)
- visible content preview (first 100 chars)

## Notes

- Probes bypass the gateway entirely - tests raw provider endpoints
- Uses minimal prompts (single word or short sentence) to reduce noise
- Successful 200 with empty visible content = fail (treated as provider issue)
- Structured output (JSON/tools) tested separately from basic text generation