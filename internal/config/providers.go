package config

import "github.com/abdo-355/llm-gateway/internal/types"

func GetProviders() []types.ProviderConfig {
	return []types.ProviderConfig{
		getGroqConfig(),
		getCerebrasConfig(),
		getMistralConfig(),
		getVertexConfig(),
		getGeminiConfig(),
	}
}

func getGroqConfig() types.ProviderConfig {
	rpm30 := 30
	rpm60 := 60
	rpd1000 := 1000
	rpd7000 := 7000
	rpd14400 := 14400
	tpm6000 := 6000
	tpm8000 := 8000
	tpm10000 := 10000
	tpm12000 := 12000
	tpm30000 := 30000
	tpd100000 := 100000
	tpd200000 := 200000
	tpd300000 := 300000
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
				"llama-3.1-8b-instant",
				"llama-3.3-70b-versatile",
				"meta-llama/llama-4-maverick-17b-128e-instruct",
				"meta-llama/llama-4-scout-17b-16e-instruct",
				"moonshotai/kimi-k2-instruct",
				"moonshotai/kimi-k2-instruct-0905",
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
				"meta-llama/llama-4-maverick-17b-128e-instruct": {
					Rpm: &rpm30,
					Rpd: &rpd1000,
					Tpm: &tpm6000,
					Tpd: &tpd500000,
				},
				"meta-llama/llama-4-scout-17b-16e-instruct": {
					Rpm: &rpm30,
					Rpd: &rpd1000,
					Tpm: &tpm30000,
					Tpd: &tpd500000,
				},
				"moonshotai/kimi-k2-instruct": {
					Rpm: &rpm60,
					Rpd: &rpd1000,
					Tpm: &tpm10000,
					Tpd: &tpd300000,
				},
				"moonshotai/kimi-k2-instruct-0905": {
					Rpm: &rpm60,
					Rpd: &rpd1000,
					Tpm: &tpm10000,
					Tpd: &tpd300000,
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
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:         true,
			Tools:             true,
			StructuredOutputs: "model_dependent",
		},
		Limits: types.ProviderLimits{},
	}
}

func getCerebrasConfig() types.ProviderConfig {
	rpm30 := 30
	rph900 := 900
	rpd14400 := 14400
	tpm60000 := 60000
	tpm64000 := 64000
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
				"gpt-oss-120b",
				"llama3.1-8b",
			},
			Limits: map[string]types.ModelLimits{
				"gpt-oss-120b": {
					Rpm: &rpm30,
					Rph: &rph900,
					Rpd: &rpd14400,
					Tpm: &tpm64000,
					Tph: &tph1000000,
					Tpd: &tpd1000000,
				},
				"llama3.1-8b": {
					Rpm: &rpm30,
					Rph: &rph900,
					Rpd: &rpd14400,
					Tpm: &tpm60000,
					Tph: &tph1000000,
					Tpd: &tpd1000000,
				},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:         true,
			Tools:             true,
			StructuredOutputs: "model_dependent",
		},
		Limits: types.ProviderLimits{},
	}
}

func getMistralConfig() types.ProviderConfig {
	rpm60 := 60
	tpm500000 := 500000

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
				"codestral-2405",
				"codestral-2501",
				"codestral-mamba-2407",
				"ministral-3b-2410",
				"ministral-8b-2410",
				"mistral-large-2402",
				"mistral-large-2407",
				"mistral-large-2411",
				"mistral-medium",
				"mistral-saba-2502",
				"mistral-small-2402",
				"mistral-small-2409",
				"mistral-small-2501",
				"mistral-small-2503",
				"open-mistral-7b",
				"open-mistral-nemo",
				"open-mixtral-8x22b",
				"open-mixtral-8x7b",
			},
			Limits: map[string]types.ModelLimits{
				"codestral-2405":    {Tpm: &tpm500000},
				"codestral-2501":    {Tpm: &tpm500000},
				"codestral-mamba-2407": {Tpm: &tpm500000},
				"ministral-3b-2410": {Tpm: &tpm500000},
				"ministral-8b-2410": {Tpm: &tpm500000},
				"mistral-large-2402": {Tpm: &tpm500000},
				"mistral-large-2407": {Tpm: &tpm500000},
				"mistral-large-2411": {Tpm: &tpm500000},
				"mistral-medium":    {Tpm: &tpm500000},
				"mistral-saba-2502": {Tpm: &tpm500000},
				"mistral-small-2402": {Tpm: &tpm500000},
				"mistral-small-2409": {Tpm: &tpm500000},
				"mistral-small-2501": {Tpm: &tpm500000},
				"mistral-small-2503": {Tpm: &tpm500000},
				"open-mistral-7b":   {Tpm: &tpm500000},
				"open-mistral-nemo": {Tpm: &tpm500000},
				"open-mixtral-8x22b": {Tpm: &tpm500000},
				"open-mixtral-8x7b": {Tpm: &tpm500000},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:         true,
			Tools:             true,
			StructuredOutputs: "json_schema_strict",
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
			Type: "adc",
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
			Streaming:         true,
			Tools:             true,
			StructuredOutputs: "json_schema",
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
				"google/gemini-2.5-flash",
				"google/gemini-2.5-flash-lite",
				"google/gemini-3-flash-preview",
				"google/gemini-3.1-flash-lite-preview",
			},
			Limits: map[string]types.ModelLimits{
				"google/gemini-2.5-flash": {
					Rpm: &rpm5,
					Rpd: &rpd20,
					Tpm: &tpm250000,
				},
				"google/gemini-2.5-flash-lite": {
					Rpm: &rpm10,
					Rpd: &rpd20,
					Tpm: &tpm250000,
				},
				"google/gemini-3-flash-preview": {
					Rpm: &rpm5,
					Rpd: &rpd20,
					Tpm: &tpm250000,
				},
				"google/gemini-3.1-flash-lite-preview": {
					Rpm: &rpm15,
					Rpd: &rpd500,
					Tpm: &tpm250000,
				},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:         true,
			Tools:             true,
			StructuredOutputs: "json_schema",
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
		{Provider: "mistral", Model: "codestral-2501", StrictSchema: true},
		{Provider: "mistral", Model: "codestral-mamba-2407", StrictSchema: true},
		{Provider: "mistral", Model: "ministral-3b-2410", StrictSchema: true},
		{Provider: "mistral", Model: "ministral-8b-2410", StrictSchema: true},
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
