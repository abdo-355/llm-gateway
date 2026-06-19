package verification

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/abdo-355/llm-gateway/internal/config"
	"github.com/abdo-355/llm-gateway/internal/types"
)

func EnumerateCombos(cfg Config) []Combo {
	appConfig := config.LoadConfig()
	strictJSONCertified := make(map[string]bool, len(appConfig.Certifications))
	for _, certification := range appConfig.Certifications {
		if certification.StrictSchema {
			strictJSONCertified[certification.Provider+"/"+certification.Model] = true
		}
	}

	combos := make([]Combo, 0)
	for _, provider := range appConfig.Providers {
		if cfg.Provider != "" && provider.ID != cfg.Provider {
			continue
		}

		for _, model := range provider.Models.List {
			if cfg.Model != "" && model != cfg.Model {
				continue
			}

			combos = append(combos, Combo{
				Provider:            provider,
				Model:               model,
				Limits:              provider.Models.Limits[model],
				Endpoint:            resolveEndpoint(provider),
				StrictJSONCertified: strictJSONCertified[provider.ID+"/"+model],
			})
		}
	}

	sort.Slice(combos, func(i, j int) bool {
		if combos[i].Provider.ID == combos[j].Provider.ID {
			return combos[i].Model < combos[j].Model
		}
		return combos[i].Provider.ID < combos[j].Provider.ID
	})

	return combos
}

