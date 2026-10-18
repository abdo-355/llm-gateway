package config

import "github.com/abdo-355/llm-gateway/internal/types"

func GetProviders() []types.ProviderConfig {
	return []types.ProviderConfig{
		getGroqConfig(),
		getCloudflareConfig(),
		getOllamaConfig(),
		getZaiConfig(),
		getCohereConfig(),
		getGeminiConfig(),
		getNousConfig(),
		getOpenRouterConfig(),
		getOrcaConfig(),
	}
}

func getGroqConfig() types.ProviderConfig {
	rpm30 := 30
	rpd1000 := 1000
	tpm8000 := 8000
	tpd200000 := 200000

	return types.ProviderConfig{
		ID:      "groq",
		BaseURL: "https://api.groq.com/openai/v1",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "GROQ_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"openai/gpt-oss-120b",
				"openai/gpt-oss-20b",
				"qwen/qwen3.6-27b",
				"qwen/qwen3.8-27b",
			},
			Limits: map[string]types.ModelLimits{
				"openai/gpt-oss-120b": {Rpm: &rpm30, Rpd: &rpd1000, Tpm: &tpm8000, Tpd: &tpd200000},
				"openai/gpt-oss-20b":  {Rpm: &rpm30, Rpd: &rpd1000, Tpm: &tpm8000, Tpd: &tpd200000},
				"qwen/qwen3.6-27b":    {Rpm: &rpm30, Rpd: &rpd1000, Tpm: &tpm8000, Tpd: &tpd200000},
				"qwen/qwen3.8-27b":    {Rpm: &rpm30, Rpd: &rpd1000, Tpm: &tpm8000, Tpd: &tpd200000},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"openai/gpt-oss-120b": {Logprobs: boolPtr(false), Reasoning: boolPtr(true)},
				"openai/gpt-oss-20b":  {Logprobs: boolPtr(false), Reasoning: boolPtr(true)},
				"qwen/qwen3.6-27b":    {Logprobs: boolPtr(false)},
				"qwen/qwen3.8-27b":    {Logprobs: boolPtr(false)},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "json_schema_strict",
			Logprobs:            true,
			Metadata:            false,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    false,
			PresencePenalty:     false,
			MaxTokens:           true,
			MaxCompletionTokens: true,
			MultipleChoices:     false,
			ToolSchema:          "json_schema",
		},
		Limits: types.ProviderLimits{},
	}
}


func GetCertifications() []types.Certification {
	return []types.Certification{}
}

func LoadConfig() types.AppConfig {
	return types.AppConfig{
		Providers:      GetProviders(),
		Certifications: GetCertifications(),
	}
}


