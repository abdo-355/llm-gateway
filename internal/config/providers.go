package config

import "github.com/abdo-355/llm-gateway/internal/types"

func GetProviders() []types.ProviderConfig {
	return []types.ProviderConfig{
		getGroqConfig(),
		getCerebrasConfig(),
		getMistralConfig(),
		getVertexConfig(),
		getGeminiConfig(),
		getNIMConfig(),
		getKiloConfig(),
		getOllamaConfig(),
	}
}

func getGroqConfig() types.ProviderConfig {
	rpm30 := 30
	rpm60 := 60
	rpd250 := 250
	rpd1000 := 1000
	rpd7000 := 7000
	rpd14400 := 14400
	tpm6000 := 6000
	tpm8000 := 8000
	tpm12000 := 12000
	tpm70000 := 70000
	tpm30000 := 30000
	tpd100000 := 100000
	tpd200000 := 200000
	tpd500000 := 500000

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
				"allam-2-7b",
				"groq/compound",
				"groq/compound-mini",
				"llama-3.1-8b-instant",
				"llama-3.3-70b-versatile",
				"meta-llama/llama-4-scout-17b-16e-instruct",
				"openai/gpt-oss-120b",
				"openai/gpt-oss-20b",
				"qwen/qwen3-32b",
			},
			Limits: map[string]types.ModelLimits{
				"allam-2-7b": {
					Rpm: &rpm30,
					Rpd: &rpd7000,
					Tpm: &tpm6000,
					Tpd: &tpd500000,
				},
				"groq/compound": {
					Rpm: &rpm30,
					Rpd: &rpd250,
					Tpm: &tpm70000,
				},
				"groq/compound-mini": {
					Rpm: &rpm30,
					Rpd: &rpd250,
					Tpm: &tpm70000,
				},
				"llama-3.1-8b-instant": {
					Rpm: &rpm30,
					Rpd: &rpd14400,
					Tpm: &tpm6000,
					Tpd: &tpd500000,
				},
				"llama-3.3-70b-versatile": {
					Rpm: &rpm30,
					Rpd: &rpd1000,
					Tpm: &tpm12000,
					Tpd: &tpd100000,
				},
				"meta-llama/llama-4-scout-17b-16e-instruct": {
					Rpm: &rpm30,
					Rpd: &rpd1000,
					Tpm: &tpm30000,
					Tpd: &tpd500000,
				},
				"openai/gpt-oss-120b": {
					Rpm: &rpm30,
					Rpd: &rpd1000,
					Tpm: &tpm8000,
					Tpd: &tpd200000,
				},
				"openai/gpt-oss-20b": {
					Rpm: &rpm30,
					Rpd: &rpd1000,
					Tpm: &tpm8000,
					Tpd: &tpd200000,
				},
				"qwen/qwen3-32b": {
					Rpm: &rpm60,
					Rpd: &rpd1000,
					Tpm: &tpm6000,
					Tpd: &tpd500000,
				},
			},
			Capabilities: map[string]types.ModelCapabilities{
				"allam-2-7b":         {Tools: boolPtr(false)},
				"groq/compound":      {Tools: boolPtr(false)},
				"groq/compound-mini": {Tools: boolPtr(false)},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "model_dependent",
			Logprobs:            false,
			Metadata:            false,
			Seed:                true,
			User:                true,
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

func getCerebrasConfig() types.ProviderConfig {
	rpm5 := 5
	rpm30 := 30
	rph900 := 900
	rpd14400 := 14400
	tpm60000 := 60000
	tpm30000 := 30000
	tph1000000 := 1000000
	tpd1000000 := 1000000

	return types.ProviderConfig{
		ID:      "cerebras",
		BaseURL: "https://api.cerebras.ai/v1",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "CEREBRAS_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"llama3.1-8b",
				"qwen-3-235b-a22b-instruct-2507",
			},
			Limits: map[string]types.ModelLimits{
				"llama3.1-8b": {
					Rpm: &rpm30,
					Rph: &rph900,
					Rpd: &rpd14400,
					Tpm: &tpm60000,
					Tph: &tph1000000,
					Tpd: &tpd1000000,
				},
				"qwen-3-235b-a22b-instruct-2507": {
					Rpm: &rpm5,
					Rph: &rph900,
					Rpd: &rpd14400,
					Tpm: &tpm30000,
					Tph: &tph1000000,
					Tpd: &tpd1000000,
				},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "model_dependent",
			Logprobs:            true,
			Metadata:            false,
			Seed:                true,
			User:                true,
			FrequencyPenalty:    true,
			PresencePenalty:     true,
			MaxTokens:           false,
			MaxCompletionTokens: true,
			MultipleChoices:     true,
			ToolSchema:          "json_schema",
		},
		Limits: types.ProviderLimits{},
	}
}

