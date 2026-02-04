# LLM Gateway: Go Migration Plan

## Global Setup & Instructions

### Prerequisites

- Go 1.25+ installed
- Git configured
- Docker and Docker Compose installed
- Air installed (`go install github.com/air-verse/air@latest`)
- Access to existing Redis instance (for testing)

### Branching Strategy

```
master (TypeScript)
  └── migration/go-main (main migration branch)
       ├── task-0/setup
       ├── task-1/foundation
       ├── task-2/core-services
       ├── task-3/http-layer
       ├── task-4/testing
       ├── task-5/deployment
       └── task-6/finalization
```

### Workflow

1. Create main migration branch from master: `migration/go-main`
2. For each task:
   - Create branch from `migration/go-main`: `task-{N}/{name}`
   - Implement all items in the task
   - Create **multiple commits** as needed within the branch
   - When complete, notify user for approval
   - **User approves** → merge to `migration/go-main`
   - **User requests changes** → make changes, recommit, request approval again
3. After all tasks complete and approved, merge `migration/go-main` to `master`

### Module Path

`github.com/abdo-355/llm-gateway`

### Reference Implementation

**Note**: The existing TypeScript implementation in the `typescript/` directory serves as the reference for all functionality. Use it to understand:

- Business logic and algorithms
- Redis key structures
- Error handling patterns
- Configuration values
- API request/response formats

### Test File Placement

Test files should be **co-located** with source files:

- `router.go` → `router_test.go` (in same directory)
- `quota.go` → `quota_test.go` (in same directory)

---

## Task Completion Tracker

| Task | Branch                 | Status         | Approved By |
| ---- | ---------------------- | -------------- | ----------- |
| 0    | `task-0/setup`         | 🟡 In Progress |             |
| 1    | `task-1/foundation`    | ⬜ Not Started |             |
| 2    | `task-2/core-services` | ⬜ Not Started |             |
| 3    | `task-3/http-layer`    | ⬜ Not Started |             |
| 4    | `task-4/testing`       | ⬜ Not Started |             |
| 5    | `task-5/deployment`    | ⬜ Not Started |             |
| 6    | `task-6/finalization`  | ⬜ Not Started |             |

**Legend**: ⬜ Not Started | 🟡 In Progress | ✅ Complete

---

## Task 0: Project Setup

**Branch**: `task-0/setup` (from `migration/go-main`)

### Objective

Initialize the Go project structure and move TypeScript code to subdirectory.

### Items

#### 0.1 Create Main Migration Branch

```bash
git checkout -b migration/go-main
```

#### 0.2 Move TypeScript Implementation

Move existing TypeScript project to `typescript/` subdirectory:

- Create `typescript/` directory
- Move ALL existing files into it:
  - `src/` → `typescript/src/`
  - `package.json`, `package-lock.json` → `typescript/`
  - `tsconfig.json`, `jest.config.js`, `jest.setup.js` → `typescript/`
  - `Dockerfile`, `.dockerignore` → `typescript/`
  - `.env.example` → `typescript/.env.example`
  - `README.md` → `typescript/README.md`
  - `ROUTING_LOGIC.md` → `typescript/ROUTING_LOGIC.md`
  - `.gitignore` → `typescript/.gitignore`
- Keep only in root:
  - `.git/` (already there)
  - `.qwen/`, `.opencode/` (skill directories)

#### 0.3 Create Root .gitignore (Go version)

Create new `.gitignore` at root for Go project:

```
# Binaries for programs and plugins
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary, built with `go test -c`
*.test

# Output of the go coverage tool
*.out

# Dependency directories
vendor/

# Go workspace file
go.work

# Air live reload
tmp/

# Environment variables
.env
.env.local

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# OS
.DS_Store

# Build artifacts
dist/
bin/
gateway
```

#### 0.4 Create Go Directory Structure

Create directories at root:

```
cmd/gateway/
internal/
├── config/
├── types/
├── services/
├── handlers/
├── middleware/
├── lib/
└── errors/
```

#### 0.5 Initialize Go Module

```bash
go mod init github.com/abdo-355/llm-gateway
```

#### 0.6 Add Air Configuration

Create `.air.toml` for live reload:

- Build cmd: `go build -o ./tmp/main ./cmd/gateway`
- Binary path: `./tmp/main`
- Watch extensions: go, tpl, tmpl, html
- Exclude directories: assets, tmp, vendor
- Delay: 1000ms

