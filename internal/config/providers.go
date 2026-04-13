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
	rpd250 := 250
	rpd1000 := 1000
	rpd7000 := 7000
	rpd14400 := 14400
	tpm6000 := 6000
	tpm8000 := 8000
	tpm10000 := 10000
	tpm12000 := 12000
	tpm70000 := 70000
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
				"groq/compound",
				"groq/compound-mini",
				"llama-3.1-8b-instant",
				"llama-3.3-70b-versatile",
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
					Rpm: &rpm30,
					Rph: &rph900,
					Rpd: &rpd14400,
					Tpm: &tpm30000,
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
	rpm4 := 4
	rpm25 := 25
	rpm30 := 30
	rpm60 := 60
	tpm50000 := 50000
	tpm75000 := 75000
	tpm375000 := 375000
	tpm600000 := 600000
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
				"ministral-14b-2512":    {Rpm: &rpm30, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"ministral-3b-2512":     {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"ministral-8b-2512":     {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"mistral-large-2411":    {Rpm: &rpm60, Tpm: &tpm600000, Tpmu: &tpmu200000000000},
				"mistral-large-2512":    {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"mistral-medium-2505":   {Rpm: &rpm25, Tpm: &tpm375000},
				"mistral-medium-2508":   {Rpm: &rpm25, Tpm: &tpm375000},
				"mistral-saba-2502":     {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"mistral-small-2506":    {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"mistral-small-2603":    {Rpm: &rpm60, Tpm: &tpm375000},
				"open-mistral-nemo":     {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
				"pixtral-large-2411":    {Rpm: &rpm60, Tpm: &tpm50000, Tpmu: &tpmu4000000},
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
				"gemini-2.5-flash",
				"gemini-2.5-flash-lite",
				"gemini-3-flash-preview",
				"gemini-3.1-flash-lite-preview",
			},
			Limits: map[string]types.ModelLimits{
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