func getCloudflareConfig() types.ProviderConfig {
	rpm1 := 1
	rpm2 := 2
	rpm3 := 3
	rpm5 := 5
	rpm10 := 10
	conc1 := 1

	return types.ProviderConfig{
		ID:      "cloudflare",
		BaseURL: "https://api.cloudflare.com/client/v4",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "CLOUDFLARE_API_TOKEN",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"@cf/meta/llama-3.3-70b-instruct-fp8-fast",
				"@cf/qwen/qwen2.5-coder-32b-instruct",
				"@cf/qwen/qwen3.8-27b",
				"@cf/qwen/qwen3-30b-a3b-fp8",
				"@cf/google/gemma-4-26b-a4b-it",
				"@cf/openai/gpt-oss-120b",
				"@cf/openai/gpt-oss-20b",
				"@cf/mistralai/mistral-small-3.1-24b-instruct",
				"@cf/zai-org/glm-4.7-flash",
				"@cf/meta/llama-4-scout-17b-16e-instruct",
				"@cf/ibm-granite/granite-4.0-h-micro",
				"@cf/deepseek-ai/deepseek-r1-distill-qwen-32b",
				"@cf/moonshotai/kimi-k2.5",
				"@cf/meta/llama-3.2-3b-instruct",
				"@cf/meta/llama-3.2-1b-instruct",
				"@cf/google/gemma-3-12b-it",
				"@cf/nvidia/nemotron-3-120b-a12b",
			},
			Limits: map[string]types.ModelLimits{
				"@cf/meta/llama-3.3-70b-instruct-fp8-fast":     {Rpm: &rpm3},
				"@cf/qwen/qwen2.5-coder-32b-instruct":          {Rpm: &rpm3},
				"@cf/qwen/qwen3.8-27b":                         {Rpm: &rpm5},
				"@cf/qwen/qwen3-30b-a3b-fp8":                   {Rpm: &rpm10},
				"@cf/google/gemma-4-26b-a4b-it":                {Rpm: &rpm5},
				"@cf/openai/gpt-oss-120b":                      {Rpm: &rpm3},
				"@cf/openai/gpt-oss-20b":                       {Rpm: &rpm5},
				"@cf/mistralai/mistral-small-3.1-24b-instruct": {Rpm: &rpm5},
				"@cf/zai-org/glm-4.7-flash":                    {Rpm: &rpm10},
				"@cf/meta/llama-4-scout-17b-16e-instruct":      {Rpm: &rpm5},
				"@cf/ibm-granite/granite-4.0-h-micro":          {Rpm: &rpm10},
				"@cf/deepseek-ai/deepseek-r1-distill-qwen-32b": {Rpm: &rpm1},
				"@cf/moonshotai/kimi-k2.5":                     {Rpm: &rpm2},
				"@cf/meta/llama-3.2-3b-instruct":               {Rpm: &rpm10},
				"@cf/meta/llama-3.2-1b-instruct":               {Rpm: &rpm10},
				"@cf/google/gemma-3-12b-it":                    {Rpm: &rpm5},
				"@cf/nvidia/nemotron-3-120b-a12b":              {Rpm: &rpm3},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           false,
			Tools:               false,
			StructuredOutputs:   "none",
			Logprobs:            false,
			Metadata:            false,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    false,
			PresencePenalty:     false,
			MaxTokens:           true,
			MaxCompletionTokens: false,
			MultipleChoices:     false,
			ToolSchema:          "json_schema",
		},
		Limits: types.ProviderLimits{
			MaxConcurrent: &conc1,
		},
		ProviderType: "cloudflare_workers_ai",
	}
}


func getOllamaConfig() types.ProviderConfig {
	conc1 := 1

	return types.ProviderConfig{
		ID:      "ollama",
		BaseURL: "https://ollama.com",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "OLLAMA_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"gemma4:31b",
				"gpt-oss:120b",
				"gpt-oss:20b",
				"nemotron-3-nano:30b",
				"nemotron-3-super",
				"nemotron-3-ultra",
			},
			Limits: map[string]types.ModelLimits{},
			Capabilities: map[string]types.ModelCapabilities{
				"gpt-oss:120b":     {Reasoning: boolPtr(true)},
				"gpt-oss:20b":      {Reasoning: boolPtr(true)},
				"nemotron-3-super": {Reasoning: boolPtr(true)},
				"nemotron-3-ultra": {Reasoning: boolPtr(true)},
				"gemma4:31b":       {Tools: boolPtr(true)},
				"nemotron-3-nano:30b": {Tools: boolPtr(true)},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "none",
			Logprobs:            false,
			Metadata:            false,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    false,
			PresencePenalty:     false,
			MaxTokens:           true,
			MaxCompletionTokens: true,
			MultipleChoices:     false,
			ToolSchema:          "json_schema",
		},
		Limits: types.ProviderLimits{
			MaxConcurrent: &conc1,
		},
		ProviderType: "ollama",
	}
}


func getZaiConfig() types.ProviderConfig {
	conc1 := 1
	conc2 := 2

	return types.ProviderConfig{
		ID:      "zai",
		BaseURL: "https://api.z.ai/api/paas/v4",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "ZAI_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"glm-4.7-flash",
				"glm-4.5-flash",
				"glm-4.6v-flash",
			},
			Limits: map[string]types.ModelLimits{
				"glm-4.7-flash":  {MaxConcurrent: &conc1},
				"glm-4.5-flash":  {MaxConcurrent: &conc2},
				"glm-4.6v-flash": {MaxConcurrent: &conc1},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"glm-4.7-flash":  {StructuredOutputs: strPtr("json_schema_strict"), Tools: boolPtr(true), Reasoning: boolPtr(true)},
				"glm-4.5-flash":  {StructuredOutputs: strPtr("json_schema_strict"), Tools: boolPtr(true), Reasoning: boolPtr(true)},
				"glm-4.6v-flash": {Tools: boolPtr(true), Reasoning: boolPtr(true)},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "json_object",
			Logprobs:            false,
			Metadata:            false,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    false,
			PresencePenalty:     false,
			MaxTokens:           true,
			MaxCompletionTokens: true,
			MultipleChoices:     false,
			ToolSchema:          "json_schema",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "openai",
	}
}


