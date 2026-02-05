package errors

import "fmt"

// ProviderError is the base error type for provider-related errors
type ProviderError struct {
	Message     string
	StatusCode  int
	IsRetryable bool
	Headers     map[string]string
}

func (e *ProviderError) Error() string {
	return e.Message
}

// RateLimitError represents a rate limit error (429)
type RateLimitError struct {
	ProviderError
	RetryAfter int    // seconds until reset
	LimitType  string // rpm, tpm, daily
}

func NewRateLimitError(message string, retryAfter int, limitType string) *RateLimitError {
	return &RateLimitError{
		ProviderError: ProviderError{
			Message:     message,
			StatusCode:  429,
			IsRetryable: true,
		},
		RetryAfter: retryAfter,
		LimitType:  limitType,
	}
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s (retry after: %ds, limit type: %s)", e.Message, e.RetryAfter, e.LimitType)
}

type CircuitBreakerError struct {
	ProviderError
	ProviderID string
	State      string // OPEN, HALF_OPEN
}

func NewCircuitBreakerError(message, providerID, state string) *CircuitBreakerError {
	return &CircuitBreakerError{
		ProviderError: ProviderError{
			Message:     message,
			StatusCode:  503,
			IsRetryable: true,
		},
		ProviderID: providerID,
		State:      state,
	}
}

func (e *CircuitBreakerError) Error() string {
	return fmt.Sprintf("%s (provider: %s, state: %s)", e.Message, e.ProviderID, e.State)
}

type TimeoutError struct {
	ProviderError
	TimeoutType string // request, inactivity
}

func NewTimeoutError(message, timeoutType string) *TimeoutError {
	return &TimeoutError{
		ProviderError: ProviderError{
			Message:     message,
			StatusCode:  504,
			IsRetryable: true,
		},
		TimeoutType: timeoutType,
	}
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("%s (type: %s)", e.Message, e.TimeoutType)
}

type ModelQuotaExceededError struct {
	ProviderError
	ProviderID string
	Model      string
	LimitType  string // rpm, rph, rpd, tpm, tph, tpd, tpmu
}

func NewModelQuotaExceededError(message, providerID, model, limitType string) *ModelQuotaExceededError {
	return &ModelQuotaExceededError{
		ProviderError: ProviderError{
			Message:     message,
			StatusCode:  429,
			IsRetryable: true,
		},
		ProviderID: providerID,
		Model:      model,
		LimitType:  limitType,
	}
}

func (e *ModelQuotaExceededError) Error() string {
	return fmt.Sprintf("%s (provider: %s, model: %s, limit: %s)", e.Message, e.ProviderID, e.Model, e.LimitType)
}

type PaymentRequiredError struct {
	ProviderError
}

func NewPaymentRequiredError(message string) *PaymentRequiredError {
	return &PaymentRequiredError{
		ProviderError: ProviderError{
			Message:     message,
			StatusCode:  402,
			IsRetryable: false,
		},
	}
}

type ValidationError struct {
	ProviderError
	Details []ValidationDetail
}

type ValidationDetail struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func NewValidationError(message string, details []ValidationDetail) *ValidationError {
	return &ValidationError{
		ProviderError: ProviderError{
			Message:     message,
			StatusCode:  400,
			IsRetryable: false,
		},
		Details: details,
	}
}

type GatewayErrorClass struct {
	Type      string
	Code      string
	Message   string
	RequestID string
	Details   map[string]any
}

func (e *GatewayErrorClass) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Type, e.Code, e.Message)
}

func NewGatewayError(errorType, code, message, requestID string, details map[string]any) *GatewayErrorClass {
	return &GatewayErrorClass{
		Type:      errorType,
		Code:      code,
		Message:   message,
		RequestID: requestID,
		Details:   details,
	}
}
