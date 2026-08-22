package config

import "github.com/abdo-355/llm-gateway/internal/types"

func GetProviders() []types.ProviderConfig {
	return []types.ProviderConfig{
		getGroqConfig(),
		getNIMConfig(),
		getKiloConfig(),
		getCloudflareConfig(),
		getOpenCodeConfig(),
		getOllamaConfig(),
		getZaiConfig(),
		getCohereConfig(),
		getOciConfig(),
		getGeminiConfig(),
		getNousConfig(),
		getOpenRouterConfig(),
		getOpenRouterAlphaConfig(),
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
			},
			Limits: map[string]types.ModelLimits{
				"openai/gpt-oss-120b": {Rpm: &rpm30, Rpd: &rpd1000, Tpm: &tpm8000, Tpd: &tpd200000},
				"openai/gpt-oss-20b":  {Rpm: &rpm30, Rpd: &rpd1000, Tpm: &tpm8000, Tpd: &tpd200000},
				"qwen/qwen3.6-27b":    {Rpm: &rpm30, Rpd: &rpd1000, Tpm: &tpm8000, Tpd: &tpd200000},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"openai/gpt-oss-120b": {Logprobs: boolPtr(false), Reasoning: boolPtr(true)},
				"openai/gpt-oss-20b":  {Logprobs: boolPtr(false), Reasoning: boolPtr(true)},
				"qwen/qwen3.6-27b":    {Logprobs: boolPtr(false)},
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
	return []types.Certification{
		{Provider: "oci", Model: "meta.llama-3.3-70b-instruct", StrictSchema: true},
	}
}

// LoadConfig returns the complete app configuration
func LoadConfig() types.AppConfig {
	return types.AppConfig{
		Providers:      GetProviders(),
		Certifications: GetCertifications(),
	}
}

func getNIMConfig() types.ProviderConfig {
	rpm35 := 35
	tpm250000 := 250000
	tpm500000 := 500000

	return types.ProviderConfig{
		ID:      "nim",
		BaseURL: "https://integrate.api.nvidia.com/v1",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "NIM_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"bytedance/seed-oss-36b-instruct",
				"meta/llama-3.1-70b-instruct",
				"meta/llama-3.2-90b-vision-instruct",
				"meta/llama-3.3-70b-instruct",
				"minimaxai/minimax-m2.7",
				"mistralai/ministral-14b-instruct-2512",
				"openai/gpt-oss-120b",
				"qwen/qwen3.5-122b-a10b",
				"qwen/qwen3.5-397b-a17b",
			},
			Limits: map[string]types.ModelLimits{
				"bytedance/seed-oss-36b-instruct":       {Tpm: &tpm500000},
				"meta/llama-3.1-70b-instruct":           {Tpm: &tpm250000},
				"meta/llama-3.2-90b-vision-instruct":    {Tpm: &tpm250000},
				"meta/llama-3.3-70b-instruct":           {Tpm: &tpm250000},
				"minimaxai/minimax-m2.7":                {Tpm: &tpm250000},
				"mistralai/ministral-14b-instruct-2512": {Tpm: &tpm500000},
				"openai/gpt-oss-120b":                   {Tpm: &tpm500000},
				"qwen/qwen3.5-122b-a10b":                {Tpm: &tpm500000},
				"qwen/qwen3.5-397b-a17b":                {Tpm: &tpm250000},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"mistralai/ministral-14b-instruct-2512": {StructuredOutputs: strPtr("none"), Tools: boolPtr(false)},
				"bytedance/seed-oss-36b-instruct":       {Tools: boolPtr(false)},
				"qwen/qwen3.5-122b-a10b":                {MaxTokens: boolPtr(false), MultipleChoices: boolPtr(false)},
				"qwen/qwen3.5-397b-a17b":                {PresencePenalty: boolPtr(false), MultipleChoices: boolPtr(false)},
				"openai/gpt-oss-120b":                   {Reasoning: boolPtr(true)},
				"minimaxai/minimax-m2.7":                {Reasoning: boolPtr(true)},
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
			MaxCompletionTokens: true,
			MultipleChoices:     false,
			ToolSchema:          "json_schema",
		},
		Limits: types.ProviderLimits{
			Rpm: &rpm35,
		},
		ProviderType: "openai",
	}
}

