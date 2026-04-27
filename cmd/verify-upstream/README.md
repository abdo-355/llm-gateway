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

# Override main-pass request timeout (recovery/replay still use 2m)
go run cmd/verify-upstream/main.go -timeout 60s

# Stop on first failure
go run cmd/verify-upstream/main.go -fail-fast

```

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-provider` | single provider ID | all configured |
| `-model` | single model ID (provider/model format) | all in matrix |
| `-probe-max-tokens` | max_tokens for probe requests | 1024 |
| `-timeout` | main-pass per-attempt timeout | 5m |
| `-retries` | max attempts for timeout/rate-limited probes in the main pass | 3 |
| `-fail-fast` | stop on first failure | false |

## Environment

Loads `.env` file automatically. Requires provider API keys:

- `GROQ_API_KEY` - Groq provider
- `CEREBRAS_API_KEY` - Cerebras provider
- `MISTRAL_API_KEY` - Mistral provider
- `GEMINI_API_KEY` - Google Gemini API
- `NIM_API_KEY` - NVIDIA NIM provider
- `OLLAMA_API_KEY` - Ollama provider
- `KILO_API_KEY` - Kilo provider (optional)

Optional:
- `ENV` - environment name (development/production)
- `LOG_LEVEL` - debug, info, warn, error (default: info)

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
- Structured output (JSON/tools) is tested separately from basic text generation
- Providers and models are scheduled concurrently
- Requests for the same provider are started with at least a 1-second gap while still allowing overlap in flight
- The scheduler uses half of the configured RPM values as a safety margin
- Main-pass probe attempts retry only on timeout or rate limit
- Main-pass probe attempts use a 10-second retry delay and at most 3 attempts total
- If a model accumulates 3 timeout/rate-limit hits during the main pass, it is marked deferred and its remaining probes are skipped
- After the main pass, each deferred model gets one 2-minute recovery check with no retries
- If that recovery check succeeds, only the probes that were not completed are replayed, also with a 2-minute timeout and no retries
- The final report includes per-probe results, per-model outcomes, and detailed per-attempt logs