func getCohereConfig() types.ProviderConfig {
	rpm20 := 20

	return types.ProviderConfig{
		ID:      "cohere",
		BaseURL: "https://api.cohere.com/v1",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "COHERE_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"command-a-03-2025",
				"command-r7b-12-2024",
			},
			Limits: map[string]types.ModelLimits{},
			Capabilities: map[string]types.ModelCapabilities{
				"command-a-03-2025": {StructuredOutputs: strPtr("none")},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               false,
			StructuredOutputs:   "json_schema_strict",
			Logprobs:            false,
			Metadata:            false,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    false,
			PresencePenalty:     false,
			MaxTokens:           true,
			MaxCompletionTokens: true,
			MultipleChoices:     false,
			ToolSchema:          "",
		},
		Limits: types.ProviderLimits{
			Rpm: &rpm20,
		},
		ProviderType: "cohere",
	}
}

func getGeminiConfig() types.ProviderConfig {
	rpm5 := 5
	rpm10 := 10
	rpm15 := 15
	rpm30 := 30
	rpd20 := 20
	rpd500 := 500
	rpd14400 := 14400
	tpm250000 := 250000
	tpm16000 := 16000

	return types.ProviderConfig{
		ID:      "gemini",
		BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "GEMINI_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"gemini-3.8-flash",
				"gemini-3.7-flash",
				"gemini-3.6-flash",
				"gemini-3-flash-preview",
				"gemini-3.5-flash",
				"gemini-3.5-flash-lite",
				"gemini-3.1-flash-lite",
				"gemma-4-31b-it",
				"gemma-4-26b-a4b-it",
				"gemini-2.5-flash",
				"gemini-2.5-flash-lite",
			},
			Limits: map[string]types.ModelLimits{
				"gemini-3.8-flash":       {Rpm: &rpm5, Rpd: &rpd20, Tpm: &tpm250000},
				"gemini-3.7-flash":       {Rpm: &rpm5, Rpd: &rpd20, Tpm: &tpm250000},
				"gemini-3.6-flash":       {Rpm: &rpm5, Rpd: &rpd20, Tpm: &tpm250000},
				"gemini-3-flash-preview": {Rpm: &rpm5, Rpd: &rpd20, Tpm: &tpm250000},
				"gemini-3.5-flash":       {Rpm: &rpm5, Rpd: &rpd20, Tpm: &tpm250000},
				"gemini-3.5-flash-lite":  {Rpm: &rpm15, Rpd: &rpd500, Tpm: &tpm250000},
				"gemini-3.1-flash-lite":  {Rpm: &rpm15, Rpd: &rpd500, Tpm: &tpm250000},
				"gemma-4-31b-it":         {Rpm: &rpm30, Rpd: &rpd14400, Tpm: &tpm16000},
				"gemma-4-26b-a4b-it":     {Rpm: &rpm30, Rpd: &rpd14400, Tpm: &tpm16000},
				"gemini-2.5-flash":       {Rpm: &rpm10, Rpd: &rpd20, Tpm: &tpm250000},
				"gemini-2.5-flash-lite":  {Rpm: &rpm10, Rpd: &rpd20, Tpm: &tpm250000},
			},
			Capabilities: map[string]types.ModelCapabilities{
				// Gemini's OpenAI-compat layer accepts reasoning_effort on
				// thinking-capable models and rejects it on Gemma.
				"gemini-2.5-flash":       {Reasoning: boolPtr(true)},
				"gemini-2.5-flash-lite":  {Reasoning: boolPtr(true)},
				"gemini-3.8-flash":       {Reasoning: boolPtr(true)},
				"gemini-3.7-flash":       {Reasoning: boolPtr(true)},
				"gemini-3.6-flash":       {Reasoning: boolPtr(true)},
				"gemini-3-flash-preview": {Reasoning: boolPtr(true)},
				"gemini-3.5-flash":       {Reasoning: boolPtr(true)},
				"gemini-3.5-flash-lite":  {Reasoning: boolPtr(true)},
				"gemini-3.1-flash-lite":  {Reasoning: boolPtr(true)},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "json_object",
			Logprobs:            false,
			Metadata:            false,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    false,
			PresencePenalty:     false,
			MaxTokens:           true,
			MaxCompletionTokens: false,
			MultipleChoices:     true,
			ToolSchema:          "json_schema",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "openai",
	}
}

