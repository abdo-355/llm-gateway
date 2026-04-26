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

type SchemaValidationError struct {
	ProviderError
	Field   string
	Details string
}

func NewSchemaValidationError(field, message string) *SchemaValidationError {
	return &SchemaValidationError{
		ProviderError: ProviderError{
			Message:     fmt.Sprintf("JSON schema validation failed for field '%s': %s", field, message),
			StatusCode:  400,
			IsRetryable: false,
		},
		Field:   field,
		Details: message,
	}
}

func (e *SchemaValidationError) Error() string {
	return e.Message
}

// NetworkError represents network-level failures (connection, DNS, TLS, etc.)
type NetworkError struct {
	ProviderError
	NetworkType   string // connection, timeout, dns, tls, unknown
	ProviderID    string
	BaseURL       string
	OriginalError error
}

// NewNetworkError creates a new NetworkError with classification
func NewNetworkError(message, networkType, providerID, baseURL string, original error) *NetworkError {
	return &NetworkError{
		ProviderError: ProviderError{
			Message:     message,
			StatusCode:  502,
			IsRetryable: true, // Network errors are usually transient
		},
		NetworkType:   networkType,
		ProviderID:    providerID,
		BaseURL:       baseURL,
		OriginalError: original,
	}
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("%s: %v", e.Message, e.OriginalError)
}

func (e *NetworkError) Unwrap() error {
	return e.OriginalError
}

// ParseError represents response parsing failures (JSON, SSE, etc.)
type ParseError struct {
	ProviderError
	ParseType   string // json, sse, header
	RawContent  string // Truncated raw content that failed to parse
	ProviderID  string
	Model       string
}

// NewParseError creates a new ParseError with context
func NewParseError(message, parseType, providerID, model, rawContent string, original error) *ParseError {
	return &ParseError{
		ProviderError: ProviderError{
			Message:     message,
			StatusCode:  502,
			IsRetryable: false, // Parse errors usually mean bad response, not retryable
		},
		ParseType:  parseType,
		RawContent: rawContent,
		ProviderID: providerID,
		Model:      model,
	}
}

func (e *ParseError) Error() string {
	return e.Message
}

// EmptyResponseError represents empty or missing response body
type EmptyResponseError struct {
	ProviderError
	ProviderID string
	Model      string
	StatusCode int
}

// NewEmptyResponseError creates a new EmptyResponseError
func NewEmptyResponseError(providerID, model string, statusCode int) *EmptyResponseError {
	return &EmptyResponseError{
		ProviderError: ProviderError{
			Message:     fmt.Sprintf("Provider %s returned empty response body", providerID),
			StatusCode:  statusCode,
			IsRetryable: true, // Empty responses can be transient
		},
		ProviderID: providerID,
		Model:      model,
		StatusCode: statusCode,
	}
}

func (e *EmptyResponseError) Error() string {
	return e.Message
}
