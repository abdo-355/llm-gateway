package config

import "github.com/abdo-355/llm-gateway/internal/types"

// LogicalModelRegistry maps logical model IDs to their configurations
var LogicalModelRegistry = map[string]types.LogicalModelConfig{
	"chat-lite":     getChatLiteConfig(),
	"chat-pro":      getChatProConfig(),
	"chat-max":      getChatMaxConfig(),
	"analysis-pro":  getAnalysisProConfig(),
	"json-fast":     getJsonFastConfig(),
	"json-safe":     getJsonSafeConfig(),
	"code-fast":     getCodeFastConfig(),
	"code-pro":      getCodeProConfig(),
	"tools-pro":     getToolsProConfig(),
	"reasoning-max": getReasoningMaxConfig(),
}

// GetLogicalModel returns a logical model configuration by ID
func GetLogicalModel(id string) *types.LogicalModelConfig {
	if config, ok := LogicalModelRegistry[id]; ok {
		return &config
	}
	return nil
}

func getChatLiteConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "chat-lite",
		TaskType: "chat",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexSecondary},
			{Provider: "groq", Model: "llama-3.1-8b-instant", Weight: weightHigh},
			{Provider: "groq", Model: "qwen/qwen3-32b", Weight: weightHigh},
			{Provider: "mistral", Model: "ministral-8b-2512", Weight: weightHigh},
			{Provider: "mistral", Model: "ministral-3b-2512", Weight: weightMedium},
			{Provider: "cerebras", Model: "llama3.1-8b", Weight: weightMedium},
			{Provider: "mistral", Model: "open-mistral-nemo", Weight: weightMedium},
			{Provider: "mistral", Model: "mistral-small-2506", Weight: weightMedium},
		}, geminiLastResortCandidates()...),
	}
}

func getChatProConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "chat-pro",
		TaskType: "chat",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexSecondary},
			{Provider: "groq", Model: "llama-3.3-70b-versatile", Weight: weightHigh},
			{Provider: "groq", Model: "moonshotai/kimi-k2-instruct", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-medium-2508", Weight: weightMedium},
			{Provider: "cerebras", Model: "llama3.1-8b", Weight: weightMedium},
			{Provider: "groq", Model: "llama-3.1-8b-instant", Weight: weightMedium},
		}, geminiLastResortCandidates()...),
	}
}

func getChatMaxConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "chat-max",
		TaskType: "chat",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexSecondary},
			{Provider: "cerebras", Model: "qwen-3-235b-a22b-instruct-2507", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-large-2512", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "groq", Model: "moonshotai/kimi-k2-instruct", Weight: weightMedium},
		}, geminiLastResortCandidates()...),
	}
}

func getAnalysisProConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "analysis-pro",
		TaskType: "analysis",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexSecondary},
			{Provider: "groq", Model: "groq/compound", Weight: weightHigh},
			{Provider: "groq", Model: "moonshotai/kimi-k2-instruct", Weight: weightHigh},
			{Provider: "cerebras", Model: "qwen-3-235b-a22b-instruct-2507", Weight: weightMedium},
		}, geminiLastResortCandidates()...),
	}
}

func getJsonFastConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "json-fast",
		TaskType: "json_extraction",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexSecondary},
			{Provider: "mistral", Model: "ministral-8b-2512", Weight: weightHigh},
			{Provider: "mistral", Model: "ministral-3b-2512", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-small-2603", Weight: weightHigh},
			{Provider: "groq", Model: "llama-3.1-8b-instant", Weight: weightMedium},
		}, geminiLastResortCandidates()...),
		RequireStrictJSON: boolPtr(true),
	}
}

func getJsonSafeConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "json-safe",
		TaskType: "json_extraction",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexSecondary},
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "mistral", Model: "open-mistral-nemo", Weight: weightMedium},
			{Provider: "mistral", Model: "mistral-large-2512", Weight: weightMedium},
		}, geminiLastResortCandidates()...),
		RequireStrictJSON: boolPtr(true),
	}
}

func getCodeFastConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "code-fast",
		TaskType: "code",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexSecondary},
			{Provider: "groq", Model: "meta-llama/llama-4-scout-17b-16e-instruct", Weight: weightHigh},
			{Provider: "cerebras", Model: "qwen-3-235b-a22b-instruct-2507", Weight: weightHigh},
			{Provider: "groq", Model: "qwen/qwen3-32b", Weight: weightMedium},
			{Provider: "mistral", Model: "ministral-8b-2512", Weight: weightMedium},
		}, geminiLastResortCandidates()...),
	}
}

func getCodeProConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "code-pro",
		TaskType: "code",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexSecondary},
			{Provider: "mistral", Model: "mistral-large-2512", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "cerebras", Model: "qwen-3-235b-a22b-instruct-2507", Weight: weightMedium},
			{Provider: "groq", Model: "meta-llama/llama-4-scout-17b-16e-instruct", Weight: weightMedium},
		}, geminiLastResortCandidates()...),
	}
}

func getToolsProConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightMedium := 0.8
	weightOverflow := 0.3

	return types.LogicalModelConfig{
		ID:       "tools-pro",
		TaskType: "tool_orchestration",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexSecondary},
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-medium-2508", Weight: weightMedium},
			{Provider: "groq", Model: "llama-3.3-70b-versatile", Weight: weightOverflow},
		}, geminiLastResortCandidates()...),
		RequireTools: boolPtr(true),
	}
}

func getReasoningMaxConfig() types.LogicalModelConfig {
	weightVertexPrimary := 1.2
	weightVertexSecondary := 1.1
	weightHigh := 1.0
	weightLow := 0.7

	return types.LogicalModelConfig{
		ID:       "reasoning-max",
		TaskType: "reasoning",
		Candidates: append([]types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightVertexPrimary},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightVertexSecondary},
			{Provider: "groq", Model: "moonshotai/kimi-k2-instruct", Weight: weightHigh},
			{Provider: "cerebras", Model: "qwen-3-235b-a22b-instruct-2507", Weight: weightLow},
		}, geminiLastResortCandidates()...),
	}
}

func geminiLastResortCandidates() []types.LogicalModelCandidate {
	return []types.LogicalModelCandidate{
		{Provider: "gemini", Model: "gemini-2.5-flash", Weight: 0.2},
		{Provider: "gemini", Model: "gemini-2.5-flash-lite", Weight: 0.19},
		{Provider: "gemini", Model: "gemini-3-flash-preview", Weight: 0.18},
		{Provider: "gemini", Model: "gemini-3.1-flash-lite-preview", Weight: 0.17},
		{Provider: "gemini", Model: "gemma-4-31b-it", Weight: 0.16},
		{Provider: "gemini", Model: "gemma-4-26b-a4b-it", Weight: 0.15},
		{Provider: "gemini", Model: "gemma-3-27b-it", Weight: 0.14},
		{Provider: "gemini", Model: "gemma-3-12b-it", Weight: 0.13},
		{Provider: "gemini", Model: "gemma-3-4b-it", Weight: 0.12},
		{Provider: "gemini", Model: "gemma-3-1b-it", Weight: 0.11},
	}
}

func boolPtr(b bool) *bool {
	return &b
}