#### 0.7 Commit(s)

```
Commit 1: Task 0: Move TypeScript implementation to subdirectory
Commit 2: Task 0: Initialize Go module and directory structure
Commit 3: Task 0: Add Air configuration for live reload
```

**→ STOP: Await user approval before merging to migration/go-main**

---

## Task 1: Foundation

**Branch**: `task-1/foundation` (from `migration/go-main`)

### Objective

Define all types, configurations, environment setup, and core utilities. No business logic.

### Dependencies

- Task 0 complete

### Items

#### 1.1 Install Dependencies

```bash
go get github.com/gin-gonic/gin
go get github.com/redis/go-redis/v9
go get github.com/prometheus/client_golang/prometheus
go get github.com/google/uuid
go get github.com/stretchr/testify
```

#### 1.2 Define Core Types (`internal/types/types.go`)

Define Go structs:

- `OpenAIMessage` - Chat message with role, content, tool_calls
- `OpenAITool` - Function definition
- `ResponseFormat` - JSON schema format
- `ChatCompletionRequest` - Main request
- `ChatCompletionResponse` - Main response
- `SSEChunk` - Streaming chunk
- `RouterHints` - Routing preferences
- `ProviderConfig` - Provider definition
- `ProviderAuth` - Auth configuration
- `ProviderModels` - Model list and limits
- `ProviderCapabilities` - Feature flags
- `ModelLimits` - Per-model quotas
- `LogicalModelConfig` - Logical model definition
- `LogicalModelCandidate` - Candidate with weight
- `AppConfig` - Top-level config
- `DerivedRequirements` - Parsed requirements
- `RoutingCandidate` - Provider-model-score tuple
- `RoutingAttempt` - Execution attempt
- `RoutingPlan` - Collection of attempts
- `ExecutionResult` - Success result
- `GatewayError` - Error response
- `RateLimitHeaders` - Rate limit info from providers

**Requirements**:

- JSON tags match OpenAI API
- Comments document each field
- Use pointers for optional fields where nil matters

#### 1.3 Define Custom Errors (`internal/errors/errors.go`)

Error types:

- `ProviderError` - Base error with status code, retryable flag
- `RateLimitError` - 429 with retry-after and limit type
- `CircuitBreakerError` - Circuit state error
- `TimeoutError` - Timeout with type (request/inactivity)
- `ModelQuotaExceededError` - Quota exceeded with limit details
- `PaymentRequiredError` - 402 error
- `ValidationError` - Validation failures with field details

#### 1.4 Environment Configuration (`internal/config/env.go`)

- `EnvConfig` struct with all environment variables
- `LoadEnv()` function:
  - Required: GATEWAY_API_KEY (≥32 chars), all provider API keys
  - Optional with defaults: PORT (8080), REDIS_URL (localhost:6379), LOG_LEVEL (info), RATE_LIMIT_PER_IP (100), RATE_LIMIT_WINDOW_MS (60000)
- Validation functions

#### 1.5 Provider Configuration (`internal/config/providers.go`)

Hardcoded provider configs:

- Groq: 9 models with limits
- Cerebras: 6 models with limits
- Mistral: 18 models with platform + per-model limits
- Vertex: 2 models with header auth

Include per-model limits, certifications (strict JSON support).

#### 1.6 Logical Models Configuration (`internal/config/logical_models.go`)

9 logical models:

- chat-lite, chat-pro, chat-max
- analysis-pro
- json-fast, json-safe
- code-fast, code-pro
- tools-pro

Each with candidates, SLO, task type.

#### 1.7 Logger Setup (`internal/lib/logger.go`)

- Initialize `log/slog`
- JSON handler in production, text in dev
- Support LOG_LEVEL
- Logger with context (request ID)

#### 1.8 Redis Client (`internal/lib/redis.go`)

- Initialize go-redis client
- Apply REDIS_KEY_PREFIX
- Retry strategy: exponential backoff, max 2s, 3 retries
- Graceful close method

#### 1.9 Commit(s)

```
Commit 1: Task 1: Add core type definitions
Commit 2: Task 1: Add custom error types
Commit 3: Task 1: Add environment configuration
Commit 4: Task 1: Add provider and logical model configs
Commit 5: Task 1: Add logging and Redis utilities
```

**→ STOP: Await user approval before merging to migration/go-main**

---

## Task 2: Core Services

**Branch**: `task-2/core-services` (from `migration/go-main`)

