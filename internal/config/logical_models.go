package config

import "github.com/abdo-355/llm-gateway/internal/types"

// LogicalModelRegistry maps logical model IDs to their configurations
var LogicalModelRegistry = map[string]types.LogicalModelConfig{
	"chat-lite":    getChatLiteConfig(),
	"chat-pro":     getChatProConfig(),
	"chat-max":     getChatMaxConfig(),
	"analysis-pro": getAnalysisProConfig(),
	"json-fast":    getJsonFastConfig(),
	"json-safe":    getJsonSafeConfig(),
	"code-fast":    getCodeFastConfig(),
	"code-pro":     getCodeProConfig(),
	"tools-pro":      getToolsProConfig(),
	"reasoning-max":  getReasoningMaxConfig(),
}

// GetLogicalModel returns a logical model configuration by ID
func GetLogicalModel(id string) *types.LogicalModelConfig {
	if config, ok := LogicalModelRegistry[id]; ok {
		return &config
	}
	return nil
}

func getChatLiteConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "chat-lite",
		TaskType: "chat",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "groq", Model: "llama-3.1-8b-instant", Weight: weightHigh},
			{Provider: "groq", Model: "qwen/qwen3-32b", Weight: weightHigh},
			{Provider: "mistral", Model: "ministral-8b-2410", Weight: weightHigh},
			{Provider: "mistral", Model: "ministral-3b-2410", Weight: weightMedium},
			{Provider: "mistral", Model: "open-mistral-nemo", Weight: weightMedium},
			{Provider: "mistral", Model: "open-mistral-7b", Weight: weightMedium},
			{Provider: "gemini", Model: "google/gemini-3.1-flash-lite-preview", Weight: weightMedium},
		},
	}
}

func getChatProConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightMedium := 0.8
	weightOverflow := 0.3

	return types.LogicalModelConfig{
		ID:       "chat-pro",
		TaskType: "chat",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "groq", Model: "llama-3.3-70b-versatile", Weight: weightHigh},
			{Provider: "groq", Model: "moonshotai/kimi-k2-instruct", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-medium", Weight: weightMedium},
			{Provider: "cerebras", Model: "llama3.1-8b", Weight: weightMedium},
			{Provider: "groq", Model: "llama-3.1-8b-instant", Weight: weightMedium},
			{Provider: "gemini", Model: "google/gemini-3-flash-preview", Weight: weightOverflow},
		},
	}
}

func getChatMaxConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "chat-max",
		TaskType: "chat",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: weightHigh},
			{Provider: "cerebras", Model: "gpt-oss-120b", Weight: weightHigh},
			{Provider: "mistral", Model: "open-mixtral-8x22b", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightMedium},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightMedium},
		},
	}
}

func getAnalysisProConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightMedium := 0.8
	weightOverflow := 0.3

	return types.LogicalModelConfig{
		ID:       "analysis-pro",
		TaskType: "analysis",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: weightHigh},
			{Provider: "groq", Model: "meta-llama/llama-4-maverick-17b-128e-instruct", Weight: weightHigh},
			{Provider: "groq", Model: "moonshotai/kimi-k2-instruct", Weight: weightHigh},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightMedium},
			{Provider: "cerebras", Model: "gpt-oss-120b", Weight: weightMedium},
			{Provider: "gemini", Model: "google/gemini-2.5-flash", Weight: weightOverflow},
		},
	}
}

func getJsonFastConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "json-fast",
		TaskType: "json_extraction",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "mistral", Model: "ministral-8b-2410", Weight: weightHigh},
			{Provider: "mistral", Model: "ministral-3b-2410", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-medium", Weight: weightHigh},
			{Provider: "groq", Model: "llama-3.1-8b-instant", Weight: weightMedium},
			{Provider: "groq", Model: "qwen/qwen3-32b", Weight: weightMedium},
			{Provider: "gemini", Model: "google/gemini-3.1-flash-lite-preview", Weight: weightMedium},
		},
		RequireStrictJSON: boolPtr(true),
	}
}

func getJsonSafeConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "json-safe",
		TaskType: "json_extraction",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightHigh},
			{Provider: "vertex", Model: "google/gemini-3-flash-preview", Weight: weightHigh},
			{Provider: "mistral", Model: "ministral-8b-2410", Weight: weightMedium},
			{Provider: "mistral", Model: "ministral-3b-2410", Weight: weightMedium},
		},
		RequireStrictJSON: boolPtr(true),
	}
}

func getCodeFastConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "code-fast",
		TaskType: "code",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "mistral", Model: "codestral-2501", Weight: weightHigh},
			{Provider: "mistral", Model: "codestral-mamba-2407", Weight: weightHigh},
			{Provider: "groq", Model: "meta-llama/llama-4-scout-17b-16e-instruct", Weight: weightHigh},
			{Provider: "groq", Model: "qwen/qwen3-32b", Weight: weightMedium},
			{Provider: "mistral", Model: "ministral-8b-2410", Weight: weightMedium},
		},
	}
}

func getCodeProConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightMedium := 0.8

	return types.LogicalModelConfig{
		ID:       "code-pro",
		TaskType: "code",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "mistral", Model: "codestral-2501", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "groq", Model: "openai/gpt-oss-120b", Weight: weightHigh},
			{Provider: "cerebras", Model: "gpt-oss-120b", Weight: weightMedium},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightMedium},
		},
	}
}

func getToolsProConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightMedium := 0.8
	weightOverflow := 0.3

	return types.LogicalModelConfig{
		ID:       "tools-pro",
		TaskType: "tool_orchestration",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "mistral", Model: "mistral-large-2411", Weight: weightHigh},
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightHigh},
			{Provider: "mistral", Model: "mistral-medium", Weight: weightMedium},
			{Provider: "groq", Model: "meta-llama/llama-4-maverick-17b-128e-instruct", Weight: weightMedium},
			{Provider: "cerebras", Model: "gpt-oss-120b", Weight: weightMedium},
			{Provider: "gemini", Model: "google/gemini-3-flash-preview", Weight: weightOverflow},
			{Provider: "gemini", Model: "google/gemini-2.5-flash", Weight: weightOverflow},
		},
		RequireTools: boolPtr(true),
	}
}

func getReasoningMaxConfig() types.LogicalModelConfig {
	weightHigh := 1.0
	weightLow := 0.7

	return types.LogicalModelConfig{
		ID:       "reasoning-max",
		TaskType: "reasoning",
		Candidates: []types.LogicalModelCandidate{
			{Provider: "vertex", Model: "google/gemini-3.1-pro-preview", Weight: weightHigh},
			{Provider: "groq", Model: "moonshotai/kimi-k2-instruct", Weight: weightHigh},
			{Provider: "cerebras", Model: "gpt-oss-120b", Weight: weightLow},
		},
	}
}

func boolPtr(b bool) *bool {
	return &b
}