func BuildProbes(cfg Config) []Probe {
	return []Probe{
		{
			Name:   "basic_text",
			Fields: []string{"messages", "max_tokens"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:     combo.Model,
					Messages:  basicMessages("Reply with OK only."),
					MaxTokens: probeTokenPtr(cfg, 8),
				}
				return r.runJSONProbe(combo, "basic_text", []string{"messages", "max_tokens"}, req, validateNonEmptyChatMessage)
			},
		},
		{
			Name:   "max_tokens",
			Fields: []string{"max_tokens"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:     combo.Model,
					Messages:  basicMessages("Reply with OK only."),
					MaxTokens: probeTokenPtr(cfg, 8),
				}
				return r.runJSONProbe(combo, "max_tokens", []string{"max_tokens"}, req, validateNonEmptyChatMessage)
			},
		},
		{
			Name:   "max_completion_tokens",
			Fields: []string{"max_completion_tokens"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:               combo.Model,
					Messages:            basicMessages("Reply with OK only."),
					MaxCompletionTokens: probeTokenPtr(cfg, 8),
				}
				return r.runJSONProbe(combo, "max_completion_tokens", []string{"max_completion_tokens"}, req, validateNonEmptyChatMessage)
			},
		},
		{
			Name:   "metadata",
			Fields: []string{"metadata"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:     combo.Model,
					Messages:  basicMessages("Reply with OK only."),
					MaxTokens: probeTokenPtr(cfg, 8),
					Metadata:  map[string]string{"probe": "metadata", "provider": combo.Provider.ID},
				}
				return r.runJSONProbe(combo, "metadata", []string{"metadata"}, req, validateNonEmptyChatMessage)
			},
		},
		{
			Name:   "seed",
			Fields: []string{"seed"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:     combo.Model,
					Messages:  basicMessages("Reply with OK only."),
					MaxTokens: probeTokenPtr(cfg, 8),
					Seed:      intPtr(42),
				}
				return r.runJSONProbe(combo, "seed", []string{"seed"}, req, validateNonEmptyChatMessage)
			},
		},
		{
			Name:   "user",
			Fields: []string{"user"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:     combo.Model,
					Messages:  basicMessages("Reply with OK only."),
					MaxTokens: probeTokenPtr(cfg, 8),
					User:      "verify-upstream",
				}
				return r.runJSONProbe(combo, "user", []string{"user"}, req, validateNonEmptyChatMessage)
			},
		},
		{
			Name:   "frequency_penalty",
			Fields: []string{"frequency_penalty"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:            combo.Model,
					Messages:         basicMessages("Reply with OK only."),
					MaxTokens:        probeTokenPtr(cfg, 8),
					FrequencyPenalty: floatPtr(0),
				}
				return r.runJSONProbe(combo, "frequency_penalty", []string{"frequency_penalty"}, req, validateNonEmptyChatMessage)
			},
		},
		{
			Name:   "presence_penalty",
			Fields: []string{"presence_penalty"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:           combo.Model,
					Messages:        basicMessages("Reply with OK only."),
					MaxTokens:       probeTokenPtr(cfg, 8),
					PresencePenalty: floatPtr(0),
				}
				return r.runJSONProbe(combo, "presence_penalty", []string{"presence_penalty"}, req, validateNonEmptyChatMessage)
			},
		},
		{
			Name:   "stream",
			Fields: []string{"stream", "stream_options.include_usage"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:    combo.Model,
					Messages: basicMessages("Reply with OK only."),
					Stream:   boolPtr(true),
					StreamOptions: &types.StreamOptions{
						IncludeUsage: boolPtr(true),
					},
					MaxCompletionTokens: probeTokenPtr(cfg, 8),
				}
				return r.runStreamProbe(combo, "stream", []string{"stream", "stream_options.include_usage"}, req)
			},
		},
		{
			Name:   "json_object",
			Fields: []string{"response_format.type=json_object"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:               combo.Model,
					Messages:            basicMessages("Return a JSON object with key ok set to true."),
					Stream:              boolPtr(false),
					ResponseFormat:      &types.ResponseFormat{Type: "json_object"},
					MaxCompletionTokens: probeTokenPtr(cfg, 12),
				}
				return r.runJSONProbe(combo, "json_object", []string{"response_format.type=json_object"}, req, validateJSONObjectChat)
			},
		},
		{
			Name:   "json_schema",
			Fields: []string{"response_format.type=json_schema"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:               combo.Model,
					Messages:            basicMessages("Return JSON only with ok=true."),
					Stream:              boolPtr(false),
					ResponseFormat:      nonStrictJSONSchemaFormat(),
					MaxCompletionTokens: probeTokenPtr(cfg, 12),
				}
				return r.runJSONProbe(combo, "json_schema", []string{"response_format.type=json_schema"}, req, validateStrictJSONChat)
			},
		},
		{
			Name:   "json_schema_strict",
			Fields: []string{"response_format.type=json_schema", "response_format.json_schema.strict"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:               combo.Model,
					Messages:            basicMessages("Return JSON only with ok=true."),
					Stream:              boolPtr(false),
					ResponseFormat:      strictJSONSchemaFormat(),
					MaxCompletionTokens: probeTokenPtr(cfg, 12),
				}
				return r.runJSONProbe(combo, "json_schema_strict", []string{"response_format.type=json_schema", "response_format.json_schema.strict"}, req, validateStrictJSONChat)
			},
		},
		{
			Name:   "logprobs",
			Fields: []string{"logprobs", "top_logprobs"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:       combo.Model,
					Messages:    basicMessages("Say hello."),
					Logprobs:    boolPtr(true),
					TopLogprobs: intPtr(5),
					MaxTokens:   probeTokenPtr(cfg, 8),
				}
				return r.runJSONProbe(combo, "logprobs", []string{"logprobs", "top_logprobs"}, req, validateLogprobs)
			},
		},
		{
			Name:   "multiple_choices",
			Fields: []string{"n"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:     combo.Model,
					Messages:  basicMessages("Say hello."),
					N:         intPtr(2),
					MaxTokens: probeTokenPtr(cfg, 8),
				}
				return r.runJSONProbe(combo, "multiple_choices", []string{"n"}, req, validateMultipleChoices)
			},
		},
		{
			Name:   "tools",
			Fields: []string{"tools", "tool_choice"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:      combo.Model,
					Messages:   basicMessages("Call the get_status tool and nothing else."),
					Tools:      probeTools(false),
					ToolChoice: "required",
					MaxTokens:  probeTokenPtr(cfg, 12),
				}
				return r.runJSONProbe(combo, "tools", []string{"tools", "tool_choice"}, req, validateToolCallChat)
			},
		},
		{
			Name:   "tool_schema",
			Fields: []string{"tools.function.parameters", "tools.function.strict", "parallel_tool_calls"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:               combo.Model,
					Messages:            basicMessages("Call the get_status tool and nothing else."),
					Tools:               probeTools(true),
					ToolChoice:          "required",
					ParallelToolCalls:   boolPtr(false),
					MaxCompletionTokens: probeTokenPtr(cfg, 12),
				}
				return r.runJSONProbe(combo, "tool_schema", []string{"tools.function.parameters", "tools.function.strict", "parallel_tool_calls"}, req, validateToolCallChat)
			},
		},
	}
}

func probeTokenPtr(cfg Config, fallback int) *int {
	if cfg.ProbeMaxTokens > 0 {
		return intPtr(cfg.ProbeMaxTokens)
	}
	return intPtr(fallback)
}

