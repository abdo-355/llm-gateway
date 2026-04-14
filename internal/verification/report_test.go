package verification

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestProviderSummaries(t *testing.T) {
	report := &Report{Results: []ProbeResult{
		{Provider: "groq", Model: "m1", Probe: "basic_text", Status: "PASS"},
		{Provider: "groq", Model: "m1", Probe: "stream", Status: "FAIL"},
		{Provider: "groq", Model: "m2", Probe: "tools", Status: "SKIP"},
		{Provider: "mistral", Model: "m3", Probe: "basic_text", Status: "FAIL"},
	}}

	summaries := providerSummaries(report)
	assert.Len(t, summaries, 2)
	assert.Equal(t, providerSummary{Provider: "groq", Models: 2, Passed: 1, Failed: 1, Skipped: 1, Total: 3}, summaries[0])
	assert.Equal(t, providerSummary{Provider: "mistral", Models: 1, Passed: 0, Failed: 1, Skipped: 0, Total: 1}, summaries[1])
}

func TestFeatureSummaries(t *testing.T) {
	report := &Report{Results: []ProbeResult{
		{Probe: "basic_text", Status: "PASS"},
		{Probe: "basic_text", Status: "FAIL"},
		{Probe: "tools", Status: "SKIP"},
	}}

	summaries := featureSummaries(report)
	assert.Len(t, summaries, 2)
	assert.Equal(t, featureSummary{Probe: "basic_text", Passed: 1, Failed: 1, Skipped: 0, Total: 2}, summaries[0])
	assert.Equal(t, featureSummary{Probe: "tools", Passed: 0, Failed: 0, Skipped: 1, Total: 1}, summaries[1])
}

func TestPrintReport(t *testing.T) {
	report := &Report{
		StartedAt: time.Unix(100, 0).UTC(),
		EndedAt:   time.Unix(160, 0).UTC(),
		Results: []ProbeResult{
			{Provider: "groq", Model: "m1", Endpoint: "https://api.groq.com/openai/v1/chat/completions", Probe: "basic_text", Status: "PASS"},
			{Provider: "groq", Model: "m1", Endpoint: "https://api.groq.com/openai/v1/chat/completions", Probe: "stream", Status: "FAIL", Failure: "missing [DONE]", HTTPStatus: 200},
		},
	}

	var output bytes.Buffer
	PrintReport(&output, report)

	contents := output.String()
	assert.Contains(t, contents, "Upstream Model Verification Report")
	assert.Contains(t, contents, "Provider Summary")
	assert.Contains(t, contents, "Feature Summary")
	assert.Contains(t, contents, "Failures")
	assert.Contains(t, contents, "missing [DONE]")
	assert.True(t, strings.Contains(contents, "Passed: 1"))
	assert.True(t, strings.Contains(contents, "Failed: 1"))
	assert.True(t, strings.Contains(contents, "Skipped: 0"))
}

func TestComboRPMLimit(t *testing.T) {
	modelRPM := 5
	providerRPM := 60

	assert.Equal(t, 5, comboRPMLimit(Combo{
		Provider: types.ProviderConfig{Limits: types.ProviderLimits{Rpm: &providerRPM}},
		Limits:   types.ModelLimits{Rpm: &modelRPM},
	}))

	assert.Equal(t, 60, comboRPMLimit(Combo{
		Provider: types.ProviderConfig{Limits: types.ProviderLimits{Rpm: &providerRPM}},
	}))
}

func TestValidateStrictJSON(t *testing.T) {
	assert.NoError(t, validateStrictJSON(`{"ok":true}`))
	assert.Error(t, validateStrictJSON(`{"missing":true}`))
}

func TestSupportsStrictJSON(t *testing.T) {
	assert.True(t, supportsStrictJSON(Combo{Provider: types.ProviderConfig{Capabilities: types.ProviderCapabilities{StructuredOutputs: "json_schema_strict"}}}))
	assert.True(t, supportsStrictJSON(Combo{StrictJSONCertified: true}))
	assert.False(t, supportsStrictJSON(Combo{Provider: types.ProviderConfig{Capabilities: types.ProviderCapabilities{StructuredOutputs: "model_dependent"}}}))
}

func TestProbeTokenPtr(t *testing.T) {
	assert.Equal(t, 8, *probeTokenPtr(Config{}, 8))
	assert.Equal(t, 64, *probeTokenPtr(Config{ProbeMaxTokens: 64}, 8))
}
