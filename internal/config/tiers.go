package config

import "github.com/abdo-355/llm-gateway/internal/types"

// Weights are coarse routing priors, not precise rankings. Bands:
//
//	0.94–0.98 exceptional/near-king   0.86–0.92 excellent
//	0.78–0.84 strong                  0.68–0.76 competent fallback
//	0.56–0.66 deep fallback           0.40–0.54 untrusted/free-pool emergency
//
// stealth/ox-alpha and x-preview-f-free intentionally share the top weight in
// every tier; live health/success/concurrency scoring breaks their ties.
var tierRegistry = map[types.Tier]types.TierConfig{
	types.TierDefault: {
		Tier: types.TierDefault,
		Entries: []types.TierEntry{
			{Provider: "openrouter-alpha", Model: "stealth/ox-alpha", Weight: 1.00},
			{Provider: "opencode", Model: "x-preview-f-free", Weight: 1.00},
			{Provider: "groq", Model: "qwen/qwen3.6-27b", Weight: 0.94},
			{Provider: "gemini", Model: "gemini-3.5-flash-lite", Weight: 0.92},
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: 0.88},
			{Provider: "opencode", Model: "muse-spark-1.2-contributor-free", Weight: 0.86},
			{Provider: "opencode", Model: "nemotron-3-ultra-free", Weight: 0.84},
			{Provider: "gemini", Model: "gemini-3.7-flash", Weight: 0.82},
			{Provider: "gemini", Model: "gemma-4-31b-it", Weight: 0.82},
			{Provider: "gemini", Model: "gemini-3.1-flash-lite", Weight: 0.80},
			{Provider: "gemini", Model: "gemma-4-26b-a4b-it", Weight: 0.78},
			{Provider: "gemini", Model: "gemini-3.6-flash", Weight: 0.78},
			{Provider: "opencode", Model: "mimo-v2.5-free", Weight: 0.76},
			{Provider: "gemini", Model: "gemini-2.5-flash", Weight: 0.72},
			{Provider: "groq", Model: "openai/gpt-oss-20b", Weight: 0.70},
			{Provider: "opencode", Model: "nemotron-3.5-lightning-free", Weight: 0.68},
			{Provider: "gemini", Model: "gemini-3-flash-preview", Weight: 0.66},
			{Provider: "oci", Model: "meta.llama-3.3-70b-instruct", Weight: 0.66},
			{Provider: "gemini", Model: "gemini-2.5-flash-lite", Weight: 0.62},
			{Provider: "opencode", Model: "hy3-free", Weight: 0.60},
			{Provider: "openrouter", Model: "nvidia/nemotron-3-super-120b-a12b:free", Weight: 0.48},
			{Provider: "openrouter", Model: "nvidia/nemotron-3-ultra-550b-a55b:free", Weight: 0.47},
			{Provider: "openrouter", Model: "z-ai/glm-5.2:free", Weight: 0.46},
			{Provider: "openrouter", Model: "google/gemma-4-31b-it:free", Weight: 0.45},
			{Provider: "kilo", Model: "poolside/laguna-s-2.1:free", Weight: 0.44},
			{Provider: "openrouter", Model: "nvidia/nemotron-3.5-lightning:free", Weight: 0.44},
			{Provider: "kilo", Model: "thinkingmachines/inkling:free", Weight: 0.43},
			{Provider: "openrouter", Model: "cohere/north-mini-code:free", Weight: 0.42},
			{Provider: "openrouter", Model: "poolside/laguna-s-2.1:free", Weight: 0.42},
			{Provider: "openrouter", Model: "thinkingmachines/inkling:free", Weight: 0.41},
			{Provider: "openrouter", Model: "google/gemma-4-26b-a4b-it:free", Weight: 0.41},
			{Provider: "openrouter", Model: "dots-studio/dots-3-note-preview:free", Weight: 0.40},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(30000),
		},
	},
	types.TierPro: {
		Tier: types.TierPro,
		Entries: []types.TierEntry{
			{Provider: "openrouter-alpha", Model: "stealth/ox-alpha", Weight: 1.00},
			{Provider: "opencode", Model: "x-preview-f-free", Weight: 1.00},
			{Provider: "gemini", Model: "gemini-3.7-flash", Weight: 0.98},
			{Provider: "opencode", Model: "muse-spark-1.2-contributor-free", Weight: 0.96},
			{Provider: "opencode", Model: "nemotron-3-ultra-free", Weight: 0.94},
			{Provider: "gemini", Model: "gemini-3.6-flash", Weight: 0.92},
			{Provider: "groq", Model: "qwen/qwen3.6-27b", Weight: 0.90},
			{Provider: "nim", Model: "minimaxai/minimax-m2.7", Weight: 0.82},
			{Provider: "gemini", Model: "gemini-3.5-flash", Weight: 0.86},
			{Provider: "nim", Model: "qwen/qwen3.5-397b-a17b", Weight: 0.78},
			{Provider: "gemini", Model: "gemma-4-31b-it", Weight: 0.78},
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: 0.78},
			{Provider: "nim", Model: "openai/gpt-oss-120b", Weight: 0.77},
			{Provider: "ollama", Model: "gpt-oss:120b", Weight: 0.76},
			{Provider: "nim", Model: "qwen/qwen3.5-122b-a10b", Weight: 0.74},
			{Provider: "ollama", Model: "qwen3-coder:480b", Weight: 0.68},
			{Provider: "opencode", Model: "hy3-free", Weight: 0.60},
			{Provider: "kilo", Model: "poolside/laguna-s-2.1:free", Weight: 0.56},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(30000),
		},
	},
	types.TierMax: {
		Tier: types.TierMax,
		Entries: []types.TierEntry{
			{Provider: "openrouter-alpha", Model: "stealth/ox-alpha", Weight: 1.00},
			{Provider: "opencode", Model: "x-preview-f-free", Weight: 1.00},
			{Provider: "gemini", Model: "gemini-3.7-flash", Weight: 0.98},
			{Provider: "opencode", Model: "muse-spark-1.2-contributor-free", Weight: 0.97},
			{Provider: "opencode", Model: "nemotron-3-ultra-free", Weight: 0.95},
			{Provider: "nim", Model: "qwen/qwen3.5-397b-a17b", Weight: 0.83},
			{Provider: "groq", Model: "qwen/qwen3.6-27b", Weight: 0.92},
			{Provider: "gemini", Model: "gemini-3.6-flash", Weight: 0.89},
			{Provider: "nim", Model: "qwen/qwen3.5-122b-a10b", Weight: 0.77},
			{Provider: "gemini", Model: "gemini-3.5-flash", Weight: 0.84},
			{Provider: "nim", Model: "minimaxai/minimax-m2.7", Weight: 0.72},
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: 0.78},
			{Provider: "nim", Model: "openai/gpt-oss-120b", Weight: 0.78},
			{Provider: "ollama", Model: "gpt-oss:120b", Weight: 0.78},
			{Provider: "gemini", Model: "gemma-4-31b-it", Weight: 0.76},
			{Provider: "ollama", Model: "qwen3-coder:480b", Weight: 0.68},
		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(60000),
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
