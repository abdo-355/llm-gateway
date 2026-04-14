package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// OpenAIMessage represents a chat message in OpenAI format
type OpenAIMessage struct {
	Role       string     `json:"role"`                   // system, user, assistant, tool
	Content    any        `json:"content,omitempty"`      // string or array of content parts
	Name       string     `json:"name,omitempty"`         // optional name
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // tool calls made by assistant
	ToolCallID string     `json:"tool_call_id,omitempty"` // ID of tool call (for tool role)
}

type ContentPart struct {
	Type     string `json:"type"`           // text or image_url
	Text     string `json:"text,omitempty"` // for text type
	ImageURL *struct {
		URL    string `json:"url"`
		Detail string `json:"detail,omitempty"` // auto, low, high
	} `json:"image_url,omitempty"` // for image_url type
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAITool represents a tool definition
type OpenAITool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"` // JSON schema
	// bool here is a pointer to differentiate between an empty value(nil) and a zero value(false)
	Strict *bool `json:"strict,omitempty"`
}

type ResponseFormat struct {
	Type       string      `json:"type"` // text, json_object, json_schema
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

type JSONSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      *bool           `json:"strict,omitempty"`
}

type ChatCompletionRequest struct {
	Model               string            `json:"model"`
	Messages            []OpenAIMessage   `json:"messages"`
	Temperature         *float64          `json:"temperature,omitempty"`
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
	RandomSeed          *int              `json:"random_seed,omitempty"`
	Stop                any               `json:"stop,omitempty"` // string or []string
	Stream              *bool             `json:"stream,omitempty"`
	StreamOptions       *StreamOptions    `json:"stream_options,omitempty"`
	TopP                *float64          `json:"top_p,omitempty"`
	Tools               []OpenAITool      `json:"tools,omitempty"`
	ToolChoice          any               `json:"tool_choice,omitempty"` // none, auto, required, or object
	ParallelToolCalls   *bool             `json:"parallel_tool_calls,omitempty"`
	User                string            `json:"user,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	Router              *RouterHints      `json:"router,omitempty"` // Internal gateway routing hints
}

type StreamOptions struct {
	IncludeUsage *bool `json:"include_usage,omitempty"`
}

type ChatCompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"` // chat.completion
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

type Choice struct {
	Index        int             `json:"index"`
	Message      ResponseMessage `json:"message"`
	Logprobs     *Logprobs       `json:"logprobs,omitempty"`
	FinishReason string          `json:"finish_reason"` // stop, length, tool_calls, content_filter, function_call
}

type ResponseMessage struct {
	Role      string     `json:"role"`
	Content   *string    `json:"content"` // null if tool calls present
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Refusal   *string    `json:"refusal,omitempty"`
}

func (m *ResponseMessage) UnmarshalJSON(data []byte) error {
	type alias struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls []ToolCall      `json:"tool_calls,omitempty"`
		Refusal   *string         `json:"refusal,omitempty"`
	}

	var parsed alias
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}

	content, err := parseVisibleContent(parsed.Content)
	if err != nil {
		return err
	}

	m.Role = parsed.Role
	m.Content = content
	m.ToolCalls = parsed.ToolCalls
	m.Refusal = parsed.Refusal
	return nil
}

// Logprobs represents log probabilities
type Logprobs struct {
	Content []LogprobContent `json:"content,omitempty"`
	Refusal []LogprobContent `json:"refusal,omitempty"`
}

type LogprobContent struct {
	Token       string           `json:"token"`
	Logprob     float64          `json:"logprob"`
	Bytes       []int            `json:"bytes,omitempty"`
	TopLogprobs []TopLogprobItem `json:"top_logprobs"`
}

type TopLogprobItem struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
	Bytes   []int   `json:"bytes,omitempty"`
}

type Usage struct {
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

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

func (m *DeltaMessage) UnmarshalJSON(data []byte) error {
	type alias struct {
		Role      string          `json:"role,omitempty"`
		Content   json.RawMessage `json:"content,omitempty"`
		ToolCalls []DeltaToolCall `json:"tool_calls,omitempty"`
		Refusal   *string         `json:"refusal,omitempty"`
	}

	var parsed alias
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}

	content, err := parseVisibleContent(parsed.Content)
	if err != nil {
		return err
	}

	m.Role = parsed.Role
	m.Content = content
	m.ToolCalls = parsed.ToolCalls
	m.Refusal = parsed.Refusal
	return nil
}

