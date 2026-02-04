// Package config provides static configuration for providers and models.
package config

import "github.com/abdo-355/llm-gateway/internal/types"

// GetProviders returns all provider configurations
func GetProviders() []types.ProviderConfig {
	return []types.ProviderConfig{
		getGroqConfig(),
		getCerebrasConfig(),
		getMistralConfig(),
		getVertexConfig(),
	}
}

func getGroqConfig() types.ProviderConfig {
	rpm30 := 30
	rpm60 := 60
	tpm6000 := 6000
	tpm70000 := 70000
	tpm140000 := 140000

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
				"llama-3.3-70b-versatile",
				"llama-3.1-8b-instant",
				"llama-4-scout-17b-16e-instruct",
				"llama-4-maverick-17b-128e-instruct",
				"kimi-k2-0711-preview",
				"kimi-k2-0711-instruct",
				"gpt-oss-120b",
				"gpt-oss-20b",
				"qwen3-32b",
			},
			Limits: map[string]types.ModelLimits{
				"llama-3.3-70b-versatile": {
					Rpm: &rpm30,
					Tpm: &tpm70000,
				},
				"llama-3.1-8b-instant": {
					Rpm: &rpm60,
					Tpm: &tpm140000,
				},
				"llama-4-scout-17b-16e-instruct": {
					Rpm: &rpm30,
					Tpm: &tpm70000,
				},
				"llama-4-maverick-17b-128e-instruct": {
					Rpm: &rpm30,
					Tpm: &tpm70000,
				},
				"kimi-k2-0711-preview": {
					Rpm: &rpm30,
					Tpm: &tpm70000,
				},
				"kimi-k2-0711-instruct": {
					Rpm: &rpm30,
					Tpm: &tpm70000,
				},
				"gpt-oss-120b": {
					Rpm: &rpm30,
					Tpm: &tpm6000,
				},
				"gpt-oss-20b": {
					Rpm: &rpm30,
					Tpm: &tpm140000,
				},
				"qwen3-32b": {
					Rpm: &rpm30,
					Tpm: &tpm70000,
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
	rph1000 := 1000
	tpm10000 := 10000
	tpm140000 := 140000

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
				"llama-3.3-70b",
				"llama3.1-8b",
				"qwen-3-32b",
				"qwen-3-235b-a22b-instruct-2507",
				"zai-glm-4.7",
			},
			Limits: map[string]types.ModelLimits{
				"gpt-oss-120b": {
					Rpm: &rpm30,
					Rph: &rph1000,
					Tpm: &tpm10000,
				},
				"llama-3.3-70b": {
					Rpm: &rpm30,
					Rph: &rph1000,
					Tpm: &tpm140000,
				},
				"llama3.1-8b": {
					Rpm: &rpm30,
					Rph: &rph1000,
					Tpm: &tpm140000,
				},
				"qwen-3-32b": {
					Rpm: &rpm30,
					Rph: &rph1000,
					Tpm: &tpm140000,
				},
				"qwen-3-235b-a22b-instruct-2507": {
					Rpm: &rpm30,
					Rph: &rph1000,
					Tpm: &tpm10000,
				},
				"zai-glm-4.7": {
					Rpm: &rpm30,
					Rph: &rph1000,
					Tpm: &tpm140000,
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
	tpd200000 := 200000
	tpd500000 := 500000

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
				"mistral-large-latest",
				"mistral-medium-latest",
				"codestral-latest",
				"codestral-mamba-latest",
				"ministral-3b-latest",
				"ministral-8b-latest",
				"open-mistral-nemo",
				"open-mistral-nemo-2407",
				"open-codestral-mamba",
				"open-mixtral-8x7b",
				"open-mixtral-8x22b",
				"open-mixtral-8x22b-2404",
				"mistral-tiny",
				"mistral-tiny-2312",
				"mistral-small",
				"mistral-small-2312",
				"mistral-small-2402",
				"mistral-small-2409",
				"mistral-embed",
			},
			Limits: map[string]types.ModelLimits{
				"mistral-large-latest": {
					Tpd: &tpd200000,
				},
				"codestral-latest": {
					Tpd: &tpd500000,
				},
				"codestral-mamba-latest": {
					Tpd: &tpd500000,
				},
				"ministral-3b-latest": {
					Tpd: &tpd500000,
				},
				"ministral-8b-latest": {
					Tpd: &tpd500000,
				},
				"open-mistral-nemo": {
					Tpd: &tpd500000,
				},
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
		BaseURL: "https://aiplatform.googleapis.com/v1",
		Auth: types.ProviderAuth{
			Type:       "header",
			Env:        "GOOGLE_VERTEX_API_KEY",
			HeaderName: "x-goog-api-key",
		},
		Models: types.ProviderModels{
			Mode: "allowlist",
			List: []string{
				"gemini-3-pro-preview",
				"gemini-3-flash-preview",
			},
			Limits: map[string]types.ModelLimits{
				"gemini-3-pro-preview": {
					Rpm: &rpm60,
				},
				"gemini-3-flash-preview": {
					Rpm: &rpm60,
				},
			},
		},
		Capabilities: types.ProviderCapabilities{
			Streaming:         true,
			Tools:             true,
			StructuredOutputs: "json_schema_strict",
		},
		Limits:       types.ProviderLimits{},
		ProviderType: "vertex",
	}
}

// GetCertifications returns all model certifications for strict JSON
func GetCertifications() []types.Certification {
	return []types.Certification{
		// Mistral models with strict JSON certification
		{Provider: "mistral", Model: "mistral-large-latest", StrictSchema: true},
		{Provider: "mistral", Model: "codestral-latest", StrictSchema: true},
		{Provider: "mistral", Model: "codestral-mamba-latest", StrictSchema: true},
		{Provider: "mistral", Model: "ministral-3b-latest", StrictSchema: true},
		{Provider: "mistral", Model: "ministral-8b-latest", StrictSchema: true},
		{Provider: "mistral", Model: "open-mistral-nemo", StrictSchema: true},
		{Provider: "mistral", Model: "open-mistral-nemo-2407", StrictSchema: true},
		// Vertex models with strict JSON certification
		{Provider: "vertex", Model: "gemini-3-pro-preview", StrictSchema: true},
		{Provider: "vertex", Model: "gemini-3-flash-preview", StrictSchema: true},
	}
}

// LoadConfig returns the complete app configuration
func LoadConfig() types.AppConfig {
	return types.AppConfig{
		Providers:      GetProviders(),
		Certifications: GetCertifications(),
	}
}
