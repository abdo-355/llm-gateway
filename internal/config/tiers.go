package config

import "github.com/abdo-355/llm-gateway/internal/types"

var tierRegistry = map[types.Tier]types.TierConfig{
	types.TierDefault: {
		Tier: types.TierDefault,
		Entries: []types.TierEntry{
			{Provider: "groq", Model: "llama-3.1-8b-instant", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-small-2603", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-small-2506", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-medium-2508", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-saba-2502", Weight: 1.0},
			{Provider: "mistral", Model: "open-mistral-nemo", Weight: 1.0},
			{Provider: "gemini", Model: "gemini-2.5-flash-lite", Weight: 1.0},
			{Provider: "gemini", Model: "gemini-3.1-flash-lite-preview", Weight: 1.0},
			{Provider: "cerebras", Model: "llama3.1-8b", Weight: 1.0},
			{Provider: "nim", Model: "nvidia/llama-3.1-nemotron-70b-instruct", Weight: 1.0},
			{Provider: "nim", Model: "nvidia/llama-3.2-90b-instruct", Weight: 1.0},
			{Provider: "nim", Model: "nvidia/llama-3.2-11b-vision-instruct", Weight: 1.0},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(15000),
			MaxAttempts:  intPtr(2),
		},
	},
	types.TierPro: {
		Tier: types.TierPro,
		Entries: []types.TierEntry{
			{Provider: "groq", Model: "llama-3.3-70b-versatile", Weight: 1.0},
			{Provider: "groq", Model: "meta-llama/llama-4-scout-17b-16e-instruct", Weight: 1.0},
			{Provider: "groq", Model: "qwen/qwen3-32b", Weight: 1.0},
			{Provider: "mistral", Model: "ministral-14b-2512", Weight: 1.0},
			{Provider: "mistral", Model: "ministral-8b-2512", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-medium-2505", Weight: 1.0},
			{Provider: "mistral", Model: "magistral-small-2509", Weight: 1.0},
			{Provider: "gemini", Model: "gemini-2.5-flash", Weight: 1.0},
			{Provider: "gemini", Model: "gemini-3-flash-preview", Weight: 1.0},
			{Provider: "gemini", Model: "gemma-3-27b-it", Weight: 1.0},
			{Provider: "gemini", Model: "gemma-4-26b-a4b-it", Weight: 1.0},
			{Provider: "gemini", Model: "gemma-4-31b-it", Weight: 1.0},
			{Provider: "cerebras", Model: "qwen-3-235b-a22b-instruct-2507", Weight: 1.0},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(30000),
			MaxAttempts:  intPtr(3),
		},
	},
	types.TierMax: {
		Tier: types.TierMax,
		Entries: []types.TierEntry{
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: 1.0},
			{Provider: "groq", Model: "moonshotai/kimi-k2-instruct", Weight: 1.0},
			{Provider: "groq", Model: "moonshotai/kimi-k2-instruct-0905", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-large-2411", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-large-2512", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-medium-2509", Weight: 1.0},
			{Provider: "mistral", Model: "pixtral-large-2411", Weight: 1.0},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: 1.0},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: 1.0},
			{Provider: "nim", Model: "nvidia/nemotron-4-340b-instruct", Weight: 1.0},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(60000),
			MaxAttempts:  intPtr(3),
		},
	},
}

func GetTierConfig(tier types.Tier) *types.TierConfig {
	config, ok := tierRegistry[tier]
	if !ok {
		return nil
	}
	return &config
}

func intPtr(i int) *int {
	return &i
}

func GetAllTierConfigs() map[types.Tier]types.TierConfig {
	return tierRegistry
}
