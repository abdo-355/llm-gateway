# Prometheus Dashboard Queries

This document contains PromQL queries for building a comprehensive LLM Gateway dashboard in Prometheus/Grafana.

## Available Labels

### Tiers (5)
- `lite` - Lowest-latency and lowest-cost tier
- `default` - Balanced general-purpose tier
- `strong` - Higher-capability tier
- `frontier` - Highest-capability general tier
- `deep-think` - Deliberative/high-reasoning tier

### Router Strategies
- `default` - Default routing
- `cheap_fast` - Optimize for cost/speed
- `reliable_structured` - Optimize for reliability/structured outputs
- `balanced` - Balanced routing

### Providers
- `groq`, `mistral`, `cerebras`, `gemini`, `vertex`

### Directions
- `prompt` - Input tokens
- `completion` - Output tokens
- `total` - Total tokens

### Static Model Metadata
- `gateway_model_info{provider,model,tier,cost_band,latency_band,context_band,reasoning_band,strict_json}`
- value is always `1` for configured models

---

## 1. Request Volume & Traffic

### Configured Models by Tier
```promql
count by (tier) (gateway_model_info)
```
**Description**: Number of configured provider models in each tier.

### Configured Models by Metadata Bands
```promql
count by (tier, cost_band, latency_band, reasoning_band) (gateway_model_info)
```
**Description**: Static model inventory breakdown by tier and capability bands.

### HTTP Requests by Tier (rate)
```promql
sum by (tier) (rate(gateway_http_requests_total[5m]))
```
**Description**: Requests per second grouped by tier.

### HTTP Requests by Path & Method
```promql
sum by (method, path) (rate(gateway_http_requests_total[5m]))
```
**Description**: Request distribution across API endpoints.

### HTTP Requests by Status Code
```promql
sum by (status) (rate(gateway_http_requests_total[5m]))
```
**Description**: Distribution of HTTP response status codes.

### Active Requests (In Flight)
```promql
gateway_http_requests_in_flight
```
**Description**: Current number of requests being processed.

---

## 2. Success & Error Rates

### Provider Success Rate by Tier
```promql
sum by (tier) (rate(gateway_provider_requests_total{status="success"}[5m])) 
/ 
sum by (tier) (rate(gateway_provider_requests_total[5m]))
```
**Description**: Percentage of successful provider requests per tier.

### Provider Success Rate by Provider
```promql
sum by (provider) (rate(gateway_provider_requests_total{status="success"}[5m])) 
/ 
sum by (provider) (rate(gateway_provider_requests_total[5m]))
```
**Description**: Percentage of successful requests per provider.

### Error Breakdown by Error Type
```promql
sum by (error_type) (rate(gateway_provider_requests_total{status!="success"}[5m]))
```
**Description**: Request errors grouped by error type (rate_limit, timeout, etc.).

### Error Rate by Tier
```promql
sum by (tier) (rate(gateway_provider_requests_total{status!="success"}[5m]))
```
**Description**: Error rate per tier.

### Auth Rejections
```promql
sum by (reason) (rate(gateway_auth_rejections_total[5m]))
```
**Description**: Authentication failures by reason.

### Rate Limit Rejections
```promql
sum by (reason) (rate(gateway_rate_limit_rejections_total[5m]))
```
**Description**: Rate limit rejections by reason.

---

## 3. Token Usage & Cost

### Total Tokens by Tier
```promql
sum by (tier) (rate(gateway_provider_tokens_total[5m]))
```
**Description**: Token consumption rate per tier.

### Tokens by Direction (prompt/completion/total)
```promql
sum by (direction) (rate(gateway_provider_tokens_total[5m]))
```
**Description**: Token usage breakdown by direction (input vs output).

### Tokens by Provider
```promql
sum by (provider, direction) (rate(gateway_provider_tokens_total[5m]))
```
**Description**: Token usage per provider, split by direction.

### Tokens by Tier & Direction
```promql
sum by (tier, direction) (rate(gateway_provider_tokens_total[5m]))
```
**Description**: Detailed token usage: tier × direction.

### Average Tokens per Request by Tier
```promql
sum by (tier) (rate(gateway_provider_tokens_total[5m])) 
/ 
sum by (tier) (rate(gateway_provider_requests_total{status="success"}[5m]))
```
**Description**: Average token consumption per successful request.