// getNousConfig configures the Nous Research inference portal. Rate limits are
// ACCOUNT-WIDE (50 rpm / 500k tpm, 2100 rph / 6m tph per the response headers),
// so they live at the provider level and are enforced through the shared
// "__provider__" quota scope rather than per model.

func getNousConfig() types.ProviderConfig {
	rpm50 := 50
	rph2100 := 2100
	tpm500000 := 500000
	tph6000000 := 6000000
	// Account-wide in-flight cap: the portal rejects concurrent requests for
	// low-credit accounts ("Too many concurrent inference requests"), which
	// 429-benched every nous model under burst load. Gate locally instead.
	conc3 := 3

	return types.ProviderConfig{
		ID:      "nous",
		BaseURL: "https://inference-api.nousresearch.com/v1",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "NOUS_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"upstage/solar-mini4:free",
				"stepfun/step-3.7-flash:free",
				"meituan/longcat-2.0:free",
				"poolside/laguna-s-2.1:free",
				"poolside/laguna-xs-2.1:free",
			},
			Limits: map[string]types.ModelLimits{},
			Capabilities: map[string]types.ModelCapabilities{
				"upstage/solar-mini4:free": {
					StructuredOutputs: strPtr("json_schema_strict"),
				},
				"stepfun/step-3.7-flash:free": {
					// Probe passed on short prompts, but production structured
					// requests (~4-5K input tokens) repeatedly returned empty
					// choice-0 content - reasoning budget starved under real
					// loads. Text-only until re-verified.
					StructuredOutputs: strPtr("none"),
				},
				"meituan/longcat-2.0:free": {
					StructuredOutputs: strPtr("none"),
				},
				"poolside/laguna-s-2.1:free": {
					StructuredOutputs: strPtr("none"),
				},
				"poolside/laguna-xs-2.1:free": {
					StructuredOutputs: strPtr("none"),
				},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "json_schema_strict",
			Logprobs:            false,
			Metadata:            false,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    false,
			PresencePenalty:     false,
			MaxTokens:           true,
			MaxCompletionTokens: false,
			MultipleChoices:     false,
			ToolSchema:          "json_schema",
		},
		Limits: types.ProviderLimits{
			Rpm:           &rpm50,
			Rph:           &rph2100,
			Tpm:           &tpm500000,
			Tph:           &tph6000000,
			MaxConcurrent: &conc3,
		},
		ProviderType: "openai",
	}
}


func getOpenRouterConfig() types.ProviderConfig {
	rpm20 := 20
	rpd50 := 50

	return types.ProviderConfig{
		ID:      "openrouter",
		BaseURL: "https://openrouter.ai/api/v1",
		Auth: types.ProviderAuth{
			Type:     "bearer",
			Env:      "OPENROUTER_API_KEY",
			Optional: true,
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"nvidia/nemotron-3-super-120b-a12b:free",
				"nvidia/nemotron-3-ultra-550b-a55b:free",
				"nvidia/nemotron-3.5-lightning:free",
				"z-ai/glm-5.2:free",
				"google/gemma-4-31b-it:free",
				"google/gemma-4-26b-a4b-it:free",
				"cohere/north-mini-code:free",
				"poolside/laguna-s-2.1:free",
				"thinkingmachines/inkling:free",
				"dots-studio/dots-3-note-preview:free",
			},
			Limits: map[string]types.ModelLimits{},
			Capabilities: map[string]types.ModelCapabilities{
				// OpenRouter normalizes reasoning_effort into its unified
				// reasoning param and drops it for models that lack support,
				// so flagging the known reasoning families is safe here.
				"nvidia/nemotron-3-super-120b-a12b:free": {Reasoning: boolPtr(true)},
				"nvidia/nemotron-3-ultra-550b-a55b:free": {Reasoning: boolPtr(true)},
				"z-ai/glm-5.2:free":                      {Reasoning: boolPtr(true)},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               false,
			StructuredOutputs:   "json_object",
			Logprobs:            false,
			Metadata:            false,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    false,
			PresencePenalty:     false,
			MaxTokens:           true,
			MaxCompletionTokens: false,
			MultipleChoices:     false,
			ToolSchema:          "none",
		},
		Limits: types.ProviderLimits{
			Rpm: &rpm20,
			Rpd: &rpd50,
		},
		ProviderType: "openai",
	}
}


