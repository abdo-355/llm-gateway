package services

import (
	"testing"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalizeReasoningEffort(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"minimal", ReasoningMinimal},
		{"LOW", ReasoningLow},
		{"  medium  ", ReasoningMedium},
		{"high", ReasoningHigh},
		{"xhigh", ReasoningXHigh},
		{"max", ReasoningMax},
		{"none", ReasoningDisabled},
		{"Disable", ReasoningDisabled},
		{"off", ReasoningDisabled},
		{"disabled", ReasoningDisabled},
		{"true", ReasoningMedium},
		{"default", ReasoningMedium},
		{"false", ReasoningDisabled},
		{"bogus", ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, CanonicalizeReasoningEffort(tc.in), "input %q", tc.in)
	}
}

func TestLoadBudgetLadderDefaults(t *testing.T) {
	ladder := loadBudgetLadder()
	assert.Equal(t, 128, ladder[ReasoningMinimal])
	assert.Equal(t, 1024, ladder[ReasoningLow])
	assert.Equal(t, 2048, ladder[ReasoningMedium])
	assert.Equal(t, 4096, ladder[ReasoningHigh])
	assert.Equal(t, 8192, ladder[ReasoningXHigh])
	assert.Equal(t, 16384, ladder[ReasoningMax])
}

func TestLoadBudgetLadderEnvOverride(t *testing.T) {
	t.Setenv("REASONING_BUDGET_HIGH", "9999")
	t.Setenv("REASONING_BUDGET_LOW", "not-a-number") // invalid input ignored
	ladder := loadBudgetLadder()
	assert.Equal(t, 9999, ladder[ReasoningHigh])
	assert.Equal(t, defaultBudgetLow, ladder[ReasoningLow])
}

func TestEffortBudgetRoundTripIsIdempotent(t *testing.T) {
	for _, level := range []string{ReasoningMinimal, ReasoningLow, ReasoningMedium, ReasoningHigh, ReasoningXHigh, ReasoningMax} {
		budget := EffortToBudget(level)
		require.Positive(t, budget)
		assert.Equal(t, level, BudgetToEffort(budget), "round trip for %s (budget %d)", level, budget)
	}
	assert.Equal(t, ReasoningDisabled, BudgetToEffort(0))
	assert.Equal(t, ReasoningDisabled, BudgetToEffort(-5))
}

func TestNormalizeReasoningParams(t *testing.T) {
	mkReq := func(effort *string, thinking *types.ThinkingConfig) types.ChatCompletionRequest {
		return types.ChatCompletionRequest{ReasoningEffort: effort, Thinking: thinking}
	}

	t.Run("absent", func(t *testing.T) {
		got := NormalizeReasoningParams(mkReq(nil, nil))
		assert.False(t, got.Present)
	})

	t.Run("effort only", func(t *testing.T) {
		got := NormalizeReasoningParams(mkReq(strPtr("high"), nil))
		assert.True(t, got.Present)
		assert.False(t, got.Disabled)
		assert.Equal(t, ReasoningHigh, got.Level)
		assert.Zero(t, got.BudgetTokens)
	})

	t.Run("explicit budget wins over effort", func(t *testing.T) {
		got := NormalizeReasoningParams(mkReq(strPtr("low"), &types.ThinkingConfig{
			Type: "enabled", BudgetTokens: intPtr(EffortToBudget(ReasoningHigh)),
		}))
		assert.True(t, got.Present)
		assert.Equal(t, EffortToBudget(ReasoningHigh), got.BudgetTokens)
		assert.Equal(t, ReasoningHigh, got.Level)
	})

	t.Run("thinking disabled always disables", func(t *testing.T) {
		got := NormalizeReasoningParams(mkReq(strPtr("high"), &types.ThinkingConfig{Type: "disabled"}))
		assert.True(t, got.Present)
		assert.True(t, got.Disabled)
		assert.Equal(t, ReasoningDisabled, got.Level)
	})

	t.Run("enabled without budget falls back to effort", func(t *testing.T) {
		got := NormalizeReasoningParams(mkReq(strPtr("low"), &types.ThinkingConfig{Type: "enabled"}))
		assert.True(t, got.Present)
		assert.Equal(t, ReasoningLow, got.Level)
	})

	t.Run("invalid effort ignored", func(t *testing.T) {
		got := NormalizeReasoningParams(mkReq(strPtr("bogus"), nil))
		assert.False(t, got.Present)
	})
}