### Average Tokens per Request by Provider
```promql
sum by (provider) (rate(gateway_provider_tokens_total[5m])) 
/ 
sum by (provider) (rate(gateway_provider_requests_total{status="success"}[5m]))
```
**Description**: Average tokens per request per provider.

### Strategy Token Usage
```promql
sum by (strategy, direction) (rate(gateway_provider_tokens_total[5m]))
```
**Description**: Token usage grouped by strategy and direction.

---

## 4. Latency & Performance

### P50 Latency by Tier
```promql
histogram_quantile(0.50, sum by (le, tier) (rate(gateway_provider_latency_seconds_bucket[5m])))
```
**Description**: Median latency per tier.

### P95 Latency by Tier
```promql
histogram_quantile(0.95, sum by (le, tier) (rate(gateway_provider_latency_seconds_bucket[5m])))
```
**Description**: 95th percentile latency per tier.

### P99 Latency by Tier
```promql
histogram_quantile(0.99, sum by (le, tier) (rate(gateway_provider_latency_seconds_bucket[5m])))
```
**Description**: 99th percentile latency per tier.

### P50 Latency by Provider
```promql
histogram_quantile(0.50, sum by (le, provider) (rate(gateway_provider_latency_seconds_bucket[5m])))
```
**Description**: Median latency per provider.

### P95 Latency by Provider
```promql
histogram_quantile(0.95, sum by (le, provider) (rate(gateway_provider_latency_seconds_bucket[5m])))
```
**Description**: 95th percentile latency per provider.

### P50 Latency by Strategy
```promql
histogram_quantile(0.50, sum by (le, strategy) (rate(gateway_provider_latency_seconds_bucket[5m])))
```
**Description**: Median latency per strategy.

### HTTP Request Duration by Path
```promql
histogram_quantile(0.50, sum by (le, path) (rate(gateway_http_request_duration_seconds_bucket[5m])))
```
**Description**: End-to-end HTTP request duration by API path.

### Latency Distribution by Tier (full histogram)
```promql
sum by (le, tier) (rate(gateway_provider_latency_seconds_bucket[5m]))
```
**Description**: Full latency histogram for each tier.

---

## 5. Routing Behavior

### Routing Attempts Distribution
```promql
sum by (le) (rate(gateway_routing_attempts_total_bucket[5m]))
```
**Description**: Distribution of routing attempts per request.

### Average Routing Attempts by Tier
```promql
sum by (tier) (rate(gateway_routing_attempts_total_sum[5m])) 
/ 
sum by (tier) (rate(gateway_routing_attempts_total_count[5m]))
```
**Description**: Average number of provider attempts before success.

### Failover Events
```promql
sum by (tier) (rate(gateway_failover_events_total[5m]))
```
**Description**: Failover events per tier.

### Failover Flow (from → to provider)
```promql
sum by (from_provider, to_provider) (rate(gateway_failover_events_total[5m]))
```
**Description**: Provider-to-provider failover flow matrix.

### Retry Success Rate
```promql
sum by (tier) (rate(gateway_retry_success_total[5m])) 
/ 
sum by (tier) (rate(gateway_provider_requests_total{status=~"error|timeout|rate_limited"}[5m]))
```
**Description**: Percentage of failed requests that succeeded on retry.

### Routing Attempts by Strategy
```promql
sum by (strategy) (rate(gateway_routing_attempts_total_sum[5m])) 
/ 
sum by (strategy) (rate(gateway_routing_attempts_total_count[5m]))
```
**Description**: Average routing attempts by strategy.

---

## 6. Streaming Performance

### Stream Duration P50
```promql
histogram_quantile(0.50, sum by (le, tier) (rate(gateway_stream_duration_seconds_bucket[5m])))
```
**Description**: Median streaming response duration.

### Stream Duration P95
```promql
histogram_quantile(0.95, sum by (le, tier) (rate(gateway_stream_duration_seconds_bucket[5m])))
```
**Description**: 95th percentile streaming duration.

### Time to First Byte P50
```promql
histogram_quantile(0.50, sum by (le, tier) (rate(gateway_stream_ttfb_seconds_bucket[5m])))
```
**Description**: Median time to first token.

### Time to First Byte P95
```promql
histogram_quantile(0.95, sum by (le, tier) (rate(gateway_stream_ttfb_seconds_bucket[5m])))
```
**Description**: 95th percentile time to first token.

### Stream Output Tokens by Tier
```promql
sum by (tier) (rate(gateway_stream_output_tokens_total[5m]))
```
**Description**: Streaming output token rate.

