# Metrics Plan

## Stack: Prometheus + Gin Middleware

Expose a `/metrics` endpoint using `promhttp.Handler()`. Use `promauto` for registration. The `prometheus/client_golang` dependency is already in `go.mod`.

---

## What to Collect

### 1. Request Metrics (HTTP Layer)

| Metric | Type | Labels | Why |
|--------|------|--------|-----|
| `gateway_http_requests_total` | Counter | `method`, `path`, `status` | Traffic volume & error rates |
| `gateway_http_request_duration_seconds` | Histogram | `method`, `path`, `status` | Latency distribution (p50/p95/p99) |
| `gateway_http_requests_in_flight` | Gauge | — | Concurrency / saturation |

### 2. LLM Provider Metrics (Routing/Execution Layer)

| Metric | Type | Labels | Why |
|--------|------|--------|-----|
| `gateway_provider_requests_total` | Counter | `provider`, `model`, `status` (success/error/timeout/rate_limited) | Per-provider reliability |
| `gateway_provider_latency_seconds` | Histogram | `provider`, `model` | Provider speed comparison |
| `gateway_provider_tokens_total` | Counter | `provider`, `model`, `direction` (prompt/completion) | Token usage & cost tracking |
| `gateway_routing_attempts_total` | Histogram | `logical_model` | How many fallbacks per request (1 = healthy) |

### 3. Circuit Breaker & Quota Metrics

| Metric | Type | Labels | Why |
|--------|------|--------|-----|
| `gateway_circuit_breaker_state` | Gauge | `provider` | 0=closed, 1=half-open, 2=open |
| `gateway_quota_remaining_ratio` | Gauge | `provider`, `model`, `window` | How close to quota exhaustion |
| `gateway_rate_limit_rejections_total` | Counter | — | IP rate limit hits |

### 4. Streaming Metrics

| Metric | Type | Labels | Why |
|--------|------|--------|-----|
| `gateway_stream_duration_seconds` | Histogram | `provider`, `model` | Time-to-last-chunk |
| `gateway_stream_ttfb_seconds` | Histogram | `provider`, `model` | Time-to-first-byte (user-perceived latency) |

---

## Where to Instrument

1. **Gin middleware** — HTTP request/duration/in-flight counters (new middleware file)
2. **Router `Execute`/`ExecuteStream`** — provider latency, attempts, tokens, TTFB
3. **HealthService `RecordSuccess`/`RecordFailure`** — circuit breaker state gauge
4. **QuotaService** — remaining quota gauge (periodic or on-check)
5. **Rate limit middleware** — rejection counter
6. **New `/metrics` route** in server setup

---

## Implementation Order

1. Create `internal/metrics/metrics.go` — define all metrics in one place
2. Add HTTP middleware for request-level metrics
3. Instrument `Execute`/`ExecuteStream` in the router
4. Add circuit breaker + quota gauges
5. Register `/metrics` route
6. Optionally add Grafana dashboard JSON