func resolveEndpoint(provider types.ProviderConfig) string {
	baseURL := provider.BaseURL
	if provider.ProviderType == "ollama" {
		return strings.TrimRight(baseURL, "/") + "/api/chat"
	}
	if provider.ProviderType == "cloudflare_workers_ai" {
		accountID := strings.TrimSpace(os.Getenv("CLOUDFLARE_ACCOUNT_ID"))
		if accountID == "" {
			accountID = "{account_id}"
		}
		return strings.TrimRight(baseURL, "/") + "/accounts/" + accountID + "/ai/run/{model}"
	}
	return strings.TrimRight(baseURL, "/") + "/chat/completions"
}

func resolveCapabilities(combo Combo) types.ProviderCapabilities {
	resolved := combo.Provider.Capabilities
	overrides, ok := combo.Provider.Models.Capabilities[combo.Model]
	if !ok {
		return resolved
	}
	if overrides.Streaming != nil {
		resolved.Streaming = *overrides.Streaming
	}
	if overrides.Tools != nil {
		resolved.Tools = *overrides.Tools
	}
	if overrides.StructuredOutputs != nil {
		resolved.StructuredOutputs = *overrides.StructuredOutputs
	}
	if overrides.Logprobs != nil {
		resolved.Logprobs = *overrides.Logprobs
	}
	if overrides.MultipleChoices != nil {
		resolved.MultipleChoices = *overrides.MultipleChoices
	}
	return resolved
}

func supportsJSONOutput(combo Combo) bool {
	caps := resolveCapabilities(combo)
	switch caps.StructuredOutputs {
	case "none", "unknown":
		return false
	default:
		return true
	}
}

func supportsStrictJSON(combo Combo) bool {
	if combo.StrictJSONCertified {
		return true
	}
	switch resolveCapabilities(combo).StructuredOutputs {
	case "json_object", "json_schema", "json_schema_strict", "model_dependent":
		return true
	default:
		return false
	}
}

func supportsStreaming(combo Combo) bool {
	return resolveCapabilities(combo).Streaming
}

func supportsTools(combo Combo) bool {
	return resolveCapabilities(combo).Tools
}

func supportsLogprobs(combo Combo) bool {
	return resolveCapabilities(combo).Logprobs
}

func supportsMultipleChoices(combo Combo) bool {
	return resolveCapabilities(combo).MultipleChoices
}

func configuredCapability(combo Combo, probe string) string {
	caps := resolveCapabilities(combo)
	switch probe {
	case "stream":
		return boolCapability(caps.Streaming)
	case "json_object", "json_schema", "json_schema_strict":
		if probe == "json_schema_strict" && combo.StrictJSONCertified {
			return caps.StructuredOutputs + "+certified"
		}
		return caps.StructuredOutputs
	case "logprobs":
		return boolCapability(caps.Logprobs)
	case "metadata":
		return boolCapability(caps.Metadata)
	case "seed":
		return boolCapability(caps.Seed)
	case "user":
		return boolCapability(caps.User)
	case "frequency_penalty":
		return boolCapability(caps.FrequencyPenalty)
	case "presence_penalty":
		return boolCapability(caps.PresencePenalty)
	case "max_tokens":
		return boolCapability(caps.MaxTokens)
	case "max_completion_tokens":
		return boolCapability(caps.MaxCompletionTokens)
	case "multiple_choices":
		return boolCapability(caps.MultipleChoices)
	case "tools":
		return boolCapability(caps.Tools)
	case "tool_schema":
		return caps.ToolSchema
	default:
		return ""
	}
}

func boolCapability(supported bool) string {
	if supported {
		return "true"
	}
	return "false"
}

func basicMessages(prompt string) []types.OpenAIMessage {
	return []types.OpenAIMessage{
		{Role: "system", Content: "You are a verification probe. Keep replies minimal and text-only."},
		{Role: "user", Content: prompt},
	}
}

func nonStrictJSONSchemaFormat() *types.ResponseFormat {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok": map[string]any{"type": "boolean"},
		},
		"required":             []string{"ok"},
		"additionalProperties": true,
	})

	return &types.ResponseFormat{
		Type: "json_schema",
		JSONSchema: &types.JSONSchema{
			Name:   "probe_schema",
			Schema: schema,
		},
	}
}

func strictJSONSchemaFormat() *types.ResponseFormat {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok": map[string]any{"type": "boolean"},
		},
		"required":             []string{"ok"},
		"additionalProperties": false,
	})

	return &types.ResponseFormat{
		Type: "json_schema",
		JSONSchema: &types.JSONSchema{
			Name:   "probe_schema",
			Schema: schema,
			Strict: boolPtr(true),
		},
	}
}