func TestSupportsReasoningLevelTwoPostures(t *testing.T) {
	noReasoning := types.ProviderCapabilities{}
	assert.False(t, SupportsReasoningLevel(noReasoning, ReasoningMedium))
	assert.False(t, SupportsReasoningLevel(noReasoning, ""))

	common := types.ProviderCapabilities{Reasoning: true}
	// Common tiers pass on the implicit default set...
	for _, level := range []string{ReasoningMinimal, ReasoningLow, ReasoningMedium, ReasoningHigh, "", ReasoningDisabled} {
		assert.True(t, SupportsReasoningLevel(common, level), "level %q", level)
	}
	// ...exotic tiers stay opt-in.
	assert.False(t, SupportsReasoningLevel(common, ReasoningXHigh))
	assert.False(t, SupportsReasoningLevel(common, ReasoningMax))

	exotic := types.ProviderCapabilities{Reasoning: true, ReasoningLevels: []string{"medium", "high", "xhigh"}}
	assert.True(t, SupportsReasoningLevel(exotic, ReasoningXHigh))
	assert.True(t, SupportsReasoningLevel(exotic, ReasoningMedium))
	// An explicit list replaces the defaults entirely.
	assert.False(t, SupportsReasoningLevel(exotic, ReasoningMinimal))
	assert.False(t, SupportsReasoningLevel(exotic, ReasoningLow))

	disabledOK := types.ProviderCapabilities{Reasoning: true}
	assert.True(t, SupportsReasoningLevel(disabledOK, ReasoningDisabled))
}

func TestInflatedMaxTokens(t *testing.T) {
	withBudget := types.ChatCompletionRequest{}
	assert.Equal(t, EffortToBudget(ReasoningHigh)+4096, InflatedMaxTokens(withBudget, EffortToBudget(ReasoningHigh)))
	assert.Zero(t, InflatedMaxTokens(withBudget, 0))

	callerSet := types.ChatCompletionRequest{MaxTokens: intPtr(100)}
	assert.Zero(t, InflatedMaxTokens(callerSet, EffortToBudget(ReasoningHigh)))

	completionSet := types.ChatCompletionRequest{MaxCompletionTokens: intPtr(100)}
	assert.Zero(t, InflatedMaxTokens(completionSet, EffortToBudget(ReasoningHigh)))
}

