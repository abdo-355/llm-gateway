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

// ---------------------------------------------------------------------------
// Additional Token Estimation Tests (from TypeScript)
// ---------------------------------------------------------------------------

func TestQuotaEstimateTokens_EdgeCases(t *testing.T) {
	client, _ := newTestRedis(t)
	svc := NewQuotaService(client, "")

	tests := []struct {
		name     string
		req      types.ChatCompletionRequest
		expected int
	}{
		{
			name: "empty messages",
			req: types.ChatCompletionRequest{
				Messages:  []types.OpenAIMessage{},
				MaxTokens: intPtr(100),
			},
			// Baseline 50 + max_tokens 100*4 = 450 chars, tokens = ceil(450/4) = 113
			expected: 113,
		},
		{
			name: "simple string content - hello world",
			req: types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{
					{Role: "user", Content: "Hello world"},
				},
				MaxTokens: intPtr(100),
			},
			// Baseline 50 + role overhead 15 + "Hello world" 11 + max_tokens 100*4 = 400
			// Total chars = 476, tokens = ceil(476/4) = 119
			expected: 119,
		},
		{
			name: "array content - hello",
			req: types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{
					{
						Role:    "user",
						Content: []any{map[string]any{"type": "text", "text": "Hello"}},
					},
				},
				MaxTokens: intPtr(50),
			},
			// Baseline 50 + role overhead 15 + "Hello" 5 + max_tokens 50*4 = 200
			// Total chars = 270, tokens = ceil(270/4) = 68
			expected: 68,
		},
		{
			name: "default max_tokens (1000)",
			req: types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{
					{Role: "user", Content: "Hi"},
				},
			},
			// Baseline 50 + role overhead 15 + "Hi" 2 + max_tokens 1000*4 = 4000
			// Total chars = 4067, tokens = ceil(4067/4) = 1017
			expected: 1017,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := svc.EstimateTokens(tt.req)
			assert.Equal(t, tt.expected, tokens)
		})
	}
}

// ---------------------------------------------------------------------------
// Provider Rate Limit Header Extraction Tests (from TypeScript)
// ---------------------------------------------------------------------------

func TestProviderCallProvider_RateLimitHeaderExtraction(t *testing.T) {
	tests := []struct {
		name            string
		statusCode      int
		headers         map[string]string
		wantRetryAfter  int
		wantIsRateLimit bool
	}{
		{
			name:            "429 with Retry-After header",
			statusCode:      429,
			headers:         map[string]string{"Retry-After": "30"},
			wantRetryAfter:  30,
			wantIsRateLimit: true,
		},
		{
			name:            "429 without Retry-After",
			statusCode:      429,
			headers:         map[string]string{},
			wantRetryAfter:  60, // Implementation uses hardcoded 60
			wantIsRateLimit: true,
		},
		{
			name:            "200 OK - no rate limit",
			statusCode:      200,
			headers:         map[string]string{},
			wantRetryAfter:  0,
			wantIsRateLimit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == 429 {
					json.NewEncoder(w).Encode(map[string]string{"error": "rate limited"})
				} else {
					json.NewEncoder(w).Encode(types.ChatCompletionResponse{
						ID: "test", Object: "chat.completion", Model: "gpt-4",
						Choices: []types.Choice{{
							Message: types.ResponseMessage{
								Role:    "assistant",
								Content: ptrString("ok"),
							},
							FinishReason: "stop",
						}},
					})
				}
			}))
			defer srv.Close()

			svc := newProviderService()
			req := types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
			}

			_, err := svc.CallProvider(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")

			if tt.wantIsRateLimit {
				require.Error(t, err)
				var rlErr *errors.RateLimitError
				require.ErrorAs(t, err, &rlErr)
				assert.Equal(t, tt.wantRetryAfter, rlErr.RetryAfter)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProviderConvertToGatewayError_SanitizesProviderError(t *testing.T) {
	svc := newProviderService()

	gatewayErr := svc.convertToGatewayError(&errors.ProviderError{
		Message:    `HTTP error 404: {"detail":"Function abc not found for account xyz"}`,
		StatusCode: http.StatusNotFound,
		Headers:    map[string]string{"x-request-id": "provider-request"},
	})

	require.NotNil(t, gatewayErr)
	assert.Equal(t, "provider_error", gatewayErr.Type)
	assert.Equal(t, "PROVIDER_ERROR", gatewayErr.Code)
	assert.Equal(t, "Upstream provider request failed", gatewayErr.Message)
	assert.Equal(t, http.StatusNotFound, gatewayErr.Details["status_code"])
	assert.NotContains(t, gatewayErr.Message, "Function abc")
	assert.NotContains(t, gatewayErr.Message, "account xyz")
	assert.NotContains(t, gatewayErr.Details, "headers")
}

// ---------------------------------------------------------------------------
// Streaming Provider Tests (from TypeScript - missing in Go)
// ---------------------------------------------------------------------------

func TestProviderStreamProvider_Success(t *testing.T) {
	chunk1 := types.SSEChunk{
		ID: "chatcmpl-1", Object: "chat.completion.chunk", Model: "gpt-4",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Role: "assistant"}}},
	}
	chunk2 := types.SSEChunk{
		ID: "chatcmpl-1", Object: "chat.completion.chunk", Model: "gpt-4",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("Hello")}}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
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
		Stream:   boolPtr(true),
	}

	result := svc.StreamProviderChannel(srv.URL, "test-key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")

	var chunks []*types.SSEChunk
	for chunk := range result.Chunks {
		chunks = append(chunks, chunk)
	}

	select {
	case gErr := <-result.Err:
		if gErr != nil {
			t.Fatalf("unexpected error: %v", gErr)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for completion signal")
	}

	require.Len(t, chunks, 2)
	assert.Equal(t, "assistant", chunks[0].Choices[0].Delta.Role)
	assert.Equal(t, "Hello", *chunks[1].Choices[0].Delta.Content)
}

