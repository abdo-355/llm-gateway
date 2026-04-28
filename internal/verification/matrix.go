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
			Name:   "field_acceptance",
			Fields: []string{"temperature", "top_p", "max_completion_tokens", "presence_penalty", "frequency_penalty", "stop", "seed", "n", "user", "metadata"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:               combo.Model,
					Messages:            basicMessages("Reply with OK only."),
					Temperature:         floatPtr(0),
					TopP:                floatPtr(1),
					MaxCompletionTokens: probeTokenPtr(cfg, 8),
					PresencePenalty:     floatPtr(0),
					FrequencyPenalty:    floatPtr(0),
					Stop:                []string{"__probe_stop__"},
					Seed:                intPtr(42),
					N:                   intPtr(1),
					User:                "verify-upstream",
					Metadata:            map[string]string{"probe": "field_acceptance", "provider": combo.Provider.ID},
				}
				return r.runJSONProbe(combo, "field_acceptance", []string{"temperature", "top_p", "max_completion_tokens", "presence_penalty", "frequency_penalty", "stop", "seed", "n", "user", "metadata"}, req, validateNonEmptyChatMessage)
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
					ResponseFormat:      &types.ResponseFormat{Type: "json_object"},
					MaxCompletionTokens: probeTokenPtr(cfg, 12),
				}
				return r.runJSONProbe(combo, "json_object", []string{"response_format.type=json_object"}, req, validateJSONObjectChat)
			},
		},
		{
			Name:   "json_schema_strict",
			Fields: []string{"response_format.type=json_schema", "response_format.json_schema.strict"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:               combo.Model,
					Messages:            basicMessages("Return JSON only with ok=true."),
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
			Fields: []string{"tools", "tool_choice", "parallel_tool_calls"},
			Run: func(r *Runner, combo Combo) ProbeResult {
				req := types.ChatCompletionRequest{
					Model:               combo.Model,
					Messages:            basicMessages("Call the get_status tool and nothing else."),
					Tools:               probeTools(),
					ToolChoice:          "required",
					ParallelToolCalls:   boolPtr(false),
					MaxCompletionTokens: probeTokenPtr(cfg, 12),
				}
				return r.runJSONProbe(combo, "tools", []string{"tools", "tool_choice", "parallel_tool_calls"}, req, validateToolCallChat)
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

func supportsJSONOutput(combo Combo) bool {
	switch combo.Provider.Capabilities.StructuredOutputs {
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
	return combo.Provider.Capabilities.StructuredOutputs == "json_schema_strict"
}

func basicMessages(prompt string) []types.OpenAIMessage {
	return []types.OpenAIMessage{
		{Role: "system", Content: "You are a verification probe. Keep replies minimal and text-only."},
		{Role: "user", Content: prompt},
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

func probeTools() []types.OpenAITool {
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
			Strict:      boolPtr(true),
		},
	}}
}

func validateNonEmptyChatMessage(resp *types.ChatCompletionResponse) error {
	if resp == nil || len(resp.Choices) == 0 {
		return fmt.Errorf("no chat choices returned")
	}
	choice := resp.Choices[0]
	if choice.Message.Content == nil || strings.TrimSpace(*choice.Message.Content) == "" {
		return fmt.Errorf("assistant message content was empty")
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
	var object map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &object); err != nil {
		return fmt.Errorf("response was not valid JSON object: %w", err)
	}
	return nil
}

func validateStrictJSON(payload string) error {
	var object map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &object); err != nil {
		return fmt.Errorf("response did not match strict JSON schema: %w", err)
	}
	value, ok := object["ok"]
	if !ok {
		return fmt.Errorf("response did not match strict JSON schema: missing ok field")
	}
	if _, ok := value.(bool); !ok {
		return fmt.Errorf("response did not match strict JSON schema: ok field was not boolean")
	}
	return nil
}

func boolPtr(v bool) *bool        { return &v }
func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