type DeltaToolCall struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"` // function
	Function *DeltaFunction `json:"function,omitempty"`
}

type DeltaFunction struct {
	Name      *string `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}

func parseVisibleContent(raw json.RawMessage) (*string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, err
	}

	text := collectVisibleText(value)
	if text == "" {
		return nil, nil
	}

	return &text, nil
}

func collectVisibleText(value any) string {
	parts := make([]string, 0)
	appendVisibleText(value, &parts)
	return strings.Join(parts, "")
}

func appendVisibleText(value any, parts *[]string) {
	switch typed := value.(type) {
	case string:
		if typed != "" {
			*parts = append(*parts, typed)
		}
	case []any:
		for _, item := range typed {
			appendVisibleText(item, parts)
		}
	case map[string]any:
		if thinking := typed["thinking"]; thinking != nil {
			return
		}
		if reasoning := typed["reasoning"]; reasoning != nil {
			return
		}
		if partType, ok := typed["type"].(string); ok {
			switch partType {
			case "text", "output_text":
				if text, ok := typed["text"].(string); ok && text != "" {
					*parts = append(*parts, text)
				}
				return
			case "thinking", "reasoning":
				return
			}
		}
		if text, ok := typed["text"].(string); ok && text != "" {
			*parts = append(*parts, text)
			return
		}
		if content, ok := typed["content"]; ok {
			appendVisibleText(content, parts)
		}
	}
}

type RouterHints struct {
	Profile      *string              `json:"profile,omitempty"` // cheap_fast, reliable_structured, balanced
	Requirements *RouterRequirements  `json:"requirements,omitempty"`
	Budget       *BudgetConfig        `json:"budget,omitempty"`
	SLO          *SLOConfig           `json:"slo,omitempty"`
	Providers    *ProviderPreferences `json:"providers,omitempty"`
	Fallback     *FallbackConfig      `json:"fallback,omitempty"`
	Trace        *TraceConfig         `json:"trace,omitempty"`
}

type RouterRequirements struct {
	Output    *string `json:"output,omitempty"`    // text, json_schema_strict
	Streaming *string `json:"streaming,omitempty"` // required, preferred, forbidden
	Tools     *string `json:"tools,omitempty"`     // required, allowed, forbidden
}

type BudgetConfig struct {
	Mode string `json:"mode"` // free_only, allow_paid
}

type SLOConfig struct {
	MaxLatencyMs  *int `json:"max_latency_ms,omitempty"`
	HardTimeoutMs *int `json:"hard_timeout_ms,omitempty"`
}

type ProviderPreferences struct {
	Allow  []string `json:"allow,omitempty"`
	Deny   []string `json:"deny,omitempty"`
	Prefer []string `json:"prefer,omitempty"`
}

type FallbackConfig struct {
	MaxAttempts *int  `json:"max_attempts,omitempty"`
	On429       *bool `json:"on_429,omitempty"`
	OnTimeout   *bool `json:"on_timeout,omitempty"`
	On5xx       *bool `json:"on_5xx,omitempty"`
}

type TraceConfig struct {
	RequestID *string  `json:"request_id,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

type ProviderConfig struct {
	ID           string               `json:"id"`
	BaseURL      string               `json:"baseUrl"`
	Auth         ProviderAuth         `json:"auth"`
	Models       ProviderModels       `json:"models"`
	Capabilities ProviderCapabilities `json:"capabilities"`
	Limits       ProviderLimits       `json:"limits"`
	ProviderType string               `json:"providerType,omitempty"` // openai, vertex (defaults to openai)
}

type ProviderAuth struct {
	Type       string `json:"type"`                 // none, bearer, header
	Env        string `json:"env"`                  // environment variable name
	HeaderName string `json:"headerName,omitempty"` // for header auth type
}

type ProviderModels struct {
	Mode         string                       `json:"mode"`                   // allowlist, denylist
	List         []string                     `json:"list"`                   // model names
	Limits       map[string]ModelLimits       `json:"limits,omitempty"`       // per-model limits
	Capabilities map[string]ModelCapabilities `json:"capabilities,omitempty"` // per-model capability overrides
}

type ProviderCapabilities struct {
	Streaming           bool   `json:"streaming"`
	Tools               bool   `json:"tools"`
	StructuredOutputs   string `json:"structuredOutputs"` // none, json_object, json_schema_strict, model_dependent, unknown
	Logprobs            bool   `json:"logprobs,omitempty"`
	Metadata            bool   `json:"metadata,omitempty"`
	Seed                bool   `json:"seed,omitempty"`
	User                bool   `json:"user,omitempty"`
	FrequencyPenalty    bool   `json:"frequencyPenalty,omitempty"`
	PresencePenalty     bool   `json:"presencePenalty,omitempty"`
	MaxTokens           bool   `json:"maxTokens,omitempty"`
	MaxCompletionTokens bool   `json:"maxCompletionTokens,omitempty"`
	MultipleChoices     bool   `json:"multipleChoices,omitempty"`
	ToolSchema          string `json:"toolSchema,omitempty"` // json_schema, openapi
}

type ModelCapabilities struct {
	Streaming           *bool   `json:"streaming,omitempty"`
	Tools               *bool   `json:"tools,omitempty"`
	StructuredOutputs   *string `json:"structuredOutputs,omitempty"`
	Logprobs            *bool   `json:"logprobs,omitempty"`
	Metadata            *bool   `json:"metadata,omitempty"`
	Seed                *bool   `json:"seed,omitempty"`
	User                *bool   `json:"user,omitempty"`
	FrequencyPenalty    *bool   `json:"frequencyPenalty,omitempty"`
	PresencePenalty     *bool   `json:"presencePenalty,omitempty"`
	MaxTokens           *bool   `json:"maxTokens,omitempty"`
	MaxCompletionTokens *bool   `json:"maxCompletionTokens,omitempty"`
	MultipleChoices     *bool   `json:"multipleChoices,omitempty"`
	ToolSchema          *string `json:"toolSchema,omitempty"`
}

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

type ModelLimits struct {
	Rpm  *int `json:"rpm,omitempty"`
	Rph  *int `json:"rph,omitempty"` // Requests per hour
	Rpd  *int `json:"rpd,omitempty"` // Requests per day
	Tpm  *int `json:"tpm,omitempty"`
	Tph  *int `json:"tph,omitempty"`  // Tokens per hour
	Tpd  *int `json:"tpd,omitempty"`  // Tokens per day
	Tpmu *int `json:"tpmu,omitempty"` // Tokens per month
}

type Certification struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	StrictSchema bool   `json:"strictSchema"`
}

