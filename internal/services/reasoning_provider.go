package services

import (
	"strings"

	"github.com/abdo-355/llm-gateway/internal/types"
)

// applyReasoningForProvider converts the request's normalized reasoning params
// into the target provider's native wire form. Following the hybrid policy,
// params are silently dropped when the target lacks support — routing already
// filtered candidates only when router.requirements.reasoning=required.
//
// Providers that build fully native bodies (cloudflare workers-ai, cohere v1)
// never read these fields, so dropping happens implicitly there.
func applyReasoningForProvider(req *types.ChatCompletionRequest, providerID string, caps types.ProviderCapabilities) {
	resolved := NormalizeReasoningParams(*req)

	// Never forward either dialect verbatim: each branch rebuilds exactly what
	// its provider understands.
	req.Thinking = nil
	req.ReasoningEffort = nil

	if !resolved.Present {
		return
	}

	if providerID == zaiProviderID {
		// Z.ai GLM models take thinking:{type} rather than an effort ladder.
		if !SupportsReasoningLevel(caps, resolved.Level) {
			return
		}
		if resolved.Disabled {
			// GLM hybrid-thinking models cannot reliably disable thinking.
			return
		}
		req.Thinking = &types.ThinkingConfig{Type: "enabled"}
		return
	}

	// Remaining targets speak the OpenAI reasoning_effort dialect through their
	// compatible endpoints (groq, nim, kilo, opencode, nous, openrouter*,
	// gemini's OpenAI layer).
	level := resolved.Level
	if (providerID == "openrouter" || providerID == "openrouter-alpha") && level == ReasoningMax {
		// Dialect translation runs before capability gating: OpenRouter's
		// vocabulary tops out at xhigh.
		level = ReasoningXHigh
	}

	if !SupportsReasoningLevel(caps, level) {
		return
	}

	if resolved.Disabled {
		// Most compatible layers reject effort "none"; dropping restores the
		// model's default behavior, which matches the intent closely enough
		// without risking upstream 400s.
		return
	}

	req.ReasoningEffort = &level

	// A synthesized token budget needs answer room above it when the caller
	// did not constrain output size themselves.
	if inflated := InflatedMaxTokens(*req, resolved.BudgetTokens); inflated > 0 {
		maxTokens := inflated
		req.MaxTokens = &maxTokens
	}
}

const zaiProviderID = "zai"

// applyOllamaThinking maps resolved reasoning params onto Ollama's native
// think option. Ollama hard-errors when a model lacks thinking support, so the
// capability gate must run before anything is emitted. gpt-oss accepts string
// levels; every other thinking model takes a boolean.
func applyOllamaThinking(dst *ollamaChatRequest, req types.ChatCompletionRequest, model string, caps types.ProviderCapabilities) {
	resolved := NormalizeReasoningParams(req)
	if !resolved.Present {
		return
	}
	if !SupportsReasoningLevel(caps, resolved.Level) {
		return
	}

	if resolved.Disabled {
		disabled := false
		dst.Think = disabled
		return
	}

	if strings.HasPrefix(model, "gpt-oss") {
		dst.Think = resolved.Level
		return
	}
	dst.Think = true
}