func getMistralConfig() types.ProviderConfig {
	rpm4 := 4
	rpm22 := 22
	rpm25 := 25
	rpm30 := 30
	rpm60 := 60
	rpm188 := 188
	rpm200 := 200
	rpm400 := 400
	rpm750 := 750
	tpm50000 := 50000
	tpm75000 := 75000
	tpm356250 := 356250
	tpm375000 := 375000
	tpm600000 := 600000
	tpm625000 := 625000
	tpm937500 := 937500
	tpm1300000 := 1300000
	tpm1500000 := 1500000
	tpmu4000000 := 4000000
	tpmu1000000000 := 1000000000
	tpmu200000000000 := 200000000000

	return types.ProviderConfig{
		ID:      "mistral",
		BaseURL: "https://api.mistral.ai/v1",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "MISTRAL_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"magistral-medium-2509",
				"magistral-small-2509",
				"ministral-14b-2512",
				"ministral-3b-2512",
				"ministral-8b-2512",
				"mistral-large-2411",
				"mistral-large-2512",
				"mistral-medium-2505",
				"mistral-medium-2508",
				"mistral-saba-2502",
				"mistral-small-2506",
				"mistral-small-2603",
				"open-mistral-nemo",
				"pixtral-large-2411",
			},
			Limits: map[string]types.ModelLimits{
				"magistral-medium-2509": {Rpm: &rpm4, Tpm: &tpm75000, Tpmu: &tpmu1000000000},
				"magistral-small-2509":  {Rpm: &rpm4, Tpm: &tpm75000, Tpmu: &tpmu1000000000},
				"ministral-14b-2512":    {Rpm: &rpm30, Tpm: &tpm937500},
				"ministral-3b-2512":     {Rpm: &rpm750, Tpm: &tpm1300000},
				"ministral-8b-2512":     {Rpm: &rpm188, Tpm: &tpm625000},
				"mistral-large-2411":    {Rpm: &rpm25, Tpm: &tpm600000, Tpmu: &tpmu200000000000},
				"mistral-large-2512":    {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"mistral-medium-2505":   {Rpm: &rpm25, Tpm: &tpm375000},
				"mistral-medium-2508":   {Rpm: &rpm22, Tpm: &tpm356250},
				"mistral-saba-2502":     {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"mistral-small-2506":    {Rpm: &rpm200, Tpm: &tpm1500000},
				"mistral-small-2603":    {Rpm: &rpm400, Tpm: &tpm1500000},
				"open-mistral-nemo":     {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"pixtral-large-2411":    {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "json_schema_strict",
			Logprobs:            false,
			Metadata:            true,
			Seed:                false,
			User:                false,
			FrequencyPenalty:    true,
			PresencePenalty:     true,
			MaxTokens:           true,
			MaxCompletionTokens: false,
			MultipleChoices:     true,
			ToolSchema:          "json_schema",
		},
		Limits: types.ProviderLimits{
			Rpm: &rpm60,
		},
	}
}

func getVertexConfig() types.ProviderConfig {
	rpm60 := 60

	return types.ProviderConfig{
		ID:      "vertex",
		BaseURL: "https://aiplatform.googleapis.com/v1beta1/projects/PROJECT_ID/locations/global/endpoints/openapi",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "GOOGLE_VERTEX_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"google/gemini-3.1-pro-preview",
				"google/gemini-3-flash-preview",
			},
			Limits: map[string]types.ModelLimits{
				"google/gemini-3.1-pro-preview": {
					Rpm: &rpm60,
				},
				"google/gemini-3-flash-preview": {
					Rpm: &rpm60,
				},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "json_schema",
			Logprobs:            false,
			Metadata:            false,
			Seed:                true,
			User:                false,
			FrequencyPenalty:    true,
			PresencePenalty:     true,
			MaxTokens:           true,
			MaxCompletionTokens: true,
			MultipleChoices:     true,
			ToolSchema:          "openapi",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "openai",
	}
}

func getGeminiConfig() types.ProviderConfig {
	rpm5 := 5
	rpm10 := 10
	rpm15 := 15
	rpd20 := 20
	rpd1500 := 1500
	rpd500 := 500
	tpm250000 := 250000

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
				"gemma-4-26b-a4b-it",
				"gemini-2.5-flash",
				"gemini-2.5-flash-lite",
				"gemini-3-flash-preview",
				"gemini-3.1-flash-lite-preview",
			},
			Limits: map[string]types.ModelLimits{
				"gemma-4-26b-a4b-it": {
					Rpm: &rpm15,
					Rpd: &rpd1500,
				},
				"gemini-2.5-flash": {
					Rpm: &rpm5,
					Rpd: &rpd20,
					Tpm: &tpm250000,
				},
				"gemini-2.5-flash-lite": {
					Rpm: &rpm10,
					Rpd: &rpd20,
					Tpm: &tpm250000,
				},
				"gemini-3-flash-preview": {
					Rpm: &rpm5,
					Rpd: &rpd20,
					Tpm: &tpm250000,
				},
				"gemini-3.1-flash-lite-preview": {
					Rpm: &rpm15,
					Rpd: &rpd500,
					Tpm: &tpm250000,
				},
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
			MaxCompletionTokens: false,
			MultipleChoices:     true,
			ToolSchema:          "json_schema",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "openai",
	}
}

// GetCertifications returns all model certifications for strict JSON
func GetCertifications() []types.Certification {
	return []types.Certification{
		// Mistral models with strict JSON certification
		{Provider: "mistral", Model: "mistral-large-2411", StrictSchema: true},
		{Provider: "mistral", Model: "open-mistral-nemo", StrictSchema: true},
		// Vertex models with strict JSON certification
		{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", StrictSchema: true},
		{Provider: "vertex", Model: "google/gemini-3-flash-preview", StrictSchema: true},
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
	rpm40 := 40
	rpd500 := 500
	rpd14400 := 14400
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
				"moonshotai/kimi-k2-instruct",
				"moonshotai/kimi-k2-instruct-0905",
				"moonshotai/kimi-k2.5",
				"moonshotai/kimi-k2-thinking",
				"qwen/qwen3-next-80b-a3b-thinking",
				"qwen/qwen3-next-80b-a3b-instruct",
				"qwen/qwen3.5-397b-a17b",
				"qwen/qwen3.5-122b-a10b",
				"mistralai/devstral-2-123b-instruct-2512",
				"mistralai/mistral-large-3-675b-instruct-2512",
				"mistralai/mistral-medium-3-instruct",
				"deepseek-ai/deepseek-v4-pro",
				"deepseek-ai/deepseek-v4-flash",
				"deepseek-ai/deepseek-v3.2",
				"deepseek-ai/deepseek-v3.1-terminus",
				"minimaxai/minimax-m2.5",
				"minimaxai/minimax-m2.7",
				"stepfun-ai/step-3.5-flash",
				"z-ai/glm-5.1",
				"z-ai/glm5",
				"z-ai/glm4.7",
				"openai/gpt-oss-120b",
				"google/gemma-4-31b-it",
			},
			Limits: map[string]types.ModelLimits{
				"moonshotai/kimi-k2-instruct":                  {Rpd: &rpd14400, Tpm: &tpm500000},
				"moonshotai/kimi-k2-instruct-0905":             {Rpd: &rpd14400, Tpm: &tpm500000},
				"moonshotai/kimi-k2.5":                         {Rpd: &rpd14400, Tpm: &tpm500000},
				"moonshotai/kimi-k2-thinking":                  {Rpd: &rpd14400, Tpm: &tpm500000},
				"qwen/qwen3-next-80b-a3b-thinking":             {Rpd: &rpd500, Tpm: &tpm250000},
				"qwen/qwen3-next-80b-a3b-instruct":             {Rpd: &rpd500, Tpm: &tpm250000},
				"qwen/qwen3.5-397b-a17b":                       {Rpd: &rpd500, Tpm: &tpm250000},
				"qwen/qwen3.5-122b-a10b":                       {Rpd: &rpd14400, Tpm: &tpm500000},
				"mistralai/devstral-2-123b-instruct-2512":      {Rpd: &rpd500, Tpm: &tpm250000},
				"mistralai/mistral-large-3-675b-instruct-2512": {Rpd: &rpd500, Tpm: &tpm250000},
				"mistralai/mistral-medium-3-instruct":          {Rpd: &rpd14400, Tpm: &tpm500000},
				"deepseek-ai/deepseek-v4-pro":                  {Rpd: &rpd14400, Tpm: &tpm500000},

				"deepseek-ai/deepseek-v3.2":          {Rpd: &rpd14400, Tpm: &tpm500000},
				"deepseek-ai/deepseek-v3.1-terminus": {Rpd: &rpd14400, Tpm: &tpm500000},
				"minimaxai/minimax-m2.5":             {Rpd: &rpd500, Tpm: &tpm250000},
				"minimaxai/minimax-m2.7":             {Rpd: &rpd500, Tpm: &tpm250000},
				"stepfun-ai/step-3.5-flash":          {Rpd: &rpd14400, Tpm: &tpm500000},
				"z-ai/glm-5.1":                       {Rpd: &rpd500, Tpm: &tpm250000},
				"z-ai/glm5":                          {Rpd: &rpd500, Tpm: &tpm250000},
				"z-ai/glm4.7":                        {Rpd: &rpd14400, Tpm: &tpm500000},
				"openai/gpt-oss-120b":                {Rpd: &rpd14400, Tpm: &tpm500000},
				"google/gemma-4-31b-it":              {Rpd: &rpd500, Tpm: &tpm250000},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:           true,
			Tools:               true,
			StructuredOutputs:   "model_dependent",
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
			Rpm: &rpm40,
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
				"kilo-auto/free",
				"stepfun/step-3.5-flash:free",
				"inclusionai/ling-2.6-flash:free",
				"openrouter/free",
				"tencent/hy3-preview:free",
				"nvidia/nemotron-3-super-120b-a12b:free",
				"inclusionai/ling-2.6-1t:free",
				"baidu/qianfan-ocr-fast:free",
			},
			Limits: map[string]types.ModelLimits{}, // Using provider-level rph limit
			Capabilities: map[string]types.ModelCapabilities{
				// Models empirically verified to support tool calling
				"inclusionai/ling-2.6-1t:free":           {Tools: boolPtr(true)},
				"inclusionai/ling-2.6-flash:free":        {Tools: boolPtr(true)},
				"kilo-auto/free":                         {Tools: boolPtr(true)},
				"nvidia/nemotron-3-super-120b-a12b:free": {Tools: boolPtr(true)},
				"openrouter/free":                        {Tools: boolPtr(true)},
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

func getOllamaConfig() types.ProviderConfig {
	rpm20 := 20

	return types.ProviderConfig{
		ID:      "ollama",
		BaseURL: "https://ollama.com/v1",
		Auth: types.ProviderAuth{
			Type: "bearer",
			Env:  "OLLAMA_API_KEY",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"qwen3-next:80b",
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
				"gemini-3-flash-preview",
				"glm-4.7",
				"glm-4.6",
				"minimax-m2.1",
				"minimax-m2",
				"minimax-m2.7",
				"qwen3.5:397b",
				"mistral-large-3:675b",
				"kimi-k2-thinking",
				"kimi-k2:1t",
				"qwen3-vl:235b-instruct",
				"qwen3-vl:235b",
			},
			Limits: map[string]types.ModelLimits{}, // using provider default
			Capabilities: map[string]types.ModelCapabilities{
				// Models empirically verified to support tool calling
				"devstral-2:123b":        {Tools: boolPtr(true)},
				"devstral-small-2:24b":   {Tools: boolPtr(true)},
				"gemini-3-flash-preview": {Tools: boolPtr(true)},
				"gemma4:31b":             {Tools: boolPtr(true)},
				"gpt-oss:120b":           {Tools: boolPtr(true)},
				"gpt-oss:20b":            {Tools: boolPtr(true)},
				"minimax-m2":             {Tools: boolPtr(true)},
				"minimax-m2.5":           {Tools: boolPtr(true)},
				"minimax-m2.7":           {Tools: boolPtr(true)},
				"ministral-3:14b":        {Tools: boolPtr(true)},
				"ministral-3:3b":         {Tools: boolPtr(true)},
				"ministral-3:8b":         {Tools: boolPtr(true)},
				"mistral-large-3:675b":   {Tools: boolPtr(true)},
				"nemotron-3-nano:30b":    {Tools: boolPtr(true)},
				"nemotron-3-super":       {Tools: boolPtr(true)},
				"qwen3-coder-next":       {Tools: boolPtr(true)},
				"qwen3-coder:480b":       {Tools: boolPtr(true)},
				"qwen3-vl:235b":          {Tools: boolPtr(true)},
				"qwen3-vl:235b-instruct": {Tools: boolPtr(true)},
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
			Rpm: &rpm20,
		},
		ProviderType: "openai",
	}
}

func boolPtr(b bool) *bool {
	return &b
}
