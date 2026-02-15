package types

import "encoding/json"

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