func TestProviderStreamProvider_MultipleChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send multiple small chunks
		for i := 0; i < 5; i++ {
			chunk := types.SSEChunk{
				ID: "test", Object: "chat.completion.chunk",
				Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString(fmt.Sprintf("%d", i))}}},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Count"}},
		Stream:   boolPtr(true),
	}

	result := svc.StreamProviderChannel(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")

	var receivedChunks int
	for range result.Chunks {
		receivedChunks++
	}

	select {
	case gErr := <-result.Err:
		if gErr != nil {
			t.Fatalf("unexpected error: %v", gErr)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for completion signal")
	}

	assert.Equal(t, 5, receivedChunks)
}

func TestProviderStreamProvider_ErrorMidStream(t *testing.T) {
	chunk1 := types.SSEChunk{
		ID: "test", Object: "chat.completion.chunk",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("Start")}}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		data, _ := json.Marshal(chunk1)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// Then close connection (simulate error)
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Test"}},
		Stream:   boolPtr(true),
	}

	result := svc.StreamProviderChannel(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")

	// Should receive at least one chunk before error
	var receivedFirstChunk bool
	for range result.Chunks {
		receivedFirstChunk = true
	}

	// Error might or might not be sent depending on timing
	<-result.Err

	assert.True(t, receivedFirstChunk, "Should receive at least one chunk before error")
}

