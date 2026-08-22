package services

import (
	"testing"

	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamNormalizerLatchesIdentity(t *testing.T) {
	normalizer := newStreamNormalizer(types.ChatCompletionRequest{})

	chunks := []*types.SSEChunk{
		{ID: "chatcmpl-aaa", Created: 1000, Model: "gpt-oss:120b",
			Choices: []types.DeltaChoice{{Delta: types.DeltaMessage{Role: "assistant"}}}},
		// Ollama-style: every chunk mints a fresh id.
		{ID: "chatcmpl-bbb", Created: 1001, Model: "other-model",
			Choices: []types.DeltaChoice{{Delta: types.DeltaMessage{Content: ptrString("Hi")}}}},
		{ID: "chatcmpl-ccc", Created: 1002, Model: "another",
			Choices: []types.DeltaChoice{{FinishReason: strPtr("stop")}}},
	}

	for _, chunk := range chunks {
		assert.True(t, normalizer.Process(chunk))
	}

	for _, chunk := range chunks[1:] {
		assert.Equal(t, "chatcmpl-aaa", chunk.ID)
		assert.Equal(t, int64(1000), chunk.Created)
		assert.Equal(t, "gpt-oss:120b", chunk.Model)
	}
}

func TestStreamNormalizerSuppressesUsageArtifacts(t *testing.T) {
	usageChunk := &types.SSEChunk{ID: "x", Model: "m", Usage: &types.Usage{}}

	withoutUsage := newStreamNormalizer(types.ChatCompletionRequest{})
	assert.False(t, withoutUsage.Process(usageChunk), "usage-only chunk suppressed without include_usage")

	withUsage := newStreamNormalizer(types.ChatCompletionRequest{
		StreamOptions: &types.StreamOptions{IncludeUsage: boolPtr(true)},
	})
	assert.True(t, withUsage.Process(usageChunk))

	// Usage attached to a content-bearing chunk is never suppressed.
	mixed := &types.SSEChunk{ID: "y", Model: "m", Usage: &types.Usage{},
		Choices: []types.DeltaChoice{{Delta: types.DeltaMessage{Content: ptrString("hi")}}}}
	assert.True(t, withoutUsage.Process(mixed))
}

func TestStreamNormalizerTerminalSynthesis(t *testing.T) {
	t.Run("synthesizes stop when upstream ends silently", func(t *testing.T) {
		normalizer := newStreamNormalizer(types.ChatCompletionRequest{})
		assert.True(t, normalizer.Process(&types.SSEChunk{ID: "id-1", Model: "m",
			Choices: []types.DeltaChoice{{Delta: types.DeltaMessage{Content: ptrString("text")}}}}))

		terminal := normalizer.TerminalChunks()
		require.Len(t, terminal, 1)
		require.Len(t, terminal[0].Choices, 1)
		require.NotNil(t, terminal[0].Choices[0].FinishReason)
		assert.Equal(t, "stop", *terminal[0].Choices[0].FinishReason)
		assert.Equal(t, "id-1", terminal[0].ID)
	})

	t.Run("no stop synthesis when finish_reason arrived", func(t *testing.T) {
		normalizer := newStreamNormalizer(types.ChatCompletionRequest{})
		normalizer.Process(&types.SSEChunk{ID: "id-2", Model: "m",
			Choices: []types.DeltaChoice{{Delta: types.DeltaMessage{Content: ptrString("t")}, FinishReason: strPtr("stop")}}})
		assert.Empty(t, normalizer.TerminalChunks())
	})

	t.Run("synthesizes usage chunk when include_usage set but upstream silent", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			StreamOptions: &types.StreamOptions{IncludeUsage: boolPtr(true)},
		}
		normalizer := newStreamNormalizer(req)
		normalizer.Process(&types.SSEChunk{ID: "id-3", Model: "m",
			Choices: []types.DeltaChoice{{Delta: types.DeltaMessage{Content: ptrString("t")}, FinishReason: strPtr("stop")}}})

		terminal := normalizer.TerminalChunks()
		require.Len(t, terminal, 1)
		assert.NotNil(t, terminal[0].Usage)
		assert.Empty(t, terminal[0].Choices, "usage chunk carries no choices")
	})

	t.Run("no usage synthesis when upstream already reported usage", func(t *testing.T) {
		req := types.ChatCompletionRequest{
			StreamOptions: &types.StreamOptions{IncludeUsage: boolPtr(true)},
		}
		normalizer := newStreamNormalizer(req)
		normalizer.Process(&types.SSEChunk{ID: "id-4", Model: "m", Usage: &types.Usage{},
			Choices: []types.DeltaChoice{{FinishReason: strPtr("stop")}}})
		assert.Empty(t, normalizer.TerminalChunks())
	})
}

func TestChunkHasPayload(t *testing.T) {
	delta := func(d types.DeltaMessage) *types.SSEChunk {
		return &types.SSEChunk{Choices: []types.DeltaChoice{{Delta: d}}}
	}

	assert.False(t, chunkHasPayload(delta(types.DeltaMessage{})))
	assert.False(t, chunkHasPayload(delta(types.DeltaMessage{Role: "assistant"})), "role preamble is not payload")
	assert.True(t, chunkHasPayload(delta(types.DeltaMessage{Content: ptrString("x")})))
	assert.True(t, chunkHasPayload(delta(types.DeltaMessage{Refusal: ptrString("x")})))
	assert.True(t, chunkHasPayload(delta(types.DeltaMessage{ReasoningContent: ptrString("thinking...")})))
	assert.True(t, chunkHasPayload(delta(types.DeltaMessage{ThinkingBlocks: []types.ThinkingBlock{{Type: "thinking"}}})))
	assert.True(t, chunkHasPayload(delta(types.DeltaMessage{ToolCalls: []types.DeltaToolCall{{Index: 0}}})))
}
