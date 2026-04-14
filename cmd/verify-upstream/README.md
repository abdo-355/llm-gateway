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
- `GEMINI_API_KEY` - Google Gemini API
- `GOOGLE_VERTEX_PROJECT_ID` - GCP project for Vertex tests

Optional:
- `ENV` - environment name (development/production)
- `LOG_LEVEL` - debug, info, warn, error (default: info)

Vertex requires valid GCP credentials (`gcloud auth application-default-login` or `GOOGLE_APPLICATION_CREDENTIALS`).

## Exit Codes

- `0` - verifier completed without run-level error
- `1` - verifier failed to run or `--fail-fast` stopped on a probe failure

## Output

Reports each probe with:
- status: pass, fail, or skip
- model tested
- latency in ms
- token usage when available
- failure or skip reason when available

## Notes

- Probes bypass the gateway entirely - tests raw provider endpoints
- Uses minimal prompts (single word or short sentence) to reduce noise
- Successful 200 with empty visible content = fail (treated as provider issue)
- Structured output (JSON/tools) tested separately from basic text generation
- `429` responses are recorded as `SKIP`, and remaining probes for that same provider/model are skipped for the rest of the run
