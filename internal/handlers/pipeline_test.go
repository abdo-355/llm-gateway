package handlers

import (
	"testing"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizePlanAttempts(t *testing.T) {
	plan := types.RoutingPlan{Attempts: []types.RoutingAttempt{
		{ProviderID: "provider-a", Model: "model-1", BaseURL: "https://a.example/v1", APIKey: "secret", Score: 2.5, TimeoutMs: 1000},
		{ProviderID: "provider-b", Model: "model-2", BaseURL: "https://b.example/v1", APIKey: "secret", Score: 2.1, TimeoutMs: 2000},
	}}

	summary := summarizePlanAttempts(plan, 1)

	require.Len(t, summary, 1)
	assert.Equal(t, 1, summary[0].Attempt)
	assert.Equal(t, "provider-a", summary[0].ProviderID)
	assert.Equal(t, "model-1", summary[0].Model)
	assert.Equal(t, 2.5, summary[0].Score)
	assert.Equal(t, 1000, summary[0].TimeoutMs)
}

func TestSummarizePlanAttemptsEmpty(t *testing.T) {
	assert.Nil(t, summarizePlanAttempts(types.RoutingPlan{}, 5))
	assert.Nil(t, summarizePlanAttempts(types.RoutingPlan{Attempts: []types.RoutingAttempt{{ProviderID: "provider-a"}}}, 0))
}