type AppConfig struct {
	Providers      []ProviderConfig `json:"providers"`
	Certifications []Certification  `json:"certifications"`
}

type LogicalModelConfig struct {
	ID                string                  `json:"id"`
	TaskType          string                  `json:"taskType"` // chat, analysis, json_extraction, code, tool_orchestration
	Candidates        []LogicalModelCandidate `json:"candidates"`
	SLO               *LogicalModelSLO        `json:"slo,omitempty"`
	RequireStrictJSON *bool                   `json:"requireStrictJson,omitempty"`
	RequireTools      *bool                   `json:"requireTools,omitempty"`
}

type LogicalModelCandidate struct {
	Provider string  `json:"provider"`         // Provider ID
	Model    string  `json:"model"`            // Provider-native model ID
	Weight   float64 `json:"weight,omitempty"` // Soft preference weight (0.0-1.0)
}

type LogicalModelSLO struct {
	MaxLatencyMs *int `json:"maxLatencyMs,omitempty"`
	MaxAttempts  *int `json:"maxAttempts,omitempty"`
}

type DerivedRequirements struct {
	Output    string `json:"output"`    // text, json_schema_strict
	Streaming string `json:"streaming"` // required, preferred, forbidden
	Tools     string `json:"tools"`     // required, allowed, forbidden
}

type RoutingCandidate struct {
	Provider                   ProviderConfig     `json:"provider"`
	Model                      string             `json:"model"`
	IsCertifiedForStrictSchema bool               `json:"isCertifiedForStrictSchema"`
	Score                      float64            `json:"score"`
	ScoreBreakdown             map[string]float64 `json:"scoreBreakdown,omitempty"`
}

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

