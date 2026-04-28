package services

import (
	"context"
	"testing"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFailureClassifier_ClassifyTimeoutFailsOver(t *testing.T) {
	classifier := NewDefaultFailureClassifier()

	decision := classifier.Classify(errors.NewTimeoutError("timeout", "request"), types.FailureContext{
		AttemptIndex:       0,
		MaxAttempts:        3,
		HasRemainingBudget: true,
	})

	assert.Equal(t, types.CategoryTimeout, decision.Category)
	assert.Equal(t, types.ActionFailover, decision.Action)
	assert.Equal(t, "provider request timed out, trying different provider", decision.Reason)
	assert.True(t, decision.IsRetryable)
	assert.Zero(t, decision.BackoffMs)
}

func TestFailureClassifier_ClassifyCanceledNetworkFailsOver(t *testing.T) {
	classifier := NewDefaultFailureClassifier()
	err := errors.NewNetworkError("Network error calling provider", "unknown", "nim", "https://example.com", context.Canceled)

	decision := classifier.Classify(err, types.FailureContext{
		AttemptIndex:       0,
		MaxAttempts:        3,
		HasRemainingBudget: true,
	})

	assert.Equal(t, types.CategoryNetwork, decision.Category)
	assert.Equal(t, types.ActionFailover, decision.Action)
	assert.Equal(t, "upstream request canceled, trying different provider", decision.Reason)
	assert.True(t, decision.IsRetryable)
	assert.Zero(t, decision.BackoffMs)
}

func TestFailureClassifier_ClassifyTransientNetworkRetries(t *testing.T) {
	classifier := NewDefaultFailureClassifier()
	err := errors.NewNetworkError("Network error calling provider", "connection", "nim", "https://example.com", assert.AnError)

	decision := classifier.Classify(err, types.FailureContext{
		AttemptIndex:       1,
		MaxAttempts:        3,
		HasRemainingBudget: true,
	})

	assert.Equal(t, types.CategoryNetwork, decision.Category)
	assert.Equal(t, types.ActionRetryWithBackoff, decision.Action)
	assert.Equal(t, 500, decision.BackoffMs)
	assert.Equal(t, "transient network/timeout error", decision.Reason)
	assert.True(t, decision.IsRetryable)
	require.NotZero(t, decision.BackoffMs)
}