func probeTools(strict bool) []types.OpenAITool {
	params, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"service": map[string]any{"type": "string"},
		},
		"required":             []string{"service"},
		"additionalProperties": false,
	})

	return []types.OpenAITool{{
		Type: "function",
		Function: types.Function{
			Name:        "get_status",
			Description: "Returns the status of a service.",
			Parameters:  params,
			Strict:      optionalBoolPtr(strict),
		},
	}}
}

func optionalBoolPtr(v bool) *bool {
	if !v {
		return nil
	}
	return &v
}

func validateNonEmptyChatMessage(resp *types.ChatCompletionResponse) error {
	if resp == nil || len(resp.Choices) == 0 {
		return fmt.Errorf("no chat choices returned")
	}
	choice := resp.Choices[0]
	if choice.Message.Content == nil || strings.TrimSpace(*choice.Message.Content) == "" {
		finish := choice.FinishReason
		toolCalls := len(choice.Message.ToolCalls)
		refusal := ""
		if choice.Message.Refusal != nil {
			refusal = *choice.Message.Refusal
		}
		return fmt.Errorf("assistant message content was empty (finish=%s, tool_calls=%d, refusal=%q)", finish, toolCalls, refusal)
	}
	return nil
}

func validateJSONObjectChat(resp *types.ChatCompletionResponse) error {
	if err := validateNonEmptyChatMessage(resp); err != nil {
		return err
	}
	return validateJSONObject(*resp.Choices[0].Message.Content)
}

func validateStrictJSONChat(resp *types.ChatCompletionResponse) error {
	if err := validateJSONObjectChat(resp); err != nil {
		return err
	}
	return validateStrictJSON(*resp.Choices[0].Message.Content)
}

func validateToolCallChat(resp *types.ChatCompletionResponse) error {
	if resp == nil || len(resp.Choices) == 0 {
		return fmt.Errorf("no chat choices returned")
	}
	if len(resp.Choices[0].Message.ToolCalls) == 0 {
		return fmt.Errorf("no tool call returned")
	}
	return nil
}

func validateLogprobs(resp *types.ChatCompletionResponse) error {
	if err := validateNonEmptyChatMessage(resp); err != nil {
		return err
	}
	if resp.Choices[0].Logprobs == nil {
		return fmt.Errorf("logprobs field missing in response")
	}
	if len(resp.Choices[0].Logprobs.Content) == 0 {
		return fmt.Errorf("logprobs content was empty")
	}
	return nil
}

func validateMultipleChoices(resp *types.ChatCompletionResponse) error {
	if resp == nil || len(resp.Choices) == 0 {
		return fmt.Errorf("no chat choices returned")
	}
	if len(resp.Choices) < 2 {
		return fmt.Errorf("expected at least 2 choices, got %d", len(resp.Choices))
	}
	for i, choice := range resp.Choices {
		if choice.Message.Content == nil || strings.TrimSpace(*choice.Message.Content) == "" {
			return fmt.Errorf("choice %d assistant message content was empty", i)
		}
	}
	return nil
}

func validateJSONObject(payload string) error {
	trimmed := strings.TrimSpace(payload)
	var object map[string]any
	if err := json.Unmarshal([]byte(trimmed), &object); err != nil {
		preview := trimmed
		if len(preview) > 250 {
			preview = preview[:250] + "..."
		}
		return fmt.Errorf("response was not valid JSON object: %w [content: %s]", err, preview)
	}
	return nil
}

func validateStrictJSON(payload string) error {
	trimmed := strings.TrimSpace(payload)
	var object map[string]any
	if err := json.Unmarshal([]byte(trimmed), &object); err != nil {
		preview := trimmed
		if len(preview) > 250 {
			preview = preview[:250] + "..."
		}
		return fmt.Errorf("response did not match strict JSON schema: %w [content: %s]", err, preview)
	}
	value, ok := object["ok"]
	if !ok {
		return fmt.Errorf("response did not match strict JSON schema: missing ok field [content: %s]", previewContent(trimmed))
	}
	if _, ok := value.(bool); !ok {
		return fmt.Errorf("response did not match strict JSON schema: ok field was not boolean [content: %s]", previewContent(trimmed))
	}
	return nil
}

func previewContent(s string) string {
	if len(s) > 250 {
		return s[:250] + "..."
	}
	return s
}

func boolPtr(v bool) *bool        { return &v }
func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