type RoutingPlan struct {
	Attempts       []RoutingAttempt `json:"attempts"`
	MaxAttempts    int              `json:"maxAttempts"`
	HardTimeoutMs  *int             `json:"hardTimeoutMs,omitempty"`
	RetryOn429     bool             `json:"retryOn429"`
	RetryOnTimeout bool             `json:"retryOnTimeout"`
	RetryOn5xx     bool             `json:"retryOn5xx"`
}

type ExecutionResult struct {
	Response   ChatCompletionResponse `json:"response"`
	Attempts   int                    `json:"attempts"`
	ProviderID string                 `json:"providerId"`
	Model      string                 `json:"model"`
	LatencyMs  int64                  `json:"latencyMs"`
}

type GatewayError struct {
	Type      string         `json:"type"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

func (e *GatewayError) Error() string {
	return fmt.Sprintf("[%s] %s: %s", e.Type, e.Code, e.Message)
}

type RateLimitHeaders struct {
	RetryAfter        *int   `json:"retryAfter,omitempty"`
	LimitRequests     *int   `json:"limitRequests,omitempty"`
	RemainingRequests *int   `json:"remainingRequests,omitempty"`
	ResetRequests     string `json:"resetRequests,omitempty"`
	LimitTokens       *int   `json:"limitTokens,omitempty"`
	RemainingTokens   *int   `json:"remainingTokens,omitempty"`
	ResetTokens       string `json:"resetTokens,omitempty"`
}

type StreamResult struct {
	Chunks <-chan *SSEChunk
	Err    <-chan *GatewayError
}

type ResponseRequest struct {
	Model               string            `json:"model"`
	Input               any               `json:"input"`
	Instructions        *string           `json:"instructions,omitempty"`
	Tools               []OpenAITool      `json:"tools,omitempty"`
	ToolChoice          any               `json:"tool_choice,omitempty"`
	Text                *TextConfig       `json:"text,omitempty"`
	Store               *bool             `json:"store,omitempty"`
	PreviousResponseID  *string           `json:"previous_response_id,omitempty"`
	Include             []string          `json:"include,omitempty"`
	Temperature         *float64          `json:"temperature,omitempty"`
	MaxTokens           *int              `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
	TopP                *float64          `json:"top_p,omitempty"`
	Stream              *bool             `json:"stream,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	Router              *RouterHints      `json:"router,omitempty"`
}

type TextConfig struct {
	Format *ResponseFormat `json:"format,omitempty"`
}

type ResponseInputItem struct {
	Type string `json:"type"`

	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"`

	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	Output string `json:"output,omitempty"`
}

func (i *ResponseInputItem) IsMessage() bool {
	return i.Type == "message"
}

func (i *ResponseInputItem) IsFunctionCall() bool {
	return i.Type == "function_call"
}

func (i *ResponseInputItem) IsFunctionCallOutput() bool {
	return i.Type == "function_call_output"
}

type Response struct {
	ID         string         `json:"id"`
	Object     string         `json:"object"`
	CreatedAt  int64          `json:"created_at"`
	Model      string         `json:"model"`
	Output     []ResponseItem `json:"output"`
	OutputText string         `json:"output_text"`
	Usage      *Usage         `json:"usage,omitempty"`
	Status     string         `json:"status"`
	Error      *ResponseError `json:"error,omitempty"`
}

type ResponseItem struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status,omitempty"`

	Role    string          `json:"role,omitempty"`
	Content []ContentOutput `json:"content,omitempty"`

	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	Output any `json:"output,omitempty"`
}

type ContentOutput struct {
	Type        string          `json:"type"`
	Text        string          `json:"text,omitempty"`
	Annotations []any           `json:"annotations,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}

type ResponseError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ParseResponseInput(input any) ([]ResponseInputItem, error) {
	switch v := input.(type) {
	case string:
		return []ResponseInputItem{
			{
				Type:    "message",
				Role:    "user",
				Content: v,
			},
		}, nil
	case []any:
		items := make([]ResponseInputItem, 0, len(v))
		for _, item := range v {
			bytes, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			var inputItem ResponseInputItem
			if err := json.Unmarshal(bytes, &inputItem); err != nil {
				return nil, err
			}
			items = append(items, inputItem)
		}
		return items, nil
	case []ResponseInputItem:
		return v, nil
	default:
		return nil, &GatewayError{
			Type:    "validation_error",
			Code:    "INVALID_INPUT_TYPE",
			Message: "input must be a string or an array of items",
		}
	}
}
