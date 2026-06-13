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
			models, ok := providerModels[cert.Provider]
			require.True(t, ok, "certification references unknown provider")
			assert.Contains(t, models, cert.Model, "certification references unknown model")
		})
	}
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