func TestApplyReasoningForProvider(t *testing.T) {
	reasoningCaps := func() types.ProviderCapabilities {
		return types.ProviderCapabilities{Reasoning: true}
	}

	run := func(req types.ChatCompletionRequest, providerID string, caps types.ProviderCapabilities) types.ChatCompletionRequest {
		applyReasoningForProvider(&req, providerID, caps)
		return req
	}

	t.Run("no params is a no-op", func(t *testing.T) {
		req := run(types.ChatCompletionRequest{}, "groq", reasoningCaps())
		assert.Nil(t, req.ReasoningEffort)
		assert.Nil(t, req.Thinking)
	})

	t.Run("unsupported target drops everything", func(t *testing.T) {
		req := run(types.ChatCompletionRequest{ReasoningEffort: strPtr("high")}, "cohere", types.ProviderCapabilities{})
		assert.Nil(t, req.ReasoningEffort)
	})

	t.Run("passthrough family forwards canonical level", func(t *testing.T) {
		req := run(types.ChatCompletionRequest{ReasoningEffort: strPtr("HIGH")}, "nim", reasoningCaps())
		require.NotNil(t, req.ReasoningEffort)
		assert.Equal(t, ReasoningHigh, *req.ReasoningEffort)
	})

	t.Run("anthropic dict converted then stripped", func(t *testing.T) {
		req := run(types.ChatCompletionRequest{
			Thinking: &types.ThinkingConfig{Type: "enabled", BudgetTokens: intPtr(4096)},
		}, "nous", reasoningCaps())
		assert.Nil(t, req.Thinking)
		require.NotNil(t, req.ReasoningEffort)
		assert.Equal(t, ReasoningHigh, *req.ReasoningEffort)
	})

	t.Run("openrouter renames max to xhigh before gating", func(t *testing.T) {
		// Dialect translation first: max becomes xhigh, which then must clear
		// the opt-in posture — plain caps reject exotic tiers outright.
		dropped := run(types.ChatCompletionRequest{ReasoningEffort: strPtr("max")}, "openrouter", reasoningCaps())
		assert.Nil(t, dropped.ReasoningEffort)

		xhighCaps := types.ProviderCapabilities{Reasoning: true, ReasoningLevels: []string{"low", "medium", "high", "xhigh"}}
		req := run(types.ChatCompletionRequest{ReasoningEffort: strPtr("max")}, "openrouter", xhighCaps)
		require.NotNil(t, req.ReasoningEffort)
		assert.Equal(t, ReasoningXHigh, *req.ReasoningEffort)
	})

	t.Run("other providers keep max verbatim", func(t *testing.T) {
		req := run(types.ChatCompletionRequest{ReasoningEffort: strPtr("max")}, "groq", types.ProviderCapabilities{
			Reasoning:       true,
			ReasoningLevels: []string{ReasoningMax},
		})
		require.NotNil(t, req.ReasoningEffort)
		assert.Equal(t, ReasoningMax, *req.ReasoningEffort)
	})

	t.Run("disabled drops rather than forwarding none", func(t *testing.T) {
		req := run(types.ChatCompletionRequest{ReasoningEffort: strPtr("none")}, "gemini", reasoningCaps())
		assert.Nil(t, req.ReasoningEffort)
	})

	t.Run("zai gets a thinking toggle instead of effort", func(t *testing.T) {
		req := run(types.ChatCompletionRequest{ReasoningEffort: strPtr("high")}, zaiProviderID, reasoningCaps())
		assert.Nil(t, req.ReasoningEffort)
		require.NotNil(t, req.Thinking)
		assert.Equal(t, "enabled", req.Thinking.Type)

		// Disabled requests forward the official toggle rather than dropping:
		// Z.ai hybrid models honor thinking.type=disabled (litellm parity).
		off := run(types.ChatCompletionRequest{ReasoningEffort: strPtr("none")}, zaiProviderID, reasoningCaps())
		require.NotNil(t, off.Thinking)
		assert.Equal(t, "disabled", off.Thinking.Type)

		dropped := run(types.ChatCompletionRequest{ReasoningEffort: strPtr("high")}, zaiProviderID, types.ProviderCapabilities{})
		assert.Nil(t, dropped.Thinking)
		assert.Nil(t, dropped.ReasoningEffort)
	})

	t.Run("synthesized budget inflates max_tokens when caller set none", func(t *testing.T) {
		req := run(types.ChatCompletionRequest{
			Thinking: &types.ThinkingConfig{Type: "enabled", BudgetTokens: intPtr(2048)},
		}, "groq", reasoningCaps())
		require.NotNil(t, req.MaxTokens)
		assert.Equal(t, 2048+4096, *req.MaxTokens)
	})

	t.Run("caller-provided max_tokens is never touched", func(t *testing.T) {
		req := run(types.ChatCompletionRequest{
			Thinking:  &types.ThinkingConfig{Type: "enabled", BudgetTokens: intPtr(2048)},
			MaxTokens: intPtr(500),
		}, "groq", reasoningCaps())
		require.NotNil(t, req.MaxTokens)
		assert.Equal(t, 500, *req.MaxTokens)
	})
}

