package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTP layer metrics.
var (
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status"})

	HTTPRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_http_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})

	HTTPRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gateway_http_requests_in_flight",
		Help: "Number of HTTP requests currently being processed.",
	})
)

// Provider layer metrics.
var (
	ProviderRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_provider_requests_total",
		Help: "Total number of requests to LLM providers.",
	}, []string{"provider", "model", "status"})

	ProviderLatencySeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_provider_latency_seconds",
		Help:    "Latency of LLM provider requests in seconds.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"provider", "model"})

	ProviderTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_provider_tokens_total",
		Help: "Total number of tokens processed by LLM providers.",
	}, []string{"provider", "model", "direction"})

	RoutingAttemptsTotal = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_routing_attempts_total",
		Help:    "Number of routing attempts per request.",
		Buckets: []float64{1, 2, 3, 4, 5},
	}, []string{"logical_model"})
)

// Circuit breaker & quota metrics.
var (
	CircuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_circuit_breaker_state",
		Help: "Current state of the circuit breaker (0=closed, 1=half-open, 2=open).",
	}, []string{"provider"})

	RateLimitRejectionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gateway_rate_limit_rejections_total",
		Help: "Total number of requests rejected due to rate limiting.",
	})
)

// Streaming metrics.
var (
	StreamDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_stream_duration_seconds",
		Help:    "Duration of streaming responses in seconds.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"provider", "model"})

	StreamTTFBSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_stream_ttfb_seconds",
		Help:    "Time to first byte for streaming responses in seconds.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"provider", "model"})
)

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