### Objective

Implement all business logic services.

### Dependencies

- Task 1 complete

### Items

#### 2.1 Quota Service (`internal/services/quota.go` + `quota_test.go`)

Methods:

- `EstimateTokens(request) int`
- `CheckModelQuota(provider, model, limits, estimatedTokens) error`
- `RecordModelUsage(provider, model, tokensUsed)`
- `HandleProviderRateLimit(provider, model, response) RateLimitInfo`
- `GetModelQuotaStatus(provider, model, limits) QuotaStatus`

Redis keys (exact same as TypeScript):

- `quota:{provider}:{model}:rpm` (sliding window)
- `quota:{provider}:{model}:rph:{YYYY-MM-DD-HH}`
- `quota:{provider}:{model}:rpd:{YYYY-MM-DD}`
- `quota:{provider}:{model}:tpm` (sliding window with tokens)
- `quota:{provider}:{model}:tph:{YYYY-MM-DD-HH}`
- `quota:{provider}:{model}:tpd:{YYYY-MM-DD}`
- `quota:{provider}:{model}:tpmu:{YYYY-MM}`

#### 2.2 Health Service (`internal/services/health.go` + `health_test.go`)

Circuit breaker:

- `GetCircuitState(provider) CircuitState`
- `CanExecute(provider) bool`
- `RecordSuccess(provider, latencyMs)`
- `RecordFailure(provider)`
- `GetHealthMetrics(provider) HealthMetrics`
- `GetAllHealthMetrics() []HealthMetrics`

States: CLOSED, OPEN, HALF_OPEN
Threshold: 5 failures → OPEN, 30s recovery → HALF_OPEN

#### 2.3 Vertex AI Adapter (`internal/services/vertex_adapter.go` + `vertex_adapter_test.go`)

- `TransformRequest(request) VertexAIRequest`
- `TransformResponse(response, model, requestId) ChatCompletionResponse`
- `BuildEndpointUrl(baseUrl, model, streaming) string`
- `TransformStreamingChunk(response) *SSEChunk`

#### 2.4 Provider Service (`internal/services/provider.go` + `provider_test.go`)

- `CallProvider(baseUrl, apiKey, model, request, timeout, ctx, providerType, auth) (*ChatCompletionResponse, error)`
- `StreamProvider(baseUrl, apiKey, model, request, timeout, onChunk, ctx, providerType, auth) error`

Features:

- HTTP client with timeout
- Rate limit header extraction
- Error handling (402, 429, 5xx, timeouts)
- Vertex AI transformation
- SSE parsing for streaming
- 60s inactivity timeout

#### 2.5 Router Service (`internal/services/router.go` + `router_test.go`)

6-stage pipeline:

**Stage 1**: `DeriveRequirements(request, hints) DerivedRequirements`

- Detect strict JSON, streaming, tools

**Stage 2**:

- `GenerateCandidates() []RoutingCandidate`
- `GenerateCandidatesFromLogicalModel(config) []RoutingCandidate`

**Stage 3**: `FilterCandidates(candidates, requirements, request, hints) (eligible, filtered)`

- Check allow/deny lists
- Check capabilities
- Check circuit breaker
- Check per-model quota

**Stage 4**: `ScoreCandidates(candidates, hints) []RoutingCandidate`

- Base + preference + health

**Stage 5**: `CompilePlan(candidates, hints, slo) RoutingPlan`

- Top N candidates, timeouts, retry policy

**Stage 6**:

- `Execute(plan, request, requestId) (*ExecutionResult, error)`
- `ExecuteStream(plan, request, requestId, onChunk, onComplete, onError)`

Helpers:

- `ShouldRetry(error, plan, attemptIndex) bool`
- `CreateGatewayError(error, attempts, requestId) *GatewayError`

#### 2.6 Service Initialization (`internal/services/services.go`)

- Initialize services with dependencies
- Export global instances

#### 2.7 Commit(s)

```
Commit 1: Task 2: Add quota service with Redis integration
Commit 2: Task 2: Add health service with circuit breaker
Commit 3: Task 2: Add Vertex AI adapter
Commit 4: Task 2: Add provider HTTP client
Commit 5: Task 2: Add router service with 6-stage pipeline
Commit 6: Task 2: Add service initialization and tests
```

**→ STOP: Await user approval before merging to migration/go-main**

---

## Task 3: HTTP Layer

**Branch**: `task-3/http-layer` (from `migration/go-main`)