func getKiloConfig() types.ProviderConfig {
	rph200 := 200

	return types.ProviderConfig{
		ID:      "kilo",
		BaseURL: "https://api.kilo.ai/api/gateway",
		Auth: types.ProviderAuth{
			Type:     "bearer",
			Env:      "KILO_API_KEY",
			Optional: true,
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"poolside/laguna-s-2.1:free",
				"thinkingmachines/inkling:free",
				"nvidia/nemotron-3-ultra-550b-a55b:free",
				"openrouter/free",
			},
			Limits: map[string]types.ModelLimits{}, // Using provider-level rph limit
			Capabilities: map[string]types.ModelCapabilities{
				"poolside/laguna-s-2.1:free": {
					StructuredOutputs: strPtr("json_object"),
				},
				"thinkingmachines/inkling:free": {
					StructuredOutputs: strPtr("json_object"),
				},
				"nvidia/nemotron-3-ultra-550b-a55b:free": {
					StructuredOutputs: strPtr("json_object"),
				},
				"openrouter/free": {
					StructuredOutputs: strPtr("json_schema_strict"),
					Tools:             boolPtr(true),
				},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               false,
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
			Rph: &rph200,
		},
		ProviderType: "openai",
	}
}

func getCloudflareConfig() types.ProviderConfig {
	rpm1 := 1
	rpm2 := 2
	rpm3 := 3
	rpm5 := 5
	rpm10 := 10

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
				"@cf/openai/gpt-oss-20b",
				"@cf/qwen/qwen3-30b-a3b-fp8",
				"@cf/zai-org/glm-4.7-flash",
				"@cf/qwen/qwen2.5-coder-32b-instruct",
				"@cf/qwen/qwq-32b",
				"@cf/deepseek-ai/deepseek-r1-distill-qwen-32b",
				"@cf/meta/llama-4-scout-17b-16e-instruct",
				"@cf/mistralai/mistral-small-3.1-24b-instruct",
				"@cf/google/gemma-3-12b-it",
				"@cf/meta/llama-3.3-70b-instruct-fp8-fast",
				"@cf/ibm-granite/granite-4.0-h-micro",
				"@cf/meta/llama-3.2-3b-instruct",
				"@cf/meta/llama-3.2-1b-instruct",
				"@cf/google/gemma-4-26b-a4b-it",
				"@cf/openai/gpt-oss-120b",
				"@cf/nvidia/nemotron-3-120b-a12b",
				"@cf/moonshotai/kimi-k2.6",
				"@cf/moonshotai/kimi-k2.5",
			},
			Limits: map[string]types.ModelLimits{
				"@cf/openai/gpt-oss-20b":                       {Rpm: &rpm5},
				"@cf/qwen/qwen3-30b-a3b-fp8":                   {Rpm: &rpm10},
				"@cf/zai-org/glm-4.7-flash":                    {Rpm: &rpm10},
				"@cf/qwen/qwen2.5-coder-32b-instruct":          {Rpm: &rpm3},
				"@cf/qwen/qwq-32b":                             {Rpm: &rpm3},
				"@cf/deepseek-ai/deepseek-r1-distill-qwen-32b": {Rpm: &rpm1},
				"@cf/meta/llama-4-scout-17b-16e-instruct":      {Rpm: &rpm5},
				"@cf/mistralai/mistral-small-3.1-24b-instruct": {Rpm: &rpm5},
				"@cf/google/gemma-3-12b-it":                    {Rpm: &rpm5},
				"@cf/meta/llama-3.3-70b-instruct-fp8-fast":     {Rpm: &rpm3},
				"@cf/ibm-granite/granite-4.0-h-micro":          {Rpm: &rpm10},
				"@cf/meta/llama-3.2-3b-instruct":               {Rpm: &rpm10},
				"@cf/meta/llama-3.2-1b-instruct":               {Rpm: &rpm10},
				"@cf/google/gemma-4-26b-a4b-it":                {Rpm: &rpm5},
				"@cf/openai/gpt-oss-120b":                      {Rpm: &rpm3},
				"@cf/nvidia/nemotron-3-120b-a12b":              {Rpm: &rpm3},
				"@cf/moonshotai/kimi-k2.6":                     {Rpm: &rpm2},
				"@cf/moonshotai/kimi-k2.5":                     {Rpm: &rpm2},
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
		Limits:       types.ProviderLimits{},
		ProviderType: "cloudflare_workers_ai",
	}
}

