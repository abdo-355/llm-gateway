package config

import "github.com/abdo-355/llm-gateway/internal/types"

var tierRegistry = map[types.Tier]types.TierConfig{
	types.TierDefault: {
		Tier: types.TierDefault,
		Entries: []types.TierEntry{
			{Provider: "cerebras", Model: "qwen-3-235b-a22b-instruct-2507", Weight: 1.0},
		{Provider: "mistral", Model: "mistral-medium-3.5", Weight: 0.9},
		{Provider: "nim", Model: "qwen/qwen3-next-80b-a3b-thinking", Weight: 0.5},
		{Provider: "nim", Model: "mistralai/mistral-large-3-675b-instruct-2512", Weight: 0.3},
		{Provider: "nim", Model: "qwen/qwen3-next-80b-a3b-instruct", Weight: 0.9},

		{Provider: "mistral", Model: "mistral-small-2603", Weight: 0.9},

			{Provider: "ollama", Model: "qwen3-next:80b", Weight: 0.8},
			{Provider: "ollama", Model: "devstral-small-2:24b", Weight: 0.8},
			{Provider: "ollama", Model: "gemma4:31b", Weight: 0.8},
			{Provider: "ollama", Model: "gemma3:27b", Weight: 0.8},
			{Provider: "ollama", Model: "gemma3:12b", Weight: 0.8},
			{Provider: "ollama", Model: "nemotron-3-nano:30b", Weight: 0.8},
			{Provider: "ollama", Model: "gpt-oss:20b", Weight: 0.8},

			{Provider: "kilo", Model: "kilo-auto/free", Weight: 0.8},
			{Provider: "kilo", Model: "stepfun/step-3.5-flash:free", Weight: 0.8},
			{Provider: "kilo", Model: "inclusionai/ling-2.6-flash:free", Weight: 0.8},

			{Provider: "ollama", Model: "gemma3:4b", Weight: 0.7},
			{Provider: "ollama", Model: "ministral-3:14b", Weight: 0.7},
			{Provider: "ollama", Model: "ministral-3:8b", Weight: 0.7},
			{Provider: "ollama", Model: "ministral-3:3b", Weight: 0.7},
			{Provider: "ollama", Model: "rnj-1:8b", Weight: 0.7},

			{Provider: "kilo", Model: "openrouter/free", Weight: 0.7},
			{Provider: "kilo", Model: "tencent/hy3-preview:free", Weight: 0.7},
			{Provider: "cloudflare", Model: "@cf/openai/gpt-oss-20b", Weight: 0.8},
			{Provider: "cloudflare", Model: "@cf/qwen/qwen3-30b-a3b-fp8", Weight: 0.8},
			{Provider: "cloudflare", Model: "@cf/zai-org/glm-4.7-flash", Weight: 0.8},

			{Provider: "cloudflare", Model: "@cf/qwen/qwen2.5-coder-32b-instruct", Weight: 0.7},
			{Provider: "cloudflare", Model: "@cf/qwen/qwq-32b", Weight: 0.7},
			{Provider: "cloudflare", Model: "@cf/deepseek-ai/deepseek-r1-distill-qwen-32b", Weight: 0.7},
			{Provider: "cloudflare", Model: "@cf/meta/llama-4-scout-17b-16e-instruct", Weight: 0.7},
			{Provider: "cloudflare", Model: "@cf/mistralai/mistral-small-3.1-24b-instruct", Weight: 0.7},
			{Provider: "cloudflare", Model: "@cf/google/gemma-3-12b-it", Weight: 0.7},
			{Provider: "cloudflare", Model: "@cf/meta/llama-3.3-70b-instruct-fp8-fast", Weight: 0.7},

			{Provider: "cloudflare", Model: "@cf/ibm-granite/granite-4.0-h-micro", Weight: 0.6},
			{Provider: "cloudflare", Model: "@cf/meta/llama-3.2-3b-instruct", Weight: 0.5},
			{Provider: "cloudflare", Model: "@cf/meta/llama-3.2-1b-instruct", Weight: 0.5},

			{Provider: "opencode", Model: "big-pickle", Weight: 0.6},
			{Provider: "opencode", Model: "deepseek-v4-flash-free", Weight: 0.6},
			{Provider: "opencode", Model: "ring-2.6-1t-free", Weight: 0.6},

			{Provider: "zai", Model: "glm-4.7-flash", Weight: 0.8},
			{Provider: "zai", Model: "glm-4.5-flash", Weight: 0.8},
			{Provider: "zai", Model: "glm-4.6v-flash", Weight: 0.7},

			{Provider: "llm7", Model: "fast", Weight: 0.8},
			{Provider: "llm7", Model: "default", Weight: 0.8},

			{Provider: "cohere", Model: "command-r7b-12-2024", Weight: 0.5},

		},
		SLO: &types.TierSLO{
			MaxLatencyMs: intPtr(15000),
			MaxAttempts:  intPtr(2),
		},
	},
	types.TierPro: {
		Tier: types.TierPro,
		Entries: []types.TierEntry{
			{Provider: "nim", Model: "minimaxai/minimax-m2.5", Weight: 1.0},
			{Provider: "mistral", Model: "mistral-medium-3.5", Weight: 1.0},
			{Provider: "nim", Model: "qwen/qwen3.5-122b-a10b", Weight: 0.9},
			{Provider: "nim", Model: "stepfun-ai/step-3.5-flash", Weight: 0.9},
			{Provider: "nim", Model: "z-ai/glm4.7", Weight: 0.9},
			{Provider: "nim", Model: "mistralai/mistral-medium-3.5-128b", Weight: 0.9},
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: 0.8},
			{Provider: "nim", Model: "deepseek-ai/deepseek-v3.1-terminus", Weight: 0.8},
		{Provider: "nim", Model: "openai/gpt-oss-120b", Weight: 0.8},

		{Provider: "ollama", Model: "deepseek-v3.2", Weight: 0.9},
			{Provider: "ollama", Model: "qwen3-coder:480b", Weight: 0.9},
			{Provider: "ollama", Model: "qwen3-coder-next", Weight: 0.9},
			{Provider: "ollama", Model: "devstral-2:123b", Weight: 0.9},
			{Provider: "ollama", Model: "minimax-m2.5", Weight: 0.9},
			{Provider: "ollama", Model: "nemotron-3-super", Weight: 0.9},
			{Provider: "ollama", Model: "cogito-2.1:671b", Weight: 0.9},

			{Provider: "ollama", Model: "deepseek-v3.1:671b", Weight: 0.8},
			{Provider: "ollama", Model: "gpt-oss:120b", Weight: 0.8},
			{Provider: "ollama", Model: "glm-4.7", Weight: 0.8},
			{Provider: "ollama", Model: "glm-4.6", Weight: 0.8},
			{Provider: "ollama", Model: "minimax-m2.1", Weight: 0.8},
			{Provider: "ollama", Model: "minimax-m2", Weight: 0.8},

			{Provider: "kilo", Model: "nvidia/nemotron-3-super-120b-a12b:free", Weight: 0.8},
			{Provider: "kilo", Model: "inclusionai/ling-2.6-1t:free", Weight: 0.8},
			{Provider: "cloudflare", Model: "@cf/google/gemma-4-26b-a4b-it", Weight: 0.8},
			{Provider: "cloudflare", Model: "@cf/openai/gpt-oss-120b", Weight: 0.8},
			{Provider: "cloudflare", Model: "@cf/nvidia/nemotron-3-120b-a12b", Weight: 0.8},

			{Provider: "opencode", Model: "minimax-m2.5-free", Weight: 0.7},
			{Provider: "opencode", Model: "nemotron-3-super-free", Weight: 0.7},

			{Provider: "cohere", Model: "command-a-03-2025", Weight: 0.6},


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
			{Provider: "cloudflare", Model: "@cf/moonshotai/kimi-k2.6", Weight: 0.9},
			{Provider: "nim", Model: "minimaxai/minimax-m2.7", Weight: 0.9},
			{Provider: "nim", Model: "moonshotai/kimi-k2.6", Weight: 0.9},
			{Provider: "cloudflare", Model: "@cf/moonshotai/kimi-k2.5", Weight: 0.8},
			{Provider: "nim", Model: "z-ai/glm5", Weight: 0.9},
			{Provider: "nim", Model: "qwen/qwen3.5-397b-a17b", Weight: 0.8},
			{Provider: "mistral", Model: "mistral-medium-3.5", Weight: 0.85},
			{Provider: "nim", Model: "mistralai/mistral-medium-3.5-128b", Weight: 0.75},

	{Provider: "ollama", Model: "minimax-m2.7", Weight: 0.9},

	{Provider: "ollama", Model: "mistral-large-3:675b", Weight: 0.8},

		{Provider: "cohere", Model: "command-a-03-2025", Weight: 0.6},
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
