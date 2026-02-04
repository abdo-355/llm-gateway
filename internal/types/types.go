// Package types contains all domain types for the LLM Gateway.
// These types mirror the TypeScript implementation for compatibility.
package types

import (
	"encoding/json"
	"fmt"
)

// OpenAIMessage represents a chat message in OpenAI format
type OpenAIMessage struct {
	Role       string      `json:"role"`                   // system, user, assistant, tool
	Content    interface{} `json:"content,omitempty"`      // string or array of content parts
	Name       string      `json:"name,omitempty"`         // optional name
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`   // tool calls made by assistant
	ToolCallID string      `json:"tool_call_id,omitempty"` // ID of tool call (for tool role)
}

// ContentPart represents a part of a message content (text or image)
type ContentPart struct {
	Type     string `json:"type"`           // text or image_url
	Text     string `json:"text,omitempty"` // for text type
	ImageURL *struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"` // auto, low, high
	} `json:"image_url,omitempty"` // for image_url type
}

// ToolCall represents a tool call in a message
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // function
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// OpenAITool represents a tool definition
type OpenAITool struct {
	Type     string   `json:"type"` // function
	Function Function `json:"function"`
}

// Function represents function parameters
type Function struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"` // JSON schema
	Strict      *bool           `json:"strict,omitempty"`
}

// ResponseFormat represents the response format configuration
type ResponseFormat struct {
	Type       string      `json:"type"` // text, json_object, json_schema
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

// JSONSchema represents a JSON schema for structured output
type JSONSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      *bool           `json:"strict,omitempty"`
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Messages            []OpenAIMessage   `json:"messages"`
	Model               string            `json:"model"`
	FrequencyPenalty    *float64          `json:"frequency_penalty,omitempty"`
	LogitBias           map[string]int    `json:"logit_bias,omitempty"`
	Logprobs            *bool             `json:"logprobs,omitempty"`
	TopLogprobs         *int              `json:"top_logprobs,omitempty"`
	MaxTokens           *int              `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
	N                   *int              `json:"n,omitempty"`
	PresencePenalty     *float64          `json:"presence_penalty,omitempty"`
	ResponseFormat      *ResponseFormat   `json:"response_format,omitempty"`
	Seed                *int              `json:"seed,omitempty"`
	Stop                interface{}       `json:"stop,omitempty"` // string or []string
	Stream              *bool             `json:"stream,omitempty"`
	StreamOptions       *StreamOptions    `json:"stream_options,omitempty"`
	Temperature         *float64          `json:"temperature,omitempty"`
	TopP                *float64          `json:"top_p,omitempty"`
	Tools               []OpenAITool      `json:"tools,omitempty"`
	ToolChoice          interface{}       `json:"tool_choice,omitempty"` // none, auto, required, or object
	ParallelToolCalls   *bool             `json:"parallel_tool_calls,omitempty"`
	User                string            `json:"user,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	Router              *RouterHints      `json:"router,omitempty"` // Internal gateway routing hints
}

// StreamOptions represents stream options
type StreamOptions struct {
	IncludeUsage *bool `json:"include_usage,omitempty"`
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"` // chat.completion
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int             `json:"index"`
	Message      ResponseMessage `json:"message"`
	Logprobs     *Logprobs       `json:"logprobs,omitempty"`
	FinishReason string          `json:"finish_reason"` // stop, length, tool_calls, content_filter, function_call
}

// ResponseMessage represents the message in a response
type ResponseMessage struct {
	Role      string     `json:"role"`
	Content   *string    `json:"content"` // null if tool calls present
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Refusal   *string    `json:"refusal,omitempty"`
}

// Logprobs represents log probabilities
type Logprobs struct {
	Content []LogprobContent `json:"content,omitempty"`
	Refusal []LogprobContent `json:"refusal,omitempty"`
}

// LogprobContent represents logprob for a token
type LogprobContent struct {
	Token       string           `json:"token"`
	Logprob     float64          `json:"logprob"`
	Bytes       []int            `json:"bytes,omitempty"`
	TopLogprobs []TopLogprobItem `json:"top_logprobs"`
}

// TopLogprobItem represents a top logprob item
type TopLogprobItem struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
	Bytes   []int   `json:"bytes,omitempty"`
}

