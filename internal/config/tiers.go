package config

import "github.com/abdo-355/llm-gateway/internal/types"

var tierRegistry = map[types.Tier]types.TierConfig{
	types.TierDefault: {
		Tier: types.TierDefault,
		Entries: []types.TierEntry{
			{Provider: "cerebras", Model: "qwen-3-235b-a22b-instruct-2507", Weight: 1.0},
			{Provider: "gemini", Model: "gemini-2.5-flash", Weight: 1.0},
			{Provider: "mistral", Model: "magistral-medium-2509", Weight: 1.0},
			{Provider: "nim", Model: "qwen/qwen3-next-80b-a3b-thinking", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-large-2512", Weight: 0.9},
			{Provider: "nim", Model: "mistralai/devstral-2-123b-instruct-2512", Weight: 0.9},
			{Provider: "nim", Model: "mistralai/mistral-large-3-675b-instruct-2512", Weight: 0.9},
			{Provider: "nim", Model: "qwen/qwen3-next-80b-a3b-instruct", Weight: 0.9},

			{Provider: "mistral", Model: "mistral-medium-2508", Weight: 0.8},
			{Provider: "mistral", Model: "mistral-small-2603", Weight: 0.8},
			{Provider: "nim", Model: "mistralai/mistral-medium-3-instruct", Weight: 0.8},
			{Provider: "nim", Model: "moonshotai/kimi-k2-instruct", Weight: 0.8},
			{Provider: "nim", Model: "moonshotai/kimi-k2-instruct-0905", Weight: 0.8},
			{Provider: "nim", Model: "moonshotai/kimi-k2-thinking", Weight: 0.8},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(15000),
			MaxAttempts:  intPtr(2),
		},
	},
	types.TierPro: {
		Tier: types.TierPro,
		Entries: []types.TierEntry{
			{Provider: "nim", Model: "deepseek-ai/deepseek-v3.2", Weight: 1.0},
			{Provider: "nim", Model: "minimaxai/minimax-m2.5", Weight: 1.0},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: 1.0},
			{Provider: "gemini", Model: "gemma-4-26b-a4b-it", Weight: 0.9},
			{Provider: "nim", Model: "google/gemma-4-31b-it", Weight: 0.9},
			{Provider: "nim", Model: "qwen/qwen3.5-122b-a10b", Weight: 0.9},
			{Provider: "nim", Model: "stepfun-ai/step-3.5-flash", Weight: 0.9},
			{Provider: "nim", Model: "z-ai/glm4.7", Weight: 0.9},
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: 0.8},
			{Provider: "nim", Model: "deepseek-ai/deepseek-v3.1-terminus", Weight: 0.8},
			{Provider: "nim", Model: "openai/gpt-oss-120b", Weight: 0.8},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(30000),
			MaxAttempts:  intPtr(3),
		},
	},
	types.TierMax: {
		Tier: types.TierMax,
		Entries: []types.TierEntry{
			{Provider: "nim", Model: "z-ai/glm-5.1", Weight: 1.0},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: 1.0},
			{Provider: "nim", Model: "minimaxai/minimax-m2.7", Weight: 0.9},
			{Provider: "nim", Model: "moonshotai/kimi-k2.5", Weight: 0.9},
			{Provider: "nim", Model: "z-ai/glm5", Weight: 0.9},
			{Provider: "nim", Model: "qwen/qwen3.5-397b-a17b", Weight: 0.8},
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
