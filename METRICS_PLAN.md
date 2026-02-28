# Metrics Plan

## Stack: Prometheus + Gin Middleware

Metrics are exposed on a dedicated metrics server (`:9090/metrics`) using `promhttp.Handler()`. All metrics are registered via `promauto` in `internal/metrics/metrics.go`. Context-based labels (`logical_model`, `router_profile`) are propagated via `internal/metrics/context.go`.

---

## Implemented Metrics

### 1. Request Metrics (HTTP Layer)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_http_requests_total` | Counter | `method`, `path`, `status`, `logical_model`, `router_profile` | Traffic volume & error rates |
| `gateway_http_request_duration_seconds` | Histogram | `method`, `path`, `logical_model`, `router_profile` | Latency distribution (p50/p95/p99) |
| `gateway_http_requests_in_flight` | Gauge | — | Concurrency / saturation |

### 2. LLM Provider Metrics (Routing/Execution Layer)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_provider_requests_total` | Counter | `provider`, `model`, `status`, `logical_model`, `router_profile`, `error_type` | Per-provider reliability |
| `gateway_provider_latency_seconds` | Histogram | `provider`, `model`, `logical_model`, `router_profile` | Provider speed comparison |
| `gateway_provider_tokens_total` | Counter | `provider`, `model`, `direction`, `logical_model` | Token usage & cost tracking |
| `gateway_routing_attempts_total` | Histogram | `logical_model`, `router_profile` | How many fallbacks per request (1 = healthy) |
| `gateway_failover_events_total` | Counter | `from_provider`, `to_provider`, `logical_model` | Provider failover events |
| `gateway_retry_success_total` | Counter | `logical_model` | Successful requests after retry |

### 3. Circuit Breaker & Quota Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_circuit_breaker_state` | Gauge | `provider`, `model` | 0=closed, 1=half-open, 2=open |
| `gateway_circuit_breaker_transitions_total` | Counter | `provider`, `model`, `from_state`, `to_state` | State transition events |
| `gateway_quota_usage_ratio` | Gauge | `provider`, `model`, `quota_type` | Current quota usage (0.0–1.0) |
| `gateway_quota_rejections_total` | Counter | `provider`, `model`, `quota_type` | Requests rejected due to quota |
| `gateway_rate_limit_rejections_total` | Counter | `reason` | IP rate limit hits |
| `gateway_auth_rejections_total` | Counter | `reason` | Authentication failures |

### 4. Streaming Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_stream_duration_seconds` | Histogram | `provider`, `model`, `logical_model` | Time-to-last-chunk |
| `gateway_stream_ttfb_seconds` | Histogram | `provider`, `model`, `logical_model` | Time-to-first-byte (user-perceived latency) |
| `gateway_stream_output_tokens_total` | Counter | `provider`, `model`, `logical_model` | Output tokens from streaming responses |

---

## Instrumentation Points

1. **Gin middleware** (`internal/middleware/metrics.go`) — HTTP request/duration/in-flight
2. **Router `Execute`/`ExecuteStream`** (`internal/services/router.go`) — provider latency, attempts, tokens, TTFB, failover, retry success
3. **HealthService `RecordSuccess`/`RecordFailure`** (`internal/services/health.go`) — circuit breaker state & transitions
4. **QuotaService** (`internal/services/quota.go`) — quota usage & rejections
5. **Rate limit middleware** (`internal/middleware/rate_limit.go`) — rate limit rejections
6. **Auth middleware** (`internal/middleware/auth.go`) — auth rejections
7. **Metrics server** (`internal/server/server.go`) — dedicated `:9090/metrics` endpoint