// Usage represents token usage
type Usage struct {
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// PromptTokensDetails represents prompt token details
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// CompletionTokensDetails represents completion token details
type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// SSEChunk represents a streaming response chunk
type SSEChunk struct {
	ID                string        `json:"id"`
	Object            string        `json:"object"` // chat.completion.chunk
	Created           int64         `json:"created"`
	Model             string        `json:"model"`
	SystemFingerprint string        `json:"system_fingerprint,omitempty"`
	Choices           []DeltaChoice `json:"choices"`
	Usage             *Usage        `json:"usage,omitempty"`
}

// DeltaChoice represents a choice in a streaming chunk
type DeltaChoice struct {
	Index        int          `json:"index"`
	Delta        DeltaMessage `json:"delta"`
	Logprobs     *Logprobs    `json:"logprobs,omitempty"`
	FinishReason *string      `json:"finish_reason"`
}

// DeltaMessage represents the delta in a streaming chunk
type DeltaMessage struct {
	Role      string          `json:"role,omitempty"`
	Content   *string         `json:"content,omitempty"`
	ToolCalls []DeltaToolCall `json:"tool_calls,omitempty"`
	Refusal   *string         `json:"refusal,omitempty"`
}

// DeltaToolCall represents a tool call delta
type DeltaToolCall struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"` // function
	Function *DeltaFunction `json:"function,omitempty"`
}

// DeltaFunction represents function delta
type DeltaFunction struct {
	Name      *string `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}

// RouterHints represents routing configuration from request
type RouterHints struct {
	Profile      *string              `json:"profile,omitempty"` // cheap_fast, reliable_structured, balanced
	Requirements *RouterRequirements  `json:"requirements,omitempty"`
	Budget       *BudgetConfig        `json:"budget,omitempty"`
	SLO          *SLOConfig           `json:"slo,omitempty"`
	Providers    *ProviderPreferences `json:"providers,omitempty"`
	Fallback     *FallbackConfig      `json:"fallback,omitempty"`
	Trace        *TraceConfig         `json:"trace,omitempty"`
}

// RouterRequirements represents routing requirements
type RouterRequirements struct {
	Output    *string `json:"output,omitempty"`    // text, json_schema_strict
	Streaming *string `json:"streaming,omitempty"` // required, preferred, forbidden
	Tools     *string `json:"tools,omitempty"`     // required, allowed, forbidden
}

// BudgetConfig represents budget configuration
type BudgetConfig struct {
	Mode string `json:"mode"` // free_only, allow_paid
}

// SLOConfig represents SLO configuration
type SLOConfig struct {
	MaxLatencyMs  *int `json:"max_latency_ms,omitempty"`
	HardTimeoutMs *int `json:"hard_timeout_ms,omitempty"`
}

// ProviderPreferences represents provider preferences
type ProviderPreferences struct {
	Allow  []string `json:"allow,omitempty"`
	Deny   []string `json:"deny,omitempty"`
	Prefer []string `json:"prefer,omitempty"`
}

// FallbackConfig represents fallback configuration
type FallbackConfig struct {
	MaxAttempts *int  `json:"max_attempts,omitempty"`
	On429       *bool `json:"on_429,omitempty"`
	OnTimeout   *bool `json:"on_timeout,omitempty"`
	On5xx       *bool `json:"on_5xx,omitempty"`
}

// TraceConfig represents trace configuration
type TraceConfig struct {
	RequestID *string  `json:"request_id,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// ProviderConfig represents a provider configuration
type ProviderConfig struct {
	ID           string               `json:"id"`
	BaseURL      string               `json:"baseUrl"`
	Auth         ProviderAuth         `json:"auth"`
	Models       ProviderModels       `json:"models"`
	Capabilities ProviderCapabilities `json:"capabilities"`
	Limits       ProviderLimits       `json:"limits"`
	ProviderType string               `json:"providerType,omitempty"` // openai, vertex (defaults to openai)
}

// ProviderAuth represents provider authentication configuration
type ProviderAuth struct {
	Type       string `json:"type"`                 // none, bearer, header
	Env        string `json:"env"`                  // environment variable name
	HeaderName string `json:"headerName,omitempty"` // for header auth type
}

// ProviderModels represents model configuration for a provider
type ProviderModels struct {
	Mode   string                 `json:"mode"`             // allowlist, denylist
	List   []string               `json:"list"`             // model names
	Limits map[string]ModelLimits `json:"limits,omitempty"` // per-model limits
}

// ProviderCapabilities represents provider capabilities
type ProviderCapabilities struct {
	Streaming         bool   `json:"streaming"`
	Tools             bool   `json:"tools"`
	StructuredOutputs string `json:"structuredOutputs"` // none, json_object, json_schema_strict, model_dependent, unknown
}

// ProviderLimits represents provider-level limits
type ProviderLimits struct {
	Rpm           *int `json:"rpm,omitempty"`
	Tpm           *int `json:"tpm,omitempty"`
	DailyRequests *int `json:"dailyRequests,omitempty"`
	MaxConcurrent *int `json:"maxConcurrent,omitempty"`
	// Extended limits
	Rph *int `json:"rph,omitempty"` // Requests per hour
	Rpd *int `json:"rpd,omitempty"` // Requests per day
	Tph *int `json:"tph,omitempty"` // Tokens per hour
	Tpd *int `json:"tpd,omitempty"` // Tokens per day
}

// ModelLimits represents per-model limits
type ModelLimits struct {
	Rpm  *int `json:"rpm,omitempty"`
	Rph  *int `json:"rph,omitempty"` // Requests per hour
	Rpd  *int `json:"rpd,omitempty"` // Requests per day
	Tpm  *int `json:"tpm,omitempty"`
	Tph  *int `json:"tph,omitempty"`  // Tokens per hour
	Tpd  *int `json:"tpd,omitempty"`  // Tokens per day
	Tpmu *int `json:"tpmu,omitempty"` // Tokens per month
}

// Certification represents a model certification for strict JSON
type Certification struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	StrictSchema bool   `json:"strictSchema"`
}

