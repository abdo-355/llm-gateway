package verification

import (
	"bytes"
	"context"
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
	assert.Contains(t, contents, "Skipped")
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

func TestPrintReportIncludesDetailedSkips(t *testing.T) {
	report := &Report{
		Results: []ProbeResult{
			{Provider: "groq", Model: "m1", Endpoint: "https://api.groq.com/openai/v1/chat/completions", Probe: "basic_text", Status: "SKIP", HTTPStatus: 429, Failure: "rate_limited: retry_after=2 limit_type=rpm"},
			{Provider: "groq", Model: "m1", Endpoint: "https://api.groq.com/openai/v1/chat/completions", Probe: "tools", Status: "SKIP", Failure: "not applicable for configured provider capabilities"},
		},
	}

	var output bytes.Buffer
	PrintReport(&output, report)

	contents := output.String()
	assert.Contains(t, contents, "Skipped")
	assert.Contains(t, contents, "rate_limited: retry_after=2 limit_type=rpm")
	assert.NotContains(t, contents, "not applicable for configured provider capabilities")
	assert.Contains(t, contents, "Failures\nNone")
}

func TestRunSkipsRemainingProbesAfterRateLimit(t *testing.T) {
	client := &fakeProbeClient{
		callResult: requestResult{HTTPStatus: 429, Failure: "rate_limited: retry_after=60 limit_type=rpm", Attempted: true},
	}

	report, err := runWithClient(context.Background(), Config{
		Provider: "groq",
		Model:    "openai/gpt-oss-120b",
		Timeout:  time.Second,
	}, client)

	assert.NoError(t, err)
	assert.Len(t, report.Results, 9)

	assert.Equal(t, "basic_text", report.Results[0].Probe)
	assert.Equal(t, "FAIL", report.Results[0].Status)
	assert.Equal(t, 429, report.Results[0].HTTPStatus)
	assert.Equal(t, "rate_limited: retry_after=60 limit_type=rpm", report.Results[0].Failure)
	assert.Equal(t, 3, report.Results[0].Attempts)
	assert.Equal(t, 3, report.Results[0].TransientHits)

	for _, result := range report.Results[1:8] {
		assert.Equal(t, "SKIP", result.Status)
		assert.Equal(t, "deferred_after_transient_failures", result.Failure)
	}

	assert.Equal(t, "final_recovery_check", report.Results[8].Probe)
	assert.Equal(t, "FAIL", report.Results[8].Status)
	assert.Equal(t, 429, report.Results[8].HTTPStatus)
	assert.Equal(t, "recovery", report.Results[8].Phase)

	assert.Equal(t, 4, client.callCount)
	assert.Equal(t, 0, client.streamCount)
	assert.Len(t, report.ModelOutcomes, 1)
	assert.Equal(t, "deferred_after_recovery", report.ModelOutcomes[0].FinalStatus)
	assert.True(t, report.ModelOutcomes[0].Deferred)
	assert.True(t, report.ModelOutcomes[0].RecoveryAttempted)
	assert.False(t, report.ModelOutcomes[0].RecoverySucceeded)
	assert.Len(t, report.AttemptLogs, 4)
}

func TestRunPreservesNonRateLimitFailures(t *testing.T) {
	client := &fakeProbeClient{
		callResult:   requestResult{HTTPStatus: 500, Failure: "provider_error: status=500 message=boom", Attempted: true},
		streamResult: requestResult{HTTPStatus: 500, Failure: "provider_error: status=500 message=boom", Attempted: true},
	}

	report, err := runWithClient(context.Background(), Config{
		Provider: "groq",
		Model:    "qwen/qwen3-32b",
		Timeout:  time.Second,
	}, client)

	assert.NoError(t, err)
	assert.Len(t, report.Results, 8)
	assert.Equal(t, "FAIL", report.Results[0].Status)
	assert.Equal(t, 500, report.Results[0].HTTPStatus)
	assert.Equal(t, "provider_error: status=500 message=boom", report.Results[0].Failure)
	assert.Equal(t, "main", report.Results[0].Phase)
	assert.False(t, report.ModelOutcomes[0].Deferred)
	assert.Equal(t, "completed", report.ModelOutcomes[0].FinalStatus)
	assert.GreaterOrEqual(t, client.callCount, 1)
	assert.GreaterOrEqual(t, client.streamCount, 1)
}

type fakeProbeClient struct {
	callResult   requestResult
	streamResult requestResult
	callCount    int
	streamCount  int
}

func (f *fakeProbeClient) call(_ context.Context, _ Combo, _ types.ChatCompletionRequest) requestResult {
	f.callCount++
	return f.callResult
}

func (f *fakeProbeClient) stream(_ context.Context, _ Combo, _ types.ChatCompletionRequest) requestResult {
	f.streamCount++
	return f.streamResult
}
