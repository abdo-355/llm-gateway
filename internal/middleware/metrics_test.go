package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	var matchedPath string
	var unmatchedPath string
	r.Use(func(c *gin.Context) {
		c.Next()
		if c.FullPath() == "" {
			unmatchedPath = metricPath(c)
		} else {
			matchedPath = metricPath(c)
		}
	})
	r.GET("/items/:id", func(c *gin.Context) {})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items/123", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/arbitrary/user/path", nil))

	assert.Equal(t, "/items/:id", matchedPath)
	assert.Equal(t, unmatchedRouteMetricPath, unmatchedPath)
}

func TestMetricsUsesConstantLabelForUnmatchedRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Metrics())

	boundedBefore := requestCounterValue(t, http.MethodGet, unmatchedRouteMetricPath)
	rawBefore := requestCounterValue(t, http.MethodGet, "/arbitrary/user/path")

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/arbitrary/user/path", nil))

	assert.Equal(t, boundedBefore+1, requestCounterValue(t, http.MethodGet, unmatchedRouteMetricPath))
	assert.Equal(t, rawBefore, requestCounterValue(t, http.MethodGet, "/arbitrary/user/path"))
}

func requestCounterValue(t *testing.T, method, path string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "gateway_http_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			labels := make(map[string]string, len(metric.Label))
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["method"] == method && labels["path"] == path &&
				labels["status"] == "404" && labels["tier"] == "unknown" && labels["strategy"] == "default" {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}
