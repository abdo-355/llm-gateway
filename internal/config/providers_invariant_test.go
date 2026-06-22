package config

import (
	"testing"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderRegistryInvariants(t *testing.T) {
	providers := GetProviders()
	require.NotEmpty(t, providers)

	validProviderTypes := map[string]bool{
		"":                      true,
		"openai":                true,
		"cloudflare_workers_ai": true,
		"ollama":                true,
		"cohere":                true,
	}
	validAuthTypes := map[string]bool{
		"none":   true,
		"bearer": true,
		"header": true,
	}
	validStructuredOutputs := map[string]bool{
		"none":               true,
		"json_object":        true,
		"json_schema":        true,
		"json_schema_strict": true,
		"model_dependent":    true,
		"unknown":            true,
	}

	providerIDs := make(map[string]struct{}, len(providers))
	providerModels := make(map[string]map[string]struct{}, len(providers))

	for _, provider := range providers {
		t.Run(provider.ID, func(t *testing.T) {
			require.NotEmpty(t, provider.ID)
			require.NotEmpty(t, provider.BaseURL)
			assert.True(t, validProviderTypes[provider.ProviderType], "invalid provider type %q", provider.ProviderType)
			assert.True(t, validAuthTypes[provider.Auth.Type], "invalid auth type %q", provider.Auth.Type)
			assert.True(t, validStructuredOutputs[provider.Capabilities.StructuredOutputs], "invalid structured output value %q", provider.Capabilities.StructuredOutputs)
			if provider.Auth.Type == "bearer" || provider.Auth.Type == "header" {
				assert.NotEmpty(t, provider.Auth.Env)
			}
			if provider.Auth.Type == "header" {
				assert.NotEmpty(t, provider.Auth.HeaderName)
			}

			if _, exists := providerIDs[provider.ID]; exists {
				t.Fatalf("duplicate provider id %q", provider.ID)
			}
			providerIDs[provider.ID] = struct{}{}

			require.Equal(t, "allowlist", provider.Models.Mode)
			require.NotEmpty(t, provider.Models.List)

			models := make(map[string]struct{}, len(provider.Models.List))
			for _, model := range provider.Models.List {
				require.NotEmpty(t, model)
				if _, exists := models[model]; exists {
					t.Fatalf("duplicate model %q for provider %q", model, provider.ID)
				}
				models[model] = struct{}{}
			}
			providerModels[provider.ID] = models

			for model := range provider.Models.Limits {
				assert.Contains(t, models, model, "limit override references unknown model")
			}
			for model := range provider.Models.Capabilities {
				assert.Contains(t, models, model, "capability override references unknown model")
				if structured := provider.Models.Capabilities[model].StructuredOutputs; structured != nil {
					assert.True(t, validStructuredOutputs[*structured], "invalid structured output override %q", *structured)
				}
			}
		})
	}

	for _, cert := range GetCertifications() {
		t.Run("certification/"+cert.Provider+"/"+cert.Model, func(t *testing.T) {
			assert.NotEqual(t, "ollama", cert.Provider, "ollama strict schema certifications are not reliable")
			models, ok := providerModels[cert.Provider]
			require.True(t, ok, "certification references unknown provider")
			assert.Contains(t, models, cert.Model, "certification references unknown model")
		})
	}
}

func TestOllamaDoesNotAdvertiseStructuredOutputs(t *testing.T) {
	for _, provider := range GetProviders() {
		if provider.ID != "ollama" {
			continue
		}

		assert.Equal(t, "none", provider.Capabilities.StructuredOutputs)
		return
	}

	t.Fatal("ollama provider not found")
}

func TestTierRegistryInvariants(t *testing.T) {
	providers := GetProviders()
	providerModels := make(map[string]map[string]struct{}, len(providers))
	for _, provider := range providers {
		models := make(map[string]struct{}, len(provider.Models.List))
		for _, model := range provider.Models.List {
			models[model] = struct{}{}
		}
		providerModels[provider.ID] = models
	}

	configs := GetAllTierConfigs()
	require.NotEmpty(t, configs)

	for tier, cfg := range configs {
		t.Run(string(tier), func(t *testing.T) {
			assert.True(t, tier.IsValid())
			assert.Equal(t, tier, cfg.Tier)
			require.NotEmpty(t, cfg.Entries)
			seen := make(map[types.TierEntry]struct{}, len(cfg.Entries))

			for _, entry := range cfg.Entries {
				assert.NotEmpty(t, entry.Provider)
				assert.NotEmpty(t, entry.Model)
				assert.Positive(t, entry.Weight)

				models, ok := providerModels[entry.Provider]
				require.True(t, ok, "tier entry references unknown provider %q", entry.Provider)
				assert.Contains(t, models, entry.Model, "tier entry references unknown model")

				if _, exists := seen[entry]; exists {
					t.Fatalf("duplicate tier entry %+v", entry)
				}
				seen[entry] = struct{}{}
			}
		})
	}
}

func TestKnownRetiredModelsAreNotConfigured(t *testing.T) {
	retired := map[string]struct{}{
		"ollama/qwen3-next:80b": {},
	}

	for _, provider := range GetProviders() {
		for _, model := range provider.Models.List {
			_, found := retired[provider.ID+"/"+model]
			assert.False(t, found, "retired model remains in provider allowlist")
		}
	}

	for _, cfg := range GetAllTierConfigs() {
		for _, entry := range cfg.Entries {
			_, found := retired[entry.Provider+"/"+entry.Model]
			assert.False(t, found, "retired model remains in tier registry")
		}
	}

	for _, cert := range GetCertifications() {
		_, found := retired[cert.Provider+"/"+cert.Model]
		assert.False(t, found, "retired model remains certified")
	}
}

func TestOCIModelConcurrencyLimit(t *testing.T) {
	const expected = 15

	for _, provider := range GetProviders() {
		if provider.ID != "oci" {
			continue
		}

		for _, model := range provider.Models.List {
			limits := provider.Models.Limits[model]
			require.NotNil(t, limits.MaxConcurrent, "OCI model %q must declare max concurrency", model)
			assert.Equal(t, expected, *limits.MaxConcurrent, "OCI model %q max concurrency", model)
		}
		return
	}

	t.Fatal("OCI provider not found")
}

func TestVerifiedOCIStrictSchemaModels(t *testing.T) {
	provider := requireProvider(t, "oci")
	verified := []string{
		"meta.llama-3.3-70b-instruct",
		"openai.gpt-oss-120b",
		"openai.gpt-oss-20b",
	}
	certified := strictSchemaCertifications()

	for _, model := range verified {
		t.Run(model, func(t *testing.T) {
			caps, ok := provider.Models.Capabilities[model]
			require.True(t, ok, "verified OCI model must declare explicit capabilities")
			require.NotNil(t, caps.StructuredOutputs)
			assert.Equal(t, "json_schema_strict", *caps.StructuredOutputs)
			assert.Contains(t, certified, "oci/"+model)
		})
	}
}

func TestOCIGeminiUsesJSONObjectOnly(t *testing.T) {
	provider := requireProvider(t, "oci")
	geminiModels := []string{
		"google.gemini-2.5-pro",
		"google.gemini-2.5-flash",
		"google.gemini-2.5-flash-lite",
	}
	certified := strictSchemaCertifications()

	for _, model := range geminiModels {
		t.Run(model, func(t *testing.T) {
			caps, ok := provider.Models.Capabilities[model]
			require.True(t, ok, "OCI Gemini model must declare explicit capabilities")
			require.NotNil(t, caps.StructuredOutputs)
			assert.Equal(t, "json_object", *caps.StructuredOutputs)
			assert.NotContains(t, certified, "oci/"+model)
		})
	}
}

func TestVerifiedKiloStrictSchemaOverrides(t *testing.T) {
	provider := requireProvider(t, "kilo")
	verified := []string{
		"stepfun/step-3.7-flash:free",
		"poolside/laguna-m.1:free",
		"openrouter/free",
	}

	for _, model := range verified {
		t.Run(model, func(t *testing.T) {
			caps, ok := provider.Models.Capabilities[model]
			require.True(t, ok, "verified Kilo model must declare explicit capabilities")
			require.NotNil(t, caps.StructuredOutputs)
			assert.Equal(t, "json_schema_strict", *caps.StructuredOutputs)
		})
	}

	if caps, ok := provider.Models.Capabilities["nvidia/nemotron-3-ultra-550b-a55b:free"]; ok && caps.StructuredOutputs != nil {
		assert.NotEqual(t, "json_schema_strict", *caps.StructuredOutputs, "timed-out Kilo Nemotron model should not be promoted")
	}
}

func TestKnownStructuredOutputFailuresStayDisabled(t *testing.T) {
	nim := requireProvider(t, "nim")
	nimCaps := nim.Models.Capabilities["mistralai/ministral-14b-instruct-2512"]
	require.NotNil(t, nimCaps.StructuredOutputs)
	assert.Equal(t, "none", *nimCaps.StructuredOutputs)

	cohere := requireProvider(t, "cohere")
	cohereCaps := cohere.Models.Capabilities["command-a-03-2025"]
	require.NotNil(t, cohereCaps.StructuredOutputs)
	assert.Equal(t, "none", *cohereCaps.StructuredOutputs)
}

func requireProvider(t *testing.T, providerID string) types.ProviderConfig {
	t.Helper()

	for _, provider := range GetProviders() {
		if provider.ID == providerID {
			return provider
		}
	}

	t.Fatalf("provider %q not found", providerID)
	return types.ProviderConfig{}
}

func strictSchemaCertifications() map[string]struct{} {
	certified := make(map[string]struct{}, len(GetCertifications()))
	for _, cert := range GetCertifications() {
		if cert.StrictSchema {
			certified[cert.Provider+"/"+cert.Model] = struct{}{}
		}
	}
	return certified
}
