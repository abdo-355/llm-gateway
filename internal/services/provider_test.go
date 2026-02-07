package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newProviderService() *ProviderService {
	return NewProviderService()
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt(v int) *int             { return &v }

// ---------------------------------------------------------------------------
// CallProvider (non-streaming)
// ---------------------------------------------------------------------------

func TestProviderCallProvider_Success(t *testing.T) {
	want := types.ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1700000000,
		Model:   "gpt-4",
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ResponseMessage{
				Role:    "assistant",
				Content: ptrString("Hello!"),
			},
			FinishReason: "stop",
		}},
		Usage: &types.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	got, err := svc.CallProvider(srv.URL, "test-key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"})
	require.NoError(t, err)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.Model, got.Model)
	assert.Equal(t, "Hello!", *got.Choices[0].Message.Content)
	assert.Equal(t, 15, got.Usage.TotalTokens)
}

func TestProviderCallProvider_429_RateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"})
	require.Error(t, err)

	var rlErr *errors.RateLimitError
	assert.ErrorAs(t, err, &rlErr)
}

func TestProviderCallProvider_402_PaymentRequiredError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error":"payment required"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"})
	require.Error(t, err)

	var pErr *errors.PaymentRequiredError
	assert.ErrorAs(t, err, &pErr)
}

func TestProviderCallProvider_500_RetryableProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal server error`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"})
	require.Error(t, err)

	var pErr *errors.ProviderError
	require.ErrorAs(t, err, &pErr)
	assert.True(t, pErr.IsRetryable)
	assert.Equal(t, 500, pErr.StatusCode)
}

func TestProviderCallProvider_BearerAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer my-secret-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ChatCompletionResponse{
			ID: "test", Object: "chat.completion", Model: "gpt-4",
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: ptrString("ok")}, FinishReason: "stop"}},
		})
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "my-secret-key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"})
	require.NoError(t, err)
}

func TestProviderCallProvider_CustomHeaderAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "my-api-key", r.Header.Get("X-Custom-Auth"))
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ChatCompletionResponse{
			ID: "test", Object: "chat.completion", Model: "gpt-4",
			Choices: []types.Choice{{Message: types.ResponseMessage{Role: "assistant", Content: ptrString("ok")}, FinishReason: "stop"}},
		})
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "my-api-key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{
		Type:       "header",
		HeaderName: "X-Custom-Auth",
	})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// StreamProviderChannel
// ---------------------------------------------------------------------------

func TestProviderStreamProviderChannel_Success(t *testing.T) {
	chunk1 := types.SSEChunk{
		ID: "chunk-1", Object: "chat.completion.chunk", Model: "gpt-4",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("Hel")}}},
	}
	chunk2 := types.SSEChunk{
		ID: "chunk-2", Object: "chat.completion.chunk", Model: "gpt-4",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("lo!")}}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		data1, _ := json.Marshal(chunk1)
		fmt.Fprintf(w, "data: %s\n\n", data1)
		flusher.Flush()

		data2, _ := json.Marshal(chunk2)
		fmt.Fprintf(w, "data: %s\n\n", data2)
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	result := svc.StreamProviderChannel(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"})

	var received []*types.SSEChunk
	for chunk := range result.Chunks {
		received = append(received, chunk)
	}

	select {
	case gErr := <-result.Err:
		t.Fatalf("unexpected error: %v", gErr)
	default:
	}

	require.Len(t, received, 2)
	assert.Equal(t, "chunk-1", received[0].ID)
	assert.Equal(t, "Hel", *received[0].Choices[0].Delta.Content)
	assert.Equal(t, "chunk-2", received[1].ID)
	assert.Equal(t, "lo!", *received[1].Choices[0].Delta.Content)
}

func TestProviderStreamProviderChannel_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	result := svc.StreamProviderChannel(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"})

	for range result.Chunks {
	}

	select {
	case gErr := <-result.Err:
		require.NotNil(t, gErr)
		assert.Contains(t, gErr.Message, "Rate limited")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for error")
	}
}

func TestProviderStreamProviderChannel_DoneTerminates(t *testing.T) {
	chunk1 := types.SSEChunk{
		ID: "c1", Object: "chat.completion.chunk", Model: "gpt-4",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("A")}}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		data, _ := json.Marshal(chunk1)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	result := svc.StreamProviderChannel(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"})

	count := 0
	for range result.Chunks {
		count++
	}

	assert.Equal(t, 1, count)

	select {
	case gErr := <-result.Err:
		t.Fatalf("unexpected error: %v", gErr)
	default:
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func ptrString(s string) *string { return &s }