### Objective

Implement Gin router, middleware, and HTTP handlers.

### Dependencies

- Task 2 complete

### Items

#### 3.1 Middleware (`internal/middleware/*.go` + `*_test.go`)

**request_id.go**:

- Generate UUID per request
- Set in context
- Add to response header

**auth.go**:

- Extract Bearer token
- Validate against GATEWAY_API_KEY
- Skip for /health
- Return 401 if invalid

**rate_limit.go**:

- Per-IP sliding window in Redis
- Key: `ratelimit:{ip}`
- Return 429 with Retry-After if exceeded
- Skip for /health

**cors.go**:

- Handle OPTIONS preflight
- Allow origins from CORS_ORIGINS
- Allow Authorization, Content-Type headers

**helmet.go**:

- Security headers (X-Content-Type-Options, X-Frame-Options, etc.)
- HSTS in production

**logger.go**:

- Structured logging with slog
- Log method, path, status, latency, request ID

**error.go**:

- Catch and format errors
- Return JSON error responses

#### 3.2 Handlers (`internal/handlers/*.go` + `*_test.go`)

**health.go**:

- `GET /health`
- Query all provider metrics
- Return status, timestamp, providers array
- No auth required

**metrics.go**:

- `GET /metrics`
- Prometheus metrics endpoint
- Auth required

**completions.go**:

- `POST /v1/chat/completions`
- Validate request
- Check logical model
- Run routing pipeline
- Handle streaming/non-streaming
- Response headers: X-Gateway-Provider, X-Gateway-Model, X-Gateway-Logical-Model, X-Gateway-Attempts

#### 3.3 Router Setup (`cmd/gateway/main.go`)

- Initialize Gin
- Register middleware (recovery, requestID, logger, helmet, CORS, error handler)
- Register routes:
  - GET /health (public)
  - POST /v1/chat/completions (auth + rate limit)
  - GET /metrics (auth + rate limit)
- Graceful shutdown on SIGTERM/SIGINT
- Close Redis on shutdown

#### 3.4 Commit(s)

```
Commit 1: Task 3: Add middleware (request ID, auth, rate limit, CORS, helmet, logger, error handler)
Commit 2: Task 3: Add health and metrics handlers
Commit 3: Task 3: Add completions handler
Commit 4: Task 3: Add main application entry point
Commit 5: Task 3: Add middleware and handler tests
```

**→ STOP: Await user approval before merging to migration/go-main**

---

## Task 4: Testing

**Branch**: `task-4/testing` (from `migration/go-main`)

### Objective

Add unit tests for all services.

### Dependencies

- Task 3 complete

### Items

#### 4.1 Mock Infrastructure

Create test helpers:

- Mock Redis client
- Mock HTTP server for providers
- Test fixtures (requests, configs)

#### 4.2 Unit Tests

Add tests alongside source files:

- `quota_test.go` - Test token estimation, quota checking, usage recording
- `health_test.go` - Test circuit breaker state transitions
- `provider_test.go` - Test HTTP client, error handling, streaming
- `router_test.go` - Test requirement derivation, filtering, scoring
- `*_test.go` for all middleware and handlers

#### 4.3 Commit(s)

```
Commit 1: Task 4: Add test utilities and mocks
Commit 2: Task 4: Add service unit tests
Commit 3: Task 4: Add middleware and handler tests
```

**→ STOP: Await user approval before merging to migration/go-main**

---

## Task 5: Deployment

**Branch**: `task-5/deployment` (from `migration/go-main`)

### Objective

Production deployment setup.

### Dependencies

- Task 4 complete

### Items

#### 5.1 Dockerfile

Multi-stage build:

- Build: golang:1.25-alpine
- Runtime: alpine:latest
- Non-root user (UID 1001)
- Health check: wget http://localhost:8080/health
- Expose 8080

#### 5.2 Docker Compose

Update root `docker-compose.yml`:

- Build from root (Go implementation)
- Same environment variables
- Same Redis service
- Same networks

#### 5.3 GitHub Actions CI

Create `.github/workflows/ci.yml`:

- Test on Go 1.25
- Cache modules
- Run tests
- Run linting
- Build binaries for linux/amd64, linux/arm64
- Build Docker image

#### 5.4 Environment Files

- `.env.example` - Template with all variables
- Ensure it matches what the Go app expects

#### 5.5 Final Testing

Manual validation:

