package config

import "github.com/abdo-355/llm-gateway/internal/types"

var tierRegistry = map[types.Tier]types.TierConfig{
	types.TierDefault: {
		Tier: types.TierDefault,
		Entries: []types.TierEntry{
			{Provider: "cerebras", Model: "gpt-oss-120b", Weight: 0.96},
			{Provider: "oci", Model: "google.gemini-2.5-flash", Weight: 0.92},
			{Provider: "mistral", Model: "mistral-small-2506", Weight: 0.90},
			{Provider: "nim", Model: "qwen/qwen3-next-80b-a3b-instruct", Weight: 0.90},
			{Provider: "oci", Model: "openai.gpt-oss-20b", Weight: 0.88},
			{Provider: "groq", Model: "llama-3.3-70b-versatile", Weight: 0.84},
			{Provider: "oci", Model: "google.gemini-2.5-flash-lite", Weight: 0.82},
			{Provider: "groq", Model: "meta-llama/llama-4-scout-17b-16e-instruct", Weight: 0.82},
			{Provider: "mistral", Model: "mistral-small-2603", Weight: 0.80},
			{Provider: "nim", Model: "mistralai/mistral-nemotron", Weight: 0.80},
			{Provider: "oci", Model: "meta.llama-3.3-70b-instruct", Weight: 0.78},
			{Provider: "mistral", Model: "mistral-medium-3.5", Weight: 0.76},
			{Provider: "nim", Model: "moonshotai/kimi-k2.6", Weight: 0.74},
			{Provider: "groq", Model: "qwen/qwen3-32b", Weight: 0.70},
			{Provider: "groq", Model: "openai/gpt-oss-20b", Weight: 0.68},
			{Provider: "ollama", Model: "qwen3-next:80b", Weight: 0.66},
			{Provider: "ollama", Model: "llama3.3:70b", Weight: 0.66},
			{Provider: "kilo", Model: "kilo-auto/free", Weight: 0.62},
			{Provider: "llm7", Model: "default", Weight: 0.60},
			{Provider: "llm7", Model: "fast", Weight: 0.58},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(15000),
			MaxAttempts:  intPtr(2),
		},
	},
	types.TierPro: {
		Tier: types.TierPro,
		Entries: []types.TierEntry{
			{Provider: "mistral", Model: "mistral-medium-2505", Weight: 1.00},
			{Provider: "mistral", Model: "mistral-medium-2508", Weight: 0.96},
			{Provider: "cerebras", Model: "zai-glm-4.7", Weight: 0.94},
			{Provider: "cerebras", Model: "gpt-oss-120b", Weight: 0.92},
			{Provider: "nim", Model: "minimaxai/minimax-m2.7", Weight: 0.92},
			{Provider: "oci", Model: "openai.gpt-oss-120b", Weight: 0.90},
			{Provider: "nim", Model: "qwen/qwen3.5-122b-a10b", Weight: 0.90},
			{Provider: "nim", Model: "moonshotai/kimi-k2.6", Weight: 0.90},
			{Provider: "oci", Model: "google.gemini-2.5-pro", Weight: 0.88},
			{Provider: "nim", Model: "qwen/qwen3.5-397b-a17b", Weight: 0.86},
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: 0.82},
			{Provider: "oci", Model: "meta.llama-3.3-70b-instruct", Weight: 0.82},
			{Provider: "groq", Model: "meta-llama/llama-4-scout-17b-16e-instruct", Weight: 0.80},
			{Provider: "nim", Model: "openai/gpt-oss-120b", Weight: 0.78},
			{Provider: "ollama", Model: "deepseek-v3.2", Weight: 0.76},
			{Provider: "ollama", Model: "qwen3-coder:480b", Weight: 0.76},
			{Provider: "ollama", Model: "gpt-oss:120b", Weight: 0.72},
			{Provider: "kilo", Model: "nvidia/nemotron-3-super-120b-a12b:free", Weight: 0.68},
			{Provider: "cohere", Model: "command-a-03-2025", Weight: 0.60},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(30000),
			MaxAttempts:  intPtr(3),
		},
	},
	types.TierMax: {
		Tier: types.TierMax,
		Entries: []types.TierEntry{
			{Provider: "nim", Model: "moonshotai/kimi-k2.6", Weight: 1.00},
			{Provider: "nim", Model: "minimaxai/minimax-m2.7", Weight: 0.96},
			{Provider: "nim", Model: "qwen/qwen3.5-397b-a17b", Weight: 0.92},
			{Provider: "cerebras", Model: "zai-glm-4.7", Weight: 0.90},
			{Provider: "nim", Model: "qwen/qwen3.5-122b-a10b", Weight: 0.88},
			{Provider: "mistral", Model: "magistral-medium-2509", Weight: 0.84},
			{Provider: "mistral", Model: "mistral-medium-2508", Weight: 0.80},
			{Provider: "oci", Model: "openai.gpt-oss-120b", Weight: 0.78},
			{Provider: "mistral", Model: "mistral-medium-2505", Weight: 0.78},
			{Provider: "oci", Model: "google.gemini-2.5-pro", Weight: 0.76},
			{Provider: "ollama", Model: "minimax-m2.7", Weight: 0.74},
			{Provider: "ollama", Model: "mistral-large-3:675b", Weight: 0.72},
			{Provider: "cohere", Model: "command-a-03-2025", Weight: 0.60},
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
