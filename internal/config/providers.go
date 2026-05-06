package config

import "github.com/abdo-355/llm-gateway/internal/types"

func GetProviders() []types.ProviderConfig {
	return []types.ProviderConfig{
		getGroqConfig(),
		getCerebrasConfig(),
		getMistralConfig(),
		getNIMConfig(),
		getKiloConfig(),
		getCloudflareConfig(),
		getOpenCodeConfig(),
		getOllamaConfig(),
		getZaiConfig(),
		getLLM7Config(),
		getCohereConfig(),
		getOciConfig(),
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
			},
			Limits: map[string]types.ModelLimits{
				"openai/gpt-oss-120b": {
					Rpm: &rpm30,
					Rpd: &rpd1000,
					Tpm: &tpm8000,
					Tpd: &tpd200000,
				},
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
	rph900 := 900
	rpd14400 := 14400
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
				"qwen-3-235b-a22b-instruct-2507",
			},
			Limits: map[string]types.ModelLimits{
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
	rpm60 := 60
	rpm400 := 400
	tpm50000 := 50000
	tpm75000 := 75000
	tpm356250 := 356250
	tpm1500000 := 1500000
	tpmu4000000 := 4000000
	tpmu1000000000 := 1000000000

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
				"mistral-large-2512",
				"mistral-medium-2508",
				"mistral-small-2603",
				"mistral-medium-3.5",
			},
			Limits: map[string]types.ModelLimits{
				"magistral-medium-2509": {Rpm: &rpm4, Tpm: &tpm75000, Tpmu: &tpmu1000000000},
				"mistral-large-2512":    {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"mistral-medium-2508":   {Rpm: &rpm22, Tpm: &tpm356250},
				"mistral-small-2603":    {Rpm: &rpm400, Tpm: &tpm1500000},
				"mistral-medium-3.5":    {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
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

func GetCertifications() []types.Certification {
	return []types.Certification{
		{Provider: "kilo", Model: "kilo-auto/free", StrictSchema: true},
		{Provider: "llm7", Model: "fast", StrictSchema: true},
		{Provider: "llm7", Model: "default", StrictSchema: true},
		{Provider: "oci", Model: "meta.llama-3.3-70b-instruct", StrictSchema: true},
		{Provider: "ollama", Model: "gemma4:31b", StrictSchema: true},
		{Provider: "ollama", Model: "gpt-oss:20b", StrictSchema: true},
		{Provider: "ollama", Model: "nemotron-3-nano:30b", StrictSchema: true},
		{Provider: "ollama", Model: "qwen3-next:80b", StrictSchema: true},
		{Provider: "zai", Model: "glm-4.5-flash", StrictSchema: true},
		{Provider: "zai", Model: "glm-4.7-flash", StrictSchema: true},
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
				"moonshotai/kimi-k2.6",
				"moonshotai/kimi-k2-thinking",
				"qwen/qwen3-next-80b-a3b-thinking",
				"qwen/qwen3-next-80b-a3b-instruct",
				"qwen/qwen3.5-397b-a17b",
				"qwen/qwen3.5-122b-a10b",
				"mistralai/mistral-medium-3.5-128b",
				"mistralai/devstral-2-123b-instruct-2512",
			"mistralai/mistral-large-3-675b-instruct-2512",
			"deepseek-ai/deepseek-v3.1-terminus",
				"minimaxai/minimax-m2.5",
				"minimaxai/minimax-m2.7",
				"stepfun-ai/step-3.5-flash",
				"z-ai/glm-5.1",
				"z-ai/glm5",
				"z-ai/glm4.7",
				"openai/gpt-oss-120b",
			},
			Limits: map[string]types.ModelLimits{
				"moonshotai/kimi-k2-instruct":                  {Rpd: &rpd14400, Tpm: &tpm500000},
				"moonshotai/kimi-k2-instruct-0905":             {Rpd: &rpd14400, Tpm: &tpm500000},
				"moonshotai/kimi-k2.6":                         {Rpd: &rpd14400, Tpm: &tpm500000},
				"moonshotai/kimi-k2-thinking":                  {Rpd: &rpd14400, Tpm: &tpm500000},
				"qwen/qwen3-next-80b-a3b-thinking":             {Rpd: &rpd500, Tpm: &tpm250000},
				"qwen/qwen3-next-80b-a3b-instruct":             {Rpd: &rpd500, Tpm: &tpm250000},
				"qwen/qwen3.5-397b-a17b":                       {Rpd: &rpd500, Tpm: &tpm250000},
				"qwen/qwen3.5-122b-a10b":                       {Rpd: &rpd14400, Tpm: &tpm500000},
				"mistralai/mistral-medium-3.5-128b":            {Rpd: &rpd500, Tpm: &tpm250000},
				"mistralai/devstral-2-123b-instruct-2512":      {Rpd: &rpd500, Tpm: &tpm250000},
			"mistralai/mistral-large-3-675b-instruct-2512": {Rpd: &rpd500, Tpm: &tpm250000},
			"deepseek-ai/deepseek-v3.1-terminus":           {Rpd: &rpd14400, Tpm: &tpm500000},
				"minimaxai/minimax-m2.5":                       {Rpd: &rpd500, Tpm: &tpm250000},
				"minimaxai/minimax-m2.7":                       {Rpd: &rpd500, Tpm: &tpm250000},
				"stepfun-ai/step-3.5-flash":                    {Rpd: &rpd14400, Tpm: &tpm500000},
				"z-ai/glm-5.1":                                 {Rpd: &rpd500, Tpm: &tpm250000},
				"z-ai/glm5":                                    {Rpd: &rpd500, Tpm: &tpm250000},
				"z-ai/glm4.7":                                  {Rpd: &rpd14400, Tpm: &tpm500000},
				"openai/gpt-oss-120b":                          {Rpd: &rpd14400, Tpm: &tpm500000},
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
	rpm10 := 10

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
				"minimax-m2.5-free",
				"big-pickle",
				"ling-2.6-flash-free",
				"hy3-preview-free",
				"nemotron-3-super-free",
			},
			Limits: map[string]types.ModelLimits{
				"minimax-m2.5-free":     {Rpm: &rpm10},
				"big-pickle":            {Rpm: &rpm10},
				"ling-2.6-flash-free":   {Rpm: &rpm10},
				"hy3-preview-free":      {Rpm: &rpm10},
				"nemotron-3-super-free": {Rpm: &rpm10},
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
			ToolSchema:          "json_schema",
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
			"glm-4.7",
			"glm-4.6",
				"minimax-m2.1",
				"minimax-m2",
				"minimax-m2.7",
			"qwen3.5:397b",
			"mistral-large-3:675b",
		},
			Limits: map[string]types.ModelLimits{
				"qwen3-next:80b":            {MaxConcurrent: &conc1},
				"devstral-small-2:24b":      {MaxConcurrent: &conc1},
				"gemma4:31b":                {MaxConcurrent: &conc1},
				"gemma3:27b":                {MaxConcurrent: &conc1},
				"gemma3:12b":                {MaxConcurrent: &conc1},
				"nemotron-3-nano:30b":       {MaxConcurrent: &conc1},
				"gpt-oss:20b":               {MaxConcurrent: &conc1},
				"gemma3:4b":                 {MaxConcurrent: &conc1},
				"ministral-3:14b":           {MaxConcurrent: &conc1},
				"ministral-3:8b":            {MaxConcurrent: &conc1},
				"ministral-3:3b":            {MaxConcurrent: &conc1},
				"rnj-1:8b":                  {MaxConcurrent: &conc1},
				"deepseek-v3.2":             {MaxConcurrent: &conc1},
				"qwen3-coder:480b":          {MaxConcurrent: &conc1},
				"qwen3-coder-next":          {MaxConcurrent: &conc1},
				"devstral-2:123b":           {MaxConcurrent: &conc1},
				"minimax-m2.5":              {MaxConcurrent: &conc1},
				"nemotron-3-super":          {MaxConcurrent: &conc1},
				"cogito-2.1:671b":           {MaxConcurrent: &conc1},
			"deepseek-v3.1:671b":        {MaxConcurrent: &conc1},
			"gpt-oss:120b":              {MaxConcurrent: &conc1},
			"glm-4.7":                   {MaxConcurrent: &conc1},
			"glm-4.6":                   {MaxConcurrent: &conc1},
				"minimax-m2.1":              {MaxConcurrent: &conc1},
				"minimax-m2":                {MaxConcurrent: &conc1},
				"minimax-m2.7":              {MaxConcurrent: &conc1},
			"qwen3.5:397b":              {MaxConcurrent: &conc1},
			"mistral-large-3:675b":      {MaxConcurrent: &conc1},
		},
			Capabilities: map[string]types.ModelCapabilities{
				"cogito-2.1:671b":    {Tools: boolPtr(false)},
				"deepseek-v3.1:671b": {Tools: boolPtr(false)},
				"gemma3:12b":         {Tools: boolPtr(false)},
				"gemma3:27b":         {Tools: boolPtr(false)},
				"gemma3:4b":          {Tools: boolPtr(false)},
				"glm-4.6":            {Tools: boolPtr(false)},
			"glm-4.7":            {Tools: boolPtr(false)},
			"minimax-m2.1":       {Tools: boolPtr(false)},
				"rnj-1:8b":           {Tools: boolPtr(false)},
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
				"glm-4.7-flash",
				"glm-4.5-flash",
				"glm-4.6v-flash",
			},
			Limits: map[string]types.ModelLimits{
				"glm-4.7-flash":  {MaxConcurrent: &conc1},
				"glm-4.5-flash":  {MaxConcurrent: &conc2},
				"glm-4.6v-flash": {MaxConcurrent: &conc1},
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
			MultipleChoices:     false,
			ToolSchema:          "json_schema",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "openai",
	}
}

func getLLM7Config() types.ProviderConfig {
	rpm20 := 20
	rph100 := 100
	conc1 := 1
	cooldown1s := 1000

	return types.ProviderConfig{
		ID:      "llm7",
		BaseURL: "https://api.llm7.io/v1",
		Auth: types.ProviderAuth{
			Type:     "bearer",
			Env:      "LLM7_API_KEY",
			Optional: true,
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"gpt-oss-20b",
				"meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
				"codestral-latest",
				"ministral-8b-2512",
				"GLM-4.6V-Flash",
				"fast",
				"pro",
				"default",
				"deepseek-chat",
				"llama-3-70b-instruct",
			},
			Limits: map[string]types.ModelLimits{
				"gpt-oss-20b":                                     {Rpm: &rpm20, Rph: &rph100},
				"meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo":     {Rpm: &rpm20, Rph: &rph100},
				"codestral-latest":                                 {Rpm: &rpm20, Rph: &rph100},
				"ministral-8b-2512":                                {Rpm: &rpm20, Rph: &rph100},
				"GLM-4.6V-Flash":                                   {Rpm: &rpm20, Rph: &rph100},
			"fast":                                             {Rpm: &rpm20, Rph: &rph100, MaxConcurrent: &conc1, CooldownAfterMs: &cooldown1s},
			"pro":                                              {Rpm: &rpm20, Rph: &rph100},
			"default":                                          {Rpm: &rpm20, Rph: &rph100, MaxConcurrent: &conc1, CooldownAfterMs: &cooldown1s},
				"deepseek-chat":                                    {Rpm: &rpm20, Rph: &rph100},
				"llama-3-70b-instruct":                             {Rpm: &rpm20, Rph: &rph100},
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
				"command-a-reasoning-08-2025",
				"command-a-vision-03-2025",
				"command-r-plus-08-2025",
				"command-r-08-2025",
				"command-r7b-12-2024",
				"tiny-aya-global-03-2025",
			},
			Limits: map[string]types.ModelLimits{
				"command-a-03-2025":             {Rpm: &rpm20},
				"command-a-reasoning-08-2025":   {Rpm: &rpm20},
				"command-a-vision-03-2025":      {Rpm: &rpm20},
				"command-r-plus-08-2025":        {Rpm: &rpm20},
				"command-r-08-2025":             {Rpm: &rpm20},
				"command-r7b-12-2024":           {Rpm: &rpm20},
				"tiny-aya-global-03-2025":       {Rpm: &rpm20},
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
			ToolSchema:          "",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "cohere",
	}
}

func getOciConfig() types.ProviderConfig {
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
				"openai.gpt-oss-120b",
			},
			Limits: map[string]types.ModelLimits{
				"meta.llama-3.3-70b-instruct": {MaxConcurrent: intPtr(4)},
				"openai.gpt-oss-120b":         {MaxConcurrent: intPtr(4)},
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