func getOrcaConfig() types.ProviderConfig {
	conc5 := 5
	ms15000 := 15000

	return types.ProviderConfig{
		ID:      "orca",
		BaseURL: "https://api.orcarouter.ai/v1",
		Auth: types.ProviderAuth{
			Type:     "bearer",
			Env:      "ORCAROUTER_API_KEY",
			Optional: true,
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"deepseek/deepseek-v4-flash-free",
				"deepseek/deepseek-v4.1-flash-free",
				"orcarouter/free",
				"orca/orcaverify-text1.0-free",
				"tencent/hy3-free",
				"tencent/hy4-preview-free",
				"z-ai/glm-5.3-flash-free",
			},
			Limits: map[string]types.ModelLimits{
				"deepseek/deepseek-v4-flash-free":   {TimeoutMs: &ms15000},
				"deepseek/deepseek-v4.1-flash-free": {TimeoutMs: &ms15000},
				"orcarouter/free":                   {TimeoutMs: &ms15000},
				"orca/orcaverify-text1.0-free":      {TimeoutMs: &ms15000},
				"tencent/hy3-free":                  {TimeoutMs: &ms15000},
				"tencent/hy4-preview-free":          {TimeoutMs: &ms15000},
				"z-ai/glm-5.3-flash-free":          {TimeoutMs: &ms15000},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"deepseek/deepseek-v4-flash-free": {
					StructuredOutputs: strPtr("json_schema_strict"),
					Tools:             boolPtr(true),
					Reasoning:         boolPtr(true),
				},
				"deepseek/deepseek-v4.1-flash-free": {
					StructuredOutputs: strPtr("json_schema_strict"),
					Tools:             boolPtr(true),
					Reasoning:         boolPtr(true),
				},
				"orcarouter/free": {
					StructuredOutputs: strPtr("json_schema_strict"),
					Tools:             boolPtr(true),
					Reasoning:         boolPtr(true),
				},
				"orca/orcaverify-text1.0-free": {
					StructuredOutputs: strPtr("json_schema_strict"),
					Tools:             boolPtr(false),
				},
				"tencent/hy3-free": {
					StructuredOutputs: strPtr("json_schema_strict"),
					Tools:             boolPtr(true),
					Reasoning:         boolPtr(true),
				},
				"tencent/hy4-preview-free": {
					StructuredOutputs: strPtr("json_schema_strict"),
					Tools:             boolPtr(true),
					Reasoning:         boolPtr(true),
				},
				"z-ai/glm-5.3-flash-free": {
					StructuredOutputs: strPtr("json_schema_strict"),
					Tools:             boolPtr(true),
					Reasoning:         boolPtr(true),
				},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "json_schema_strict",
			Logprobs:            false,
			Metadata:            false,
			Seed:                false,
			User:                true,
			FrequencyPenalty:    true,
			PresencePenalty:     true,
			MaxTokens:           true,
			MaxCompletionTokens: true,
			MultipleChoices:     false,
			ToolSchema:          "json_schema",
		},
		Limits: types.ProviderLimits{
			MaxConcurrent: &conc5,
		},
		ProviderType: "openai",
	}
}

func boolPtr(b bool) *bool {
	return &b
}


func strPtr(s string) *string {
	return &s
}