### Stream Output Tokens by Provider
```promql
sum by (provider) (rate(gateway_stream_output_tokens_total[5m]))
```
**Description**: Streaming output tokens per provider.

---

## 7. Resilience & Circuit Breakers

### Circuit Breaker State by Provider
```promql
gateway_circuit_breaker_state
```
**Description**: Current circuit breaker state (0=closed, 1=half-open, 2=open).

### Circuit Breaker Transitions
```promql
sum by (provider, model, from_state, to_state) (rate(gateway_circuit_breaker_transitions_total[5m]))
```
**Description**: Circuit breaker state transitions.

---

## 8. Quota & Limits

### Quota Usage Ratio by Provider
```promql
gateway_quota_usage_ratio
```
**Description**: Current quota usage as ratio (0.0-1.0).

### Quota Usage by Quota Type
```promql
sum by (quota_type) (gateway_quota_usage_ratio)
```
**Description**: Quota usage aggregated by type (rpm, tpm, etc.).

### Quota Rejections
```promql
sum by (provider, model, quota_type) (rate(gateway_quota_rejections_total[5m]))
```
**Description**: Quota limit rejections.

---

## 9. Cross-Dimensional Analytics

### Requests: Tier × Strategy
```promql
sum by (tier, strategy) (rate(gateway_http_requests_total[5m]))
```
**Description**: Request volume cross-section.

### Success Rate: Tier × Strategy
```promql
sum by (tier, strategy) (rate(gateway_provider_requests_total{status="success"}[5m])) 
/ 
sum by (tier, strategy) (rate(gateway_provider_requests_total[5m]))
```
**Description**: Success rate matrix.

### Latency P50: Tier × Provider
```promql
histogram_quantile(0.50, sum by (le, tier, provider) (rate(gateway_provider_latency_seconds_bucket[5m])))
```
**Description**: Median latency per tier and provider combination.

### Token Usage: Tier × Strategy
```promql
sum by (tier, strategy, direction) (rate(gateway_provider_tokens_total[5m]))
```
**Description**: Token consumption matrix.

---

## 10. Alerting Queries

### High Error Rate Alert
```promql
sum(rate(gateway_provider_requests_total{status!="success"}[5m])) / sum(rate(gateway_provider_requests_total[5m])) > 0.1
```
**Alert**: Error rate exceeds 10%.

### High Latency Alert (P95 > 30s)
```promql
histogram_quantile(0.95, sum by (le, tier) (rate(gateway_provider_latency_seconds_bucket[5m]))) > 30
```
**Alert**: 95th percentile latency exceeds 30 seconds.

### Circuit Breaker Open Alert
```promql
gateway_circuit_breaker_state == 2
```
**Alert**: Circuit breaker is open for any provider.

### Quota Usage High Alert (> 90%)
```promql
gateway_quota_usage_ratio > 0.9
```
**Alert**: Quota usage exceeds 90%.

### High Failover Rate Alert
```promql
sum(rate(gateway_failover_events_total[5m])) > 10
```
**Alert**: More than 10 failover events per second.

---

## Dashboard Layout Recommendations

### Row 1: Overview
- HTTP Requests by Tier (graph)
- Active Requests In Flight (single value)
- Overall Success Rate (gauge)

### Row 2: Request Volume
- HTTP Requests by Path & Method (table)
- Provider Requests by Provider (bar chart)
- Active Requests Over Time (time series)

### Row 3: Token Usage
- Total Tokens by Tier (stack)
- Tokens by Direction (pie chart)
- Average Tokens per Request (bar)

### Row 4: Latency
- P50/P95/P99 Latency by Tier (multi-line)
- Latency by Provider (heatmap)
- HTTP Duration by Path (table)

### Row 5: Errors & Routing
- Error Rate by Tier (graph)
- Failover Events (graph)
- Routing Attempts Distribution (histogram)

### Row 6: Streaming
- Stream Duration P50/P95 (multi-line)
- Time to First Byte (graph)
- Stream Output Tokens (stack)

### Row 7: Resilience
- Circuit Breaker State (state timeline)
- Quota Usage Ratio (heatmap)
- Retry Success Rate (gauge)

### Row 8: Cross-Dimensional
- Tier × Strategy Heatmap
- Latency Matrix (Tier × Provider)
- Success Rate Matrix (Tier × Provider)