func TestApplyOllamaThinking(t *testing.T) {
	thinkingCaps := types.ProviderCapabilities{Reasoning: true}

	build := func(req types.ChatCompletionRequest, model string, caps types.ProviderCapabilities) any {
		dst := &ollamaChatRequest{}
		applyOllamaThinking(dst, req, model, caps)
		return dst.Think
	}

	assert.Nil(t, build(types.ChatCompletionRequest{}, "qwen3-coder:480b", thinkingCaps))

	// gpt-oss takes string levels.
	assert.Equal(t, ReasoningHigh, build(
		types.ChatCompletionRequest{ReasoningEffort: strPtr("high")}, "gpt-oss:120b", thinkingCaps))

	// Other thinking models take a boolean.
	assert.Equal(t, true, build(
		types.ChatCompletionRequest{ReasoningEffort: strPtr("medium")}, "deepseek-v3.2", thinkingCaps))

	// Disabled requests send think=false so hybrid models actually stop thinking.
	assert.Equal(t, false, build(
		types.ChatCompletionRequest{ReasoningEffort: strPtr("off")}, "qwen3-coder:480b", thinkingCaps))

	// Capability gate runs first — ollama hard-errors otherwise.
	assert.Nil(t, build(
		types.ChatCompletionRequest{ReasoningEffort: strPtr("high")}, "gemma3:27b", types.ProviderCapabilities{}))
}

func TestPrepareRequestStripsThinkingBlocks(t *testing.T) {
	svc := newProviderService()

	request := types.ChatCompletionRequest{
		Model: "gpt-oss:120b",
		Messages: []types.OpenAIMessage{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "answer",
				ThinkingBlocks: []types.ThinkingBlock{{Type: "thinking", Thinking: "hmm", Signature: "sig"}}},
			{Role: "user", Content: "continue"},
		},
	}

	body, err := svc.prepareRequest(request, "gpt-oss:120b", "https://api.groq.com/openai/v1", "openai", types.ProviderAuth{})
	require.NoError(t, err)
	assert.NotContains(t, string(body), "thinking_blocks")
	assert.NotContains(t, string(body), `"signature"`)

	// The caller's struct keeps the blocks — stripping is upstream-only.
	assert.Len(t, request.Messages[1].ThinkingBlocks, 1)
}

func TestDeriveRequirementsDetectsReasoning(t *testing.T) {
	router := &Router{}

	base := types.ChatCompletionRequest{Messages: []types.OpenAIMessage{{Role: "user", Content: "hi"}}}
	reqs := router.DeriveRequirements(base, nil)
	assert.Equal(t, "forbidden", reqs.Reasoning)
	assert.Empty(t, reqs.ReasoningLevel)

	withEffort := base
	withEffort.ReasoningEffort = strPtr("high")
	reqs = router.DeriveRequirements(withEffort, nil)
	assert.Equal(t, "preferred", reqs.Reasoning)
	assert.Equal(t, ReasoningHigh, reqs.ReasoningLevel)

	disabledAsk := base
	disabledAsk.ReasoningEffort = strPtr("none")
	reqs = router.DeriveRequirements(disabledAsk, nil)
	assert.Equal(t, "preferred", reqs.Reasoning)
	assert.Empty(t, reqs.ReasoningLevel)

	// The handler unwraps req.Router and passes it as the hints argument.
	hintedRouter := &types.RouterHints{Requirements: &types.RouterRequirements{Reasoning: strPtr("required")}}
	reqs = router.DeriveRequirements(withEffort, hintedRouter)
	assert.Equal(t, "required", reqs.Reasoning)
}
