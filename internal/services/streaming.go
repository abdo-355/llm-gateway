package services

import (
	"github.com/abdo-355/llm-gateway/internal/types"
)

// streamNormalizer enforces litellm-style stream discipline for one upstream
// attempt: stable chunk identity across the whole stream, usage-artifact
// suppression unless the client asked for include_usage, and terminal
// synthesis (finish_reason, usage) for upstreams that end without them.
type streamNormalizer struct {
	latched         bool
	id              string
	created         int64
	model           string
	sawFinishReason bool
	usageSeen       bool
	includeUsage    bool
}

func newStreamNormalizer(req types.ChatCompletionRequest) *streamNormalizer {
	normalizer := &streamNormalizer{}
	if req.StreamOptions != nil && req.StreamOptions.IncludeUsage != nil {
		normalizer.includeUsage = *req.StreamOptions.IncludeUsage
	}
	return normalizer
}

// Process stamps a provider chunk into its client-facing shape and reports
// whether it should be forwarded at all.
func (n *streamNormalizer) Process(chunk *types.SSEChunk) bool {
	if chunk == nil {
		return false
	}

	// Identity latch: the first chunk that carries any identifying field wins;
	// every later chunk inherits it so clients can correlate the stream.
	// (Fixes providers like ollama that mint fresh IDs per NDJSON line.)
	if !n.latched && (chunk.ID != "" || chunk.Model != "" || chunk.Created != 0) {
		n.id = chunk.ID
		n.created = chunk.Created
		n.model = chunk.Model
		n.latched = true
	} else if n.latched {
		if n.id != "" {
			chunk.ID = n.id
		}
		if n.created != 0 {
			chunk.Created = n.created
		}
		if n.model != "" {
			chunk.Model = n.model
		}
	}

	for _, choice := range chunk.Choices {
		if choice.FinishReason != nil {
			n.sawFinishReason = true
		}
	}
	if chunk.Usage != nil {
		n.usageSeen = true
	}

	// A choices-less usage chunk is an OpenAI include_usage artifact; forward
	// it only when the caller actually requested usage.
	if chunk.Usage != nil && !chunkHasPayload(chunk) && !n.includeUsage {
		return false
	}

	return true
}

// TerminalChunks builds the closing frames for upstreams that ended without a
// finish_reason and/or without the usage the client explicitly requested,
// keeping client parsers well-formed before [DONE]. Returns whatever is
// missing; empty slice when the upstream already terminated properly.
func (n *streamNormalizer) TerminalChunks() []*types.SSEChunk {
	var chunks []*types.SSEChunk

	if !n.sawFinishReason {
		stop := "stop"
		chunks = append(chunks, &types.SSEChunk{
			Object:  "chat.completion.chunk",
			Created: n.created,
			ID:      n.id,
			Model:   n.model,
			Choices: []types.DeltaChoice{{
				Index:        0,
				Delta:        types.DeltaMessage{},
				FinishReason: &stop,
			}},
		})
	}

	if n.includeUsage && !n.usageSeen {
		chunks = append(chunks, &types.SSEChunk{
			Object:  "chat.completion.chunk",
			Created: n.created,
			ID:      n.id,
			Model:   n.model,
			Choices: []types.DeltaChoice{},
			Usage:   &types.Usage{},
		})
	}

	return chunks
}

// chunkHasPayload reports whether a chunk carries anything a client renders:
// role, visible text, reasoning text, thinking blocks, tool calls, or refusal.
// Role-only preambles do not count as payload for failover-locking purposes.
func chunkHasPayload(chunk *types.SSEChunk) bool {
	for _, choice := range chunk.Choices {
		delta := choice.Delta
		if delta.Content != nil || delta.Refusal != nil || delta.ReasoningContent != nil ||
			len(delta.ThinkingBlocks) > 0 || len(delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}