- [ ] Docker build succeeds
- [ ] docker-compose up works
- [ ] Health endpoint returns 200
- [ ] Chat completions work
- [ ] Streaming works
- [ ] Failover works
- [ ] Quota tracking works
- [ ] Rate limiting works

#### 5.6 Commit(s)

```
Commit 1: Task 5: Add Dockerfile with multi-stage build
Commit 2: Task 5: Update docker-compose.yml for Go implementation
Commit 3: Task 5: Add GitHub Actions CI workflow
Commit 4: Task 5: Add .env.example and final validation
```

**→ STOP: Await user approval before merging to migration/go-main**

---

## Task 6: Finalization

**Branch**: `task-6/finalization` (from `migration/go-main`)

### Objective

Final documentation and cleanup.

### Dependencies

- All previous tasks complete and approved

### Items

#### 6.1 Documentation Updates

- README.md: Go implementation details, build instructions, API usage
- ROUTING_LOGIC.md: Document Go implementation (not code, but logic)
- CHANGELOG.md: Document v2.0.0 migration

#### 6.2 Final Checklist

Verify:

- [ ] All tests pass
- [ ] Docker build works
- [ ] docker-compose works
- [ ] Air live reload works
- [ ] Documentation is complete
- [ ] typescript/ directory still exists (will be removed manually later)

#### 6.3 Commit(s)

```
Commit 1: Task 6: Add ROUTING_LOGIC.md for Go implementation
Commit 2: Task 6: Update README.md with Go documentation
Commit 3: Task 6: Add CHANGELOG for v2.0.0
Commit 4: Task 6: Final documentation and cleanup
```

**→ STOP: Await user approval before merging to migration/go-main**

---

## Final Merge

After Task 6 approved:

1. Create PR from `migration/go-main` to `master`
2. Final review
3. Merge to master
4. Tag release: `v2.0.0`

---

## File Structure (Final)

```
/home/abdo/projects/llm-gateway/
├── cmd/
│   └── gateway/
│       └── main.go              # Entry point
├── internal/
│   ├── config/
│   │   ├── providers.go         # Provider configs
│   │   ├── logical_models.go    # Logical models
│   │   └── env.go               # Environment
│   ├── types/
│   │   └── types.go             # All type definitions
│   ├── services/
│   │   ├── quota.go             # Quota service
│   │   ├── quota_test.go        # Quota tests
│   │   ├── health.go            # Health service
│   │   ├── health_test.go       # Health tests
│   │   ├── provider.go          # HTTP client
│   │   ├── provider_test.go     # Provider tests
│   │   ├── vertex_adapter.go    # Vertex adapter
│   │   ├── vertex_adapter_test.go
│   │   ├── router.go            # Router service
│   │   ├── router_test.go       # Router tests
│   │   └── services.go          # Initialization
│   ├── handlers/
│   │   ├── health.go            # Health handler
│   │   ├── health_test.go       # Health tests
│   │   ├── metrics.go           # Metrics handler
│   │   ├── metrics_test.go      # Metrics tests
│   │   ├── completions.go       # Completions handler
│   │   └── completions_test.go  # Completions tests
│   ├── middleware/
│   │   ├── request_id.go
│   │   ├── request_id_test.go
│   │   ├── auth.go
│   │   ├── auth_test.go
│   │   ├── rate_limit.go
│   │   ├── rate_limit_test.go
│   │   ├── cors.go
│   │   ├── cors_test.go
│   │   ├── helmet.go
│   │   ├── helmet_test.go
│   │   ├── logger.go
│   │   ├── logger_test.go
│   │   ├── error.go
│   │   └── error_test.go
│   ├── lib/
│   │   ├── logger.go            # Logger setup
│   │   └── redis.go             # Redis client
│   └── errors/
│       └── errors.go            # Custom errors
├── typescript/                  # Original implementation (kept for reference)
│   ├── src/
│   ├── package.json
│   └── ...
├── Dockerfile                   # Multi-stage build
├── docker-compose.yml           # App + Redis
├── .air.toml                    # Air config
├── go.mod                       # Go module
├── go.sum                       # Dependencies
├── .env.example                 # Environment template
├── .gitignore                   # Go gitignore
├── ROUTING_LOGIC.md             # Go implementation docs
├── CHANGELOG.md                 # v2.0.0 migration notes
└── README.md                    # Updated documentation
```

---

**Migration Plan Created: Ready to Start Task 0**
