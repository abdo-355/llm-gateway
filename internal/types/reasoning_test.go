package types

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseMessageReasoningVariants(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantContent *string
		wantReason  string
		wantBlocks  int
	}{
		{
			name:        "reasoning_content field",
			body:        `{"role":"assistant","content":"Answer","reasoning_content":"Because"}`,
			wantContent: strPtrT("Answer"),
			wantReason:  "Because",
		},
		{
			name:        "groq reasoning alias",
			body:        `{"role":"assistant","content":"Answer","reasoning":"Thinking hard"}`,
			wantContent: strPtrT("Answer"),
			wantReason:  "Thinking hard",
		},
		{
			name:        "ollama thinking alias",
			body:        `{"role":"assistant","content":"Answer","thinking":"Hmm"}`,
			wantContent: strPtrT("Answer"),
			wantReason:  "Hmm",
		},
		{
			name:        "explicit reasoning_content beats aliases",
			body:        `{"role":"assistant","content":"A","reasoning_content":"canonical","reasoning":"alias","thinking":"other"}`,
			wantContent: strPtrT("A"),
			wantReason:  "canonical",
		},
		{
			name:        "anthropic-style typed blocks harvested with signature",
			body:        `{"role":"assistant","content":[{"type":"thinking","thinking":"secret plan","signature":"sig-1"},{"type":"text","text":"Visible"}]}`,
			wantContent: strPtrT("Visible"),
			wantReason:  "secret plan",
			wantBlocks:  1,
		},
		{
			name:        "mistral-style thinking container",
			body:        `{"role":"assistant","content":[{"type":"text","text":"Vis"},{"thinking":[{"type":"text","text":"Inner thought"}]}]}`,
			wantContent: strPtrT("Vis"),
			wantReason:  "Inner thought",
			wantBlocks:  1,
		},
		{
			name:        "redacted thinking keeps data out of flat text",
			body:        `{"role":"assistant","content":[{"type":"redacted_thinking","data":"opaque"},{"type":"text","text":"Out"}]}`,
			wantContent: strPtrT("Out"),
			wantReason:  "",
			wantBlocks:  1,
		},
		{
			name:        "deepseek think tags extracted from content",
			body:        `{"role":"assistant","content":"<think>chain of thought</think>Final answer"}`,
			wantContent: strPtrT("Final answer"),
			wantReason:  "chain of thought",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var msg ResponseMessage
			require.NoError(t, json.Unmarshal([]byte(tc.body), &msg))
			assert.Equal(t, tc.wantContent, msg.Content)
			if tc.wantReason == "" {
				assert.Nil(t, msg.ReasoningContent)
			} else {
				require.NotNil(t, msg.ReasoningContent)
				assert.Equal(t, tc.wantReason, *msg.ReasoningContent)
			}
			assert.Len(t, msg.ThinkingBlocks, tc.wantBlocks)
			if tc.wantBlocks > 0 && strings.Contains(tc.body, "signature") {
				assert.Equal(t, "sig-1", msg.ThinkingBlocks[0].Signature)
			}
		})
	}
}

func TestDeltaMessageReasoningVariants(t *testing.T) {
	var delta DeltaMessage
	require.NoError(t, json.Unmarshal(
		[]byte(`{"reasoning":"partial thought"}`), &delta))
	require.NotNil(t, delta.ReasoningContent)
	assert.Equal(t, "partial thought", *delta.ReasoningContent)

	var ollamaDelta DeltaMessage
	require.NoError(t, json.Unmarshal(
		[]byte(`{"thinking":"ndjson thought","content":"visible"}`), &ollamaDelta))
	require.NotNil(t, ollamaDelta.ReasoningContent)
	assert.Equal(t, "ndjson thought", *ollamaDelta.ReasoningContent)
	assert.NotNil(t, ollamaDelta.Content)

	var plain DeltaMessage
	require.NoError(t, json.Unmarshal([]byte(`{"content":"just text"}`), &plain))
	assert.Nil(t, plain.ReasoningContent)
	assert.Nil(t, plain.ThinkingBlocks)
}

func TestExtractThinkTags(t *testing.T) {
	visible, reasoning, found := extractThinkTags("<think>plan</think>answer")
	assert.True(t, found)
	assert.Equal(t, "plan", reasoning)
	assert.Equal(t, "answer", visible)

	_, _, found = extractThinkTags("no tags here")
	assert.False(t, found)

	// Leading whitespace before the opening block still counts as leading.
	visible, reasoning, found = extractThinkTags("\n  <think>plan</think>answer")
	assert.True(t, found)
	assert.Equal(t, "plan", reasoning)
	assert.Equal(t, "answer", visible)

	// Multiple blocks join; surrounding whitespace is trimmed.
	visible, reasoning, found = extractThinkTags("<think>a</think>\n<think>b</think> done")
	assert.True(t, found)
	assert.Equal(t, "a\nb", reasoning)
	assert.Equal(t, "done", visible)

	// Tags that appear mid-text are literal output (e.g. a model
	// demonstrating XML on request) and must not be split out.
	literal := "Here is an example: <think>demo</think> — that's the format."
	visible, reasoning, found = extractThinkTags(literal)
	assert.False(t, found)
	assert.Equal(t, literal, visible)
	assert.Empty(t, reasoning)
}

func strPtrT(s string) *string { return &s }