// AppConfig represents the application configuration
type AppConfig struct {
	Providers      []ProviderConfig `json:"providers"`
	Certifications []Certification  `json:"certifications"`
}

// LogicalModelConfig represents a logical model configuration
type LogicalModelConfig struct {
	ID                string                  `json:"id"`
	TaskType          string                  `json:"taskType"` // chat, analysis, json_extraction, code, tool_orchestration
	Candidates        []LogicalModelCandidate `json:"candidates"`
	SLO               *LogicalModelSLO        `json:"slo,omitempty"`
	RequireStrictJSON *bool                   `json:"requireStrictJson,omitempty"`
	RequireTools      *bool                   `json:"requireTools,omitempty"`
}

// LogicalModelCandidate represents a candidate model for a logical model
type LogicalModelCandidate struct {
	Provider string  `json:"provider"`         // Provider ID
	Model    string  `json:"model"`            // Provider-native model ID
	Weight   float64 `json:"weight,omitempty"` // Soft preference weight (0.0-1.0)
}

// LogicalModelSLO represents SLO for a logical model
type LogicalModelSLO struct {
	MaxLatencyMs *int `json:"maxLatencyMs,omitempty"`
	MaxAttempts  *int `json:"maxAttempts,omitempty"`
}

// DerivedRequirements represents parsed requirements from a request
type DerivedRequirements struct {
	Output    string `json:"output"`    // text, json_schema_strict
	Streaming string `json:"streaming"` // required, preferred, forbidden
	Tools     string `json:"tools"`     // required, allowed, forbidden
}

// RoutingCandidate represents a candidate for routing
type RoutingCandidate struct {
	Provider                   ProviderConfig     `json:"provider"`
	Model                      string             `json:"model"`
	IsCertifiedForStrictSchema bool               `json:"isCertifiedForStrictSchema"`
	Score                      float64            `json:"score"`
	ScoreBreakdown             map[string]float64 `json:"scoreBreakdown,omitempty"`
}

// RoutingAttempt represents a single execution attempt
type RoutingAttempt struct {
	ProviderID   string       `json:"providerId"`
	Model        string       `json:"model"`
	BaseURL      string       `json:"baseUrl"`
	APIKey       string       `json:"apiKey"`
	Score        float64      `json:"score"`
	TimeoutMs    int          `json:"timeoutMs"`
	ProviderType string       `json:"providerType"`
	Auth         ProviderAuth `json:"auth"`
}

// RoutingPlan represents the compiled routing plan
type RoutingPlan struct {
	Attempts       []RoutingAttempt `json:"attempts"`
	MaxAttempts    int              `json:"maxAttempts"`
	HardTimeoutMs  *int             `json:"hardTimeoutMs,omitempty"`
	RetryOn429     bool             `json:"retryOn429"`
	RetryOnTimeout bool             `json:"retryOnTimeout"`
	RetryOn5xx     bool             `json:"retryOn5xx"`
}

// ExecutionResult represents the result of executing a plan
type ExecutionResult struct {
	Response   ChatCompletionResponse `json:"response"`
	Attempts   int                    `json:"attempts"`
	ProviderID string                 `json:"providerId"`
	Model      string                 `json:"model"`
	LatencyMs  int64                  `json:"latencyMs"`
}

// GatewayError represents an error response from the gateway
type GatewayError struct {
	Type      string                 `json:"type"`
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	RequestID string                 `json:"request_id,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// Error implements the error interface
func (e *GatewayError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Type, e.Code, e.Message)
}

// RateLimitHeaders represents rate limit information from provider responses
type RateLimitHeaders struct {
	RetryAfter        *int   `json:"retryAfter,omitempty"`
	LimitRequests     *int   `json:"limitRequests,omitempty"`
	RemainingRequests *int   `json:"remainingRequests,omitempty"`
	ResetRequests     string `json:"resetRequests,omitempty"`
	LimitTokens       *int   `json:"limitTokens,omitempty"`
	RemainingTokens   *int   `json:"remainingTokens,omitempty"`
	ResetTokens       string `json:"resetTokens,omitempty"`
}