func getOpenCodeConfig() types.ProviderConfig {
	conc5 := 5
	conc10 := 10
	pause24h := 24 * 60 * 60 * 1000
	pause6h := 6 * 60 * 60 * 1000

	return types.ProviderConfig{
		ID:      "opencode",
		BaseURL: "https://opencode.ai/zen/v1",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "OPENCODE_ZEN_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"x-preview-f-free",
				"hy3-free",
				"mimo-v2.5-free",
				"muse-spark-1.2-contributor-free",
				"nemotron-3-ultra-free",
				"nemotron-3.5-lightning-free",
			},
			Limits: map[string]types.ModelLimits{
				"x-preview-f-free":                {MaxConcurrent: &conc10, RateLimitPauseMs: &pause6h},
				"hy3-free":                        {MaxConcurrent: &conc5, RateLimitPauseMs: &pause24h},
				"mimo-v2.5-free":                  {MaxConcurrent: &conc5, RateLimitPauseMs: &pause24h},
				"muse-spark-1.2-contributor-free": {MaxConcurrent: &conc5, RateLimitPauseMs: &pause24h},
				"nemotron-3-ultra-free":           {MaxConcurrent: &conc5, RateLimitPauseMs: &pause24h},
				"nemotron-3.5-lightning-free":     {MaxConcurrent: &conc5, RateLimitPauseMs: &pause24h},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"hy3-free": {
					StructuredOutputs: strPtr("json_object"),
				},
				"mimo-v2.5-free": {
					StructuredOutputs: strPtr("json_schema"),
				},
				"nemotron-3-ultra-free": {
					StructuredOutputs: strPtr("json_schema_strict"),
					Tools:             boolPtr(true),
					ToolSchema:        strPtr("json_schema"),
					Reasoning:         boolPtr(true),
				},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               false,
			StructuredOutputs:   "none",
			Logprobs:            false,
			Metadata:            true,
			Seed:                true,
			User:                true,
			FrequencyPenalty:    true,
			PresencePenalty:     true,
			MaxTokens:           true,
			MaxCompletionTokens: true,
			MultipleChoices:     false,
			ToolSchema:          "none",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "openai",
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
				"llama3.3:70b",
				"devstral-small-2:24b",
				"gemma4:31b",
				"gemma3:27b",
				"gemma3:12b",
				"nemotron-3-nano:30b",
				"gpt-oss:20b",
				"gemma3:4b",
				"ministral-3:14b",
				"ministral-3:8b",
				"ministral-3:3b",
				"rnj-1:8b",
				"deepseek-v3.2",
				"qwen3-coder:480b",
				"qwen3-coder-next",
				"devstral-2:123b",
				"minimax-m2.5",
				"nemotron-3-super",
				"cogito-2.1:671b",
				"deepseek-v3.1:671b",
				"gpt-oss:120b",
				"glm-4.7",
				"glm-4.6",
				"minimax-m2.1",
				"minimax-m2",
				"minimax-m2.7",
				"mistral-large-3:675b",
			},
			Limits: map[string]types.ModelLimits{
				"llama3.3:70b":         {MaxConcurrent: &conc1},
				"devstral-small-2:24b": {MaxConcurrent: &conc1},
				"gemma4:31b":           {MaxConcurrent: &conc1},
				"gemma3:27b":           {MaxConcurrent: &conc1},
				"gemma3:12b":           {MaxConcurrent: &conc1},
				"nemotron-3-nano:30b":  {MaxConcurrent: &conc1},
				"gpt-oss:20b":          {MaxConcurrent: &conc1},
				"gemma3:4b":            {MaxConcurrent: &conc1},
				"ministral-3:14b":      {MaxConcurrent: &conc1},
				"ministral-3:8b":       {MaxConcurrent: &conc1},
				"ministral-3:3b":       {MaxConcurrent: &conc1},
				"rnj-1:8b":             {MaxConcurrent: &conc1},
				"deepseek-v3.2":        {MaxConcurrent: &conc1},
				"qwen3-coder:480b":     {MaxConcurrent: &conc1},
				"qwen3-coder-next":     {MaxConcurrent: &conc1},
				"devstral-2:123b":      {MaxConcurrent: &conc1},
				"minimax-m2.5":         {MaxConcurrent: &conc1},
				"nemotron-3-super":     {MaxConcurrent: &conc1},
				"cogito-2.1:671b":      {MaxConcurrent: &conc1},
				"deepseek-v3.1:671b":   {MaxConcurrent: &conc1},
				"gpt-oss:120b":         {MaxConcurrent: &conc1},
				"glm-4.7":              {MaxConcurrent: &conc1},
				"glm-4.6":              {MaxConcurrent: &conc1},
				"minimax-m2.1":         {MaxConcurrent: &conc1},
				"minimax-m2":           {MaxConcurrent: &conc1},
				"minimax-m2.7":         {MaxConcurrent: &conc1},
				"mistral-large-3:675b": {MaxConcurrent: &conc1},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"cogito-2.1:671b":    {Tools: boolPtr(false), Reasoning: boolPtr(true)},
				"deepseek-v3.1:671b": {Tools: boolPtr(false), Reasoning: boolPtr(true)},
				"deepseek-v3.2":      {Reasoning: boolPtr(true)},
				"gemma3:12b":         {Tools: boolPtr(false)},
				"gemma3:27b":         {Tools: boolPtr(false)},
				"gemma3:4b":          {Tools: boolPtr(false)},
				"glm-4.6":            {Tools: boolPtr(false), Reasoning: boolPtr(true)},
				"glm-4.7":            {Tools: boolPtr(false), Reasoning: boolPtr(true)},
				"minimax-m2":         {Reasoning: boolPtr(true)},
				"minimax-m2.1":       {Tools: boolPtr(false), Reasoning: boolPtr(true)},
				"minimax-m2.5":       {Reasoning: boolPtr(true)},
				"minimax-m2.7":       {Reasoning: boolPtr(true)},
				"qwen3-coder-next":   {Reasoning: boolPtr(true)},
				"qwen3-coder:480b":   {Reasoning: boolPtr(true)},
				"rnj-1:8b":           {Tools: boolPtr(false)},
				"gpt-oss:20b":        {Reasoning: boolPtr(true)},
				"gpt-oss:120b":       {Reasoning: boolPtr(true)},
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
		Limits:       types.ProviderLimits{},
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
				"glm-4.5-flash",
				"glm-4.6v-flash",
			},
			Limits: map[string]types.ModelLimits{
				"glm-4.5-flash":  {MaxConcurrent: &conc2},
				"glm-4.6v-flash": {MaxConcurrent: &conc1},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"glm-4.5-flash":  {StructuredOutputs: strPtr("json_schema_strict"), Reasoning: boolPtr(true)},
				"glm-4.6v-flash": {Reasoning: boolPtr(true)},
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
			Limits: map[string]types.ModelLimits{
				"command-a-03-2025":   {Rpm: &rpm20},
				"command-r7b-12-2024": {Rpm: &rpm20},
			},
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
		Limits:       types.ProviderLimits{},
		ProviderType: "cohere",
	}
}
func getOciConfig() types.ProviderConfig {
	conc20 := 20

	return types.ProviderConfig{
		ID:      "oci",
		BaseURL: "https://inference.generativeai.eu-frankfurt-1.oci.oraclecloud.com/openai/v1",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "OCI_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"meta.llama-3.3-70b-instruct",
			},
			Limits: map[string]types.ModelLimits{
				"meta.llama-3.3-70b-instruct": {MaxConcurrent: &conc20},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"meta.llama-3.3-70b-instruct": {StructuredOutputs: strPtr("json_schema_strict"), Logprobs: boolPtr(true)},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "json_schema",
			Logprobs:            false,
			Metadata:            false,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    false,
			PresencePenalty:     false,
			MaxTokens:           true,
			MaxCompletionTokens: true,
			MultipleChoices:     true,
			ToolSchema:          "json_schema",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "openai",
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
	ms60000 := 60000

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
				"stealth/ox-alpha",
				"upstage/solar-pro4:free",
				"stepfun/step-3.7-flash:free",
				"tencent/hy3:free",
				"meituan/longcat-2.0:free",
				"poolside/laguna-s-2.1:free",
				"poolside/laguna-xs-2.1:free",
			},
			Limits: map[string]types.ModelLimits{
				// Live generations finish at 19-27s; the default tier SLO of
				// 30s killed legitimate completions at the wall.
				"stealth/ox-alpha": {TimeoutMs: &ms60000},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"stealth/ox-alpha": {
					StructuredOutputs: strPtr("json_object"),
					Tools:             boolPtr(true),
					Reasoning:         boolPtr(true),
				},
				"upstage/solar-pro4:free": {
					StructuredOutputs: strPtr("json_schema_strict"),
				},
				"stepfun/step-3.7-flash:free": {
					// Probe passed on short prompts, but production structured
					// requests (~4-5K input tokens) repeatedly returned empty
					// choice-0 content - reasoning budget starved under real
					// loads. Text-only until re-verified.
					StructuredOutputs: strPtr("none"),
				},
				"tencent/hy3:free": {
					// Live probe: returns malformed JSON on response_format
					// requests ("}{": {"ok": true}}) — text-only until fixed.
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
			ToolSchema:          "none",
		},
		Limits: types.ProviderLimits{
			Rpm: &rpm20,
			Rpd: &rpd50,
		},
		ProviderType: "openai",
	}
}

// getOpenRouterAlphaConfig isolates stealth/ox-alpha under its own provider ID
// so it gets a dedicated Redis quota scope with concurrency-only handling — no
// request/token counters, no shared free-pool limits.
func getOpenRouterAlphaConfig() types.ProviderConfig {
	conc15 := 15

	return types.ProviderConfig{
		ID:      "openrouter-alpha",
		BaseURL: "https://openrouter.ai/api/v1",
		Auth: types.ProviderAuth{
			Type:     "bearer",
			Env:      "OPENROUTER_API_KEY",
			Optional: true,
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"stealth/ox-alpha",
			},
			Limits: map[string]types.ModelLimits{
				"stealth/ox-alpha": {MaxConcurrent: &conc15},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"stealth/ox-alpha": {
					StructuredOutputs: strPtr("json_object"),
					Tools:             boolPtr(true),
					Reasoning:         boolPtr(true),
				},
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
			MultipleChoices:     false,
			ToolSchema:          "json_schema",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "openai",
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func strPtr(s string) *string {
	return &s
}
