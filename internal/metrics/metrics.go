package metrics

import (
	"sync"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var registerModelInfoOnce sync.Once

var (
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status", "tier", "strategy"})

	HTTPRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_http_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "tier", "strategy"})

	HTTPRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gateway_http_requests_in_flight",
		Help: "Number of HTTP requests currently being processed.",
	})
)

var (
	ProviderRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_provider_requests_total",
		Help: "Total number of requests to LLM providers.",
	}, []string{"provider", "model", "status", "tier", "strategy", "error_type"})

	ProviderLatencySeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_provider_latency_seconds",
		Help:    "Latency of LLM provider requests in seconds.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"provider", "model", "tier", "strategy"})

	ProviderTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_provider_tokens_total",
		Help: "Total number of tokens processed by LLM providers.",
	}, []string{"provider", "model", "direction", "tier", "strategy"})

	RoutingAttemptsTotal = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_routing_attempts_total",
		Help:    "Number of routing attempts per request.",
		Buckets: []float64{1, 2, 3, 4, 5},
	}, []string{"tier", "strategy"})

	ModelInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_model_info",
		Help: "Static metadata for configured provider models.",
	}, []string{"provider", "model", "tier", "strict_json"})
)

var (
	FailoverEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_failover_events_total",
		Help: "Total number of provider failover events.",
	}, []string{"from_provider", "to_provider", "tier"})

	RetrySuccessTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_retry_success_total",
		Help: "Total number of successful requests after retry.",
	}, []string{"tier"})
)

var (
	CircuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_circuit_breaker_state",
		Help: "Current state of the circuit breaker (0=closed, 1=half-open, 2=open).",
	}, []string{"provider", "model"})

	CircuitBreakerTransitionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_circuit_breaker_transitions_total",
		Help: "Total number of circuit breaker state transitions.",
	}, []string{"provider", "model", "from_state", "to_state"})
)

var (
	QuotaUsageRatio = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_quota_usage_ratio",
		Help: "Current quota usage as a ratio (0.0-1.0).",
	}, []string{"provider", "model", "quota_type"})

	QuotaRejectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_quota_rejections_total",
		Help: "Total number of requests rejected due to quota limits.",
	}, []string{"provider", "model", "quota_type"})
)

var (
	RateLimitRejectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_rate_limit_rejections_total",
		Help: "Total number of requests rejected due to rate limiting.",
	}, []string{"reason"})
)

var (
	AuthRejectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_auth_rejections_total",
		Help: "Total number of requests rejected due to authentication failures.",
	}, []string{"reason"})
)

var (
	StreamDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_stream_duration_seconds",
		Help:    "Duration of streaming responses in seconds.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"provider", "model", "tier", "strategy"})

	StreamTTFBSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_stream_ttfb_seconds",
		Help:    "Time to first byte for streaming responses in seconds.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"provider", "model", "tier", "strategy"})

	StreamOutputTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_stream_output_tokens_total",
		Help: "Total number of output tokens from streaming responses.",
	}, []string{"provider", "model", "tier", "strategy"})
)

func RegisterModelInfo(cfg types.AppConfig) {
	registerModelInfoOnce.Do(func() {
		strictJSON := map[string]bool{}
		for _, cert := range cfg.Certifications {
			if cert.StrictSchema {
				strictJSON[cert.Provider+"/"+cert.Model] = true
			}
		}

		for _, provider := range cfg.Providers {
			for _, model := range provider.Models.List {
				attr := provider.Models.Attributes[model]
				strict := "false"
				if strictJSON[provider.ID+"/"+model] || provider.Capabilities.StructuredOutputs == "json_schema_strict" {
					strict = "true"
				}
				ModelInfo.WithLabelValues(
					provider.ID,
					model,
					string(attr.Tier),
					strict,
				).Set(1)
			}
		}
	})
}

func CircuitStateToFloat64(state string) float64 {
	switch state {
	case "CLOSED":
		return 0
	case "HALF_OPEN":
		return 1
	case "OPEN":
		return 2
	default:
		return 0
	}
}
