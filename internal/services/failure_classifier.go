package services

import (
	"strings"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
)

// FailureClassifier classifies errors into failure categories and decides actions
type FailureClassifier interface {
	// Classify categorizes an error and decides the appropriate action
	Classify(err error, ctx types.FailureContext) types.FailureDecision
	// Categorize maps an error to a failure category
	Categorize(err error) types.FailureCategory
}

// DefaultFailureClassifier is the default implementation
type DefaultFailureClassifier struct{}

// NewDefaultFailureClassifier creates a new default classifier
func NewDefaultFailureClassifier() *DefaultFailureClassifier {
	return &DefaultFailureClassifier{}
}

// Categorize maps an error to a failure category
func (c *DefaultFailureClassifier) Categorize(err error) types.FailureCategory {
	switch e := err.(type) {
	case *errors.NetworkError:
		return types.CategoryNetwork
	case *errors.TimeoutError:
		return types.CategoryTimeout
	case *errors.RateLimitError:
		return types.CategoryRateLimit
	case *errors.ParseError:
		return types.CategoryParse
	case *errors.EmptyResponseError:
		return types.CategoryEmpty
	case *errors.CircuitBreakerError:
		return types.CategoryCircuitBreaker
	case *errors.ModelQuotaExceededError:
		return types.CategoryQuota
	case *errors.PaymentRequiredError:
		return types.CategoryPayment
	case *errors.ValidationError:
		return types.CategoryValidation
	case *errors.ProviderError:
		if e.StatusCode >= 500 {
			return types.CategoryProvider5xx
		}
		return types.CategoryProvider4xx
	default:
		return types.CategoryUnknown
	}
}

// Classify categorizes an error and decides the appropriate action
func (c *DefaultFailureClassifier) Classify(err error, ctx types.FailureContext) types.FailureDecision {
	category := c.Categorize(err)

	decision := types.FailureDecision{
		Category:            category,
		IsRetryable:         c.isRetryable(category),
		ShouldRecordFailure: true,
		ShouldRecordSuccess: false,
	}

	// Determine action based on category
	switch category {
	case types.CategoryNetwork:
		if c.isCanceledNetworkError(err) {
			decision.Action = types.ActionFailover
			decision.Reason = "upstream request canceled, trying different provider"
			break
		}

		decision.Action = types.ActionRetryWithBackoff
		decision.BackoffMs = c.calculateBackoff(ctx.AttemptIndex)
		decision.Reason = "transient network/timeout error"

	case types.CategoryTimeout:
		decision.Action = types.ActionFailover
		decision.Reason = "provider request timed out, trying different provider"

	case types.CategoryRateLimit:
		decision.ShouldRecordFailure = false
		subtype := c.extractRateLimitSubtype(err)
		switch subtype {
		case "quota_exhausted":
			decision.Action = types.ActionFailover
			decision.IsRetryable = false
			decision.Reason = "quota/billing exhausted, trying different provider"
		case "overload":
			decision.Action = types.ActionFailover
			decision.Reason = "provider overloaded, trying different provider"
		default:
			if ctx.AttemptIndex < ctx.MaxAttempts-1 {
				decision.Action = types.ActionFailover
				decision.Reason = "rate limited, trying different provider"
			} else {
				decision.Action = types.ActionCooldown
				decision.CooldownSeconds = c.extractRetryAfter(err)
				decision.Reason = "all providers rate limited, applying cooldown"
			}
		}

	case types.CategoryProvider5xx:
		decision.Action = types.ActionRetryWithBackoff
		decision.BackoffMs = c.calculateBackoff(ctx.AttemptIndex)
		decision.Reason = "provider server error, retrying"

	case types.CategoryParse, types.CategoryEmpty:
		decision.Action = types.ActionFailover
		decision.Reason = "parsing/empty response issue, trying different provider"

	case types.CategoryPayment, types.CategoryValidation:
		decision.Action = types.ActionAbort
		decision.IsRetryable = false
		decision.Reason = "non-retryable client error"

	case types.CategoryCircuitBreaker:
		decision.Action = types.ActionFailover
		decision.ShouldRecordFailure = false
		decision.Reason = "circuit breaker open, failover"

	case types.CategoryQuota:
		decision.ShouldRecordFailure = false
		decision.Action = types.ActionFailover
		decision.Reason = "quota exceeded, trying different provider"

	default:
		decision.Action = types.ActionAbort
		decision.Reason = "unknown error type"
	}

	// Check retry budget
	if !ctx.HasRemainingBudget {
		decision.Action = types.ActionAbort
		decision.Reason = "retry budget exhausted"
	}

	return decision
}

// isRetryable returns whether the category is generally retryable
func (c *DefaultFailureClassifier) isRetryable(category types.FailureCategory) bool {
	switch category {
	case types.CategoryNetwork, types.CategoryTimeout, types.CategoryProvider5xx, types.CategoryEmpty:
		return true
	case types.CategoryRateLimit:
		return true
	case types.CategoryPayment, types.CategoryValidation:
		return false
	default:
		return true
	}
}

// calculateBackoff returns backoff in milliseconds based on attempt number
func (c *DefaultFailureClassifier) calculateBackoff(attemptIndex int) int {
	// Simple exponential backoff: base * 2^attempt
	// With max of 8 seconds
	baseBackoff := 250 // ms
	backoff := baseBackoff

	for i := 0; i < attemptIndex && backoff < 8000; i++ {
		backoff *= 2
	}

	if backoff > 8000 {
		backoff = 8000
	}

	return backoff
}

// extractRetryAfter extracts retry-after from error if available
func (c *DefaultFailureClassifier) extractRetryAfter(err error) int {
	if rateErr, ok := err.(*errors.RateLimitError); ok {
		return rateErr.RetryAfter
	}
	return 60
}

// extractRateLimitSubtype extracts the limit subtype from a rate limit error
func (c *DefaultFailureClassifier) extractRateLimitSubtype(err error) string {
	if rateErr, ok := err.(*errors.RateLimitError); ok {
		return rateErr.LimitSubtype
	}
	return "rate_limit"
}

func (c *DefaultFailureClassifier) isCanceledNetworkError(err error) bool {
	networkErr, ok := err.(*errors.NetworkError)
	if !ok {
		return false
	}

	errText := strings.ToLower(networkErr.Error())
	return strings.Contains(errText, "context canceled") || strings.Contains(errText, "context cancelled")
}
