package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abdo-355/llm-gateway/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHealthMonitor struct {
	metrics []services.HealthMetrics
}

func (m *mockHealthMonitor) GetAllHealthMetrics(_ context.Context) []services.HealthMetrics {
	return m.metrics
}

func setupHealthRouter(monitor HealthMonitor) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", Health(monitor))
	return r
}

func TestHealth_AllCircuitsClosed(t *testing.T) {
	monitor := &mockHealthMonitor{
		metrics: []services.HealthMetrics{
			{ProviderID: "groq", Model: "llama-3.1-8b-instant", CircuitState: services.StateClosed},
			{ProviderID: "mistral", Model: "mistral-large-latest", CircuitState: services.StateClosed},
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	setupHealthRouter(monitor).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "healthy", body["status"])
}

func TestHealth_OneCircuitOpen(t *testing.T) {
	monitor := &mockHealthMonitor{
		metrics: []services.HealthMetrics{
			{ProviderID: "groq", Model: "llama-3.1-8b-instant", CircuitState: services.StateClosed},
			{ProviderID: "mistral", Model: "mistral-large-latest", CircuitState: services.StateOpen},
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	setupHealthRouter(monitor).ServeHTTP(w, req)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "degraded", body["status"])
}

func TestHealth_TimestampIsRFC3339(t *testing.T) {
	monitor := &mockHealthMonitor{
		metrics: []services.HealthMetrics{},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	setupHealthRouter(monitor).ServeHTTP(w, req)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	ts, ok := body["timestamp"].(string)
	require.True(t, ok, "timestamp should be a string")
	_, err := time.Parse(time.RFC3339, ts)
	assert.NoError(t, err, "timestamp should be parseable as RFC3339")
}

func TestHealth_ResponseHasModelsArray(t *testing.T) {
	monitor := &mockHealthMonitor{
		metrics: []services.HealthMetrics{
			{ProviderID: "groq", Model: "llama-3.1-8b-instant", CircuitState: services.StateClosed},
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	setupHealthRouter(monitor).ServeHTTP(w, req)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	models, ok := body["models"].([]any)
	require.True(t, ok, "models should be an array")
	assert.Len(t, models, 1)
}