func TestProviderStreamProvider_Timeout(t *testing.T) {
	chunk1 := types.SSEChunk{
		ID: "test", Object: "chat.completion.chunk",
		Choices: []types.DeltaChoice{{Index: 0, Delta: types.DeltaMessage{Content: ptrString("Start")}}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		data, _ := json.Marshal(chunk1)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// Wait longer than timeout
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Test"}},
		Stream:   boolPtr(true),
	}

	// Use a short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := svc.StreamProviderChannel(srv.URL, "key", "gpt-4", req, 10000, ctx, "openai", types.ProviderAuth{Type: "bearer"}, "")

	// Should receive at least the first chunk
	var receivedChunk bool
	for range result.Chunks {
		receivedChunk = true
	}

	// Drain error channel
	<-result.Err

	assert.True(t, receivedChunk, "Should receive at least one chunk before timeout")
}

// ---------------------------------------------------------------------------
// Error Edge Cases (from TypeScript - negative codes, 599, etc.)
// ---------------------------------------------------------------------------

func TestProviderCallProvider_ErrorEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		wantErrorType  string
		wantRetryable  bool
		wantStatusCode int
	}{
		{
			name:           "502 Bad Gateway - retryable",
			statusCode:     502,
			responseBody:   `{"error":"bad gateway"}`,
			wantErrorType:  "ProviderError",
			wantRetryable:  true,
			wantStatusCode: 502,
		},
		{
			name:           "503 Service Unavailable - retryable",
			statusCode:     503,
			responseBody:   `{"error":"service unavailable"}`,
			wantErrorType:  "ProviderError",
			wantRetryable:  true,
			wantStatusCode: 503,
		},
		{
			name:           "504 Gateway Timeout - retryable (returns ProviderError)",
			statusCode:     504,
			responseBody:   `{"error":"gateway timeout"}`,
			wantErrorType:  "ProviderError",
			wantRetryable:  true,
			wantStatusCode: 504,
		},
		{
			name:           "400 Bad Request - not retryable",
			statusCode:     400,
			responseBody:   `{"error":"bad request"}`,
			wantErrorType:  "ValidationError",
			wantRetryable:  false,
			wantStatusCode: 400,
		},
		{
			name:           "401 Unauthorized - not retryable",
			statusCode:     401,
			responseBody:   `{"error":"unauthorized"}`,
			wantErrorType:  "ProviderError",
			wantRetryable:  false,
			wantStatusCode: 401,
		},
		{
			name:           "403 Forbidden - not retryable",
			statusCode:     403,
			responseBody:   `{"error":"forbidden"}`,
			wantErrorType:  "ProviderError",
			wantRetryable:  false,
			wantStatusCode: 403,
		},
		{
			name:           "404 Not Found - not retryable",
			statusCode:     404,
			responseBody:   `{"error":"not found"}`,
			wantErrorType:  "ProviderError",
			wantRetryable:  false,
			wantStatusCode: 404,
		},
		{
			name:           "422 Unprocessable Entity - not retryable",
			statusCode:     422,
			responseBody:   `{"error":"validation failed"}`,
			wantErrorType:  "ValidationError",
			wantRetryable:  false,
			wantStatusCode: 422,
		},
		{
			name:           "Edge of 5xx range - 599",
			statusCode:     599,
			responseBody:   `{"error":"server error"}`,
			wantErrorType:  "ProviderError",
			wantRetryable:  true,
			wantStatusCode: 599,
		},
		{
			name:           "Invalid JSON response",
			statusCode:     200,
			responseBody:   `invalid json {`,
			wantErrorType:  "json.SyntaxError",
			wantRetryable:  false,
			wantStatusCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer srv.Close()

			svc := newProviderService()
			req := types.ChatCompletionRequest{
				Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
			}

			_, err := svc.CallProvider(srv.URL, "key", "gpt-4", req, 10000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")
			require.Error(t, err)

			switch tt.wantErrorType {
			case "TimeoutError":
				var timeoutErr *errors.TimeoutError
				require.ErrorAs(t, err, &timeoutErr)
				assert.Equal(t, tt.wantStatusCode, timeoutErr.StatusCode)
				assert.Equal(t, tt.wantRetryable, timeoutErr.IsRetryable)
			case "ValidationError":
				var validationErr *errors.ValidationError
				require.ErrorAs(t, err, &validationErr)
				assert.Equal(t, tt.wantStatusCode, validationErr.StatusCode)
				assert.Equal(t, tt.wantRetryable, validationErr.IsRetryable)
			case "json.SyntaxError":
				// JSON parsing errors are returned directly
				assert.Error(t, err)
			default:
				var pErr *errors.ProviderError
				require.ErrorAs(t, err, &pErr)
				assert.Equal(t, tt.wantStatusCode, pErr.StatusCode)
				assert.Equal(t, tt.wantRetryable, pErr.IsRetryable)
			}
		})
	}
}

func TestProviderCallProvider_NetworkError(t *testing.T) {
	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	// Try to connect to a non-existent server
	_, err := svc.CallProvider("http://localhost:99999", "key", "gpt-4", req, 1000, context.Background(), "openai", types.ProviderAuth{Type: "bearer"}, "")
	require.Error(t, err)
}

func TestProviderCallProvider_ContextCanceledReturnsTimeoutError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test","object":"chat.completion","model":"gpt-4","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.CallProvider(srv.URL, "key", "gpt-4", req, 1000, ctx, "openai", types.ProviderAuth{Type: "bearer"}, "")
	require.Error(t, err)

	var timeoutErr *errors.TimeoutError
	require.ErrorAs(t, err, &timeoutErr)
	assert.Equal(t, "request", timeoutErr.TimeoutType)
	assert.Equal(t, 504, timeoutErr.StatusCode)
}

func TestProviderCallProvider_Ollama429ReturnsRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	svc := newProviderService()
	req := types.ChatCompletionRequest{
		Messages: []types.OpenAIMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := svc.CallProvider(srv.URL, "", "minimax-m2.7", req, 1000, context.Background(), "ollama", types.ProviderAuth{Type: "none"}, "")
	require.Error(t, err)

	var rateErr *errors.RateLimitError
	require.ErrorAs(t, err, &rateErr)
	assert.Equal(t, 42, rateErr.RetryAfter)
	assert.Equal(t, "rate_limit", rateErr.LimitSubtype)
}

// Helper functions - ptrString is defined in provider_test.go
func boolPtr(b bool) *bool { return &b }
