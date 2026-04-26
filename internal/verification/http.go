package verification

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	gatewayerrors "github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
)

type client struct {
	providerService *services.ProviderService
	requestTimeout  time.Duration
}

type probeClient interface {
	call(ctx context.Context, combo Combo, request types.ChatCompletionRequest) requestResult
	stream(ctx context.Context, combo Combo, request types.ChatCompletionRequest) requestResult
}

func newClient(cfg Config) *client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &client{
		providerService: services.NewProviderService(),
		requestTimeout:  timeout,
	}
}

func (c *client) call(ctx context.Context, combo Combo, request types.ChatCompletionRequest) requestResult {
	apiKey, err := resolveAPIKey(ctx, combo.Provider)
	if err != nil {
		return requestResult{Failure: err.Error()}
	}

	started := time.Now()
	resp, err := c.providerService.CallProvider(
		resolvedBaseURL(combo.Provider),
		apiKey,
		combo.Model,
		request,
		int(c.requestTimeout.Milliseconds()),
		ctx,
		providerType(combo.Provider),
		combo.Provider.Auth,
	)
	latency := time.Since(started)
	if err != nil {
		status, failure := describeProviderError(err)
		return requestResult{Latency: latency, HTTPStatus: status, Failure: failure, Attempted: true}
	}

	result := requestResult{Latency: latency, HTTPStatus: 200, Response: resp, Attempted: true}
	if resp != nil && resp.Usage != nil {
		result.TokensUsed = fmt.Sprintf("%d", resp.Usage.TotalTokens)
	}
	return result
}

func (c *client) stream(ctx context.Context, combo Combo, request types.ChatCompletionRequest) requestResult {
	apiKey, err := resolveAPIKey(ctx, combo.Provider)
	if err != nil {
		return requestResult{Failure: err.Error()}
	}

	started := time.Now()
	stream := c.providerService.StreamProviderChannel(
		resolvedBaseURL(combo.Provider),
		apiKey,
		combo.Model,
		request,
		int(c.requestTimeout.Milliseconds()),
		ctx,
		providerType(combo.Provider),
		combo.Provider.Auth,
	)

	chunks := make([]types.SSEChunk, 0)
	for chunk := range stream.Chunks {
		if chunk != nil {
			chunks = append(chunks, *chunk)
		}
	}

	latency := time.Since(started)
	if err := <-stream.Err; err != nil {
		status := 0
		if err.Code == "RATE_LIMITED" {
			status = 429
		} else if err.Code == "PAYMENT_REQUIRED" {
			status = 402
		}
		return requestResult{Latency: latency, HTTPStatus: status, Failure: fmt.Sprintf("provider_error: code=%s message=%s", err.Code, err.Message), Chunks: chunks, Attempted: true}
	}

	result := requestResult{Latency: latency, HTTPStatus: 200, Chunks: chunks, Done: true, Attempted: true}
	for i := len(chunks) - 1; i >= 0; i-- {
		if chunks[i].Usage != nil {
			result.TokensUsed = fmt.Sprintf("%d", chunks[i].Usage.TotalTokens)
			break
		}
	}
	return result
}

func resolveAPIKey(ctx context.Context, provider types.ProviderConfig) (string, error) {
	switch provider.Auth.Type {
	case "bearer", "header":
		if provider.Auth.Env == "" {
			return "", nil
		}
		value := strings.TrimSpace(os.Getenv(provider.Auth.Env))
		if value == "" {
			if provider.Auth.Optional {
				return "", nil
			}
			return "", fmt.Errorf("auth_missing: %s is empty", provider.Auth.Env)
		}
		return value, nil
	default:
		return "", nil
	}
}

func resolvedBaseURL(provider types.ProviderConfig) string {
	baseURL := provider.BaseURL
	if provider.ID == "vertex" {
		projectID := os.Getenv("GOOGLE_VERTEX_PROJECT_ID")
		if projectID != "" {
			baseURL = strings.ReplaceAll(baseURL, "PROJECT_ID", projectID)
		}
	}
	return baseURL
}

func providerType(provider types.ProviderConfig) string {
	if provider.ProviderType != "" {
		return provider.ProviderType
	}
	return "openai"
}

func describeProviderError(err error) (int, string) {
	switch e := err.(type) {
	case *gatewayerrors.RateLimitError:
		return 429, fmt.Sprintf("rate_limited: retry_after=%d limit_type=%s", e.RetryAfter, e.LimitType)
	case *gatewayerrors.PaymentRequiredError:
		return 402, "payment_required"
	case *gatewayerrors.TimeoutError:
		return 408, fmt.Sprintf("timeout: type=%s message=%s", e.TimeoutType, e.Message)
	case *gatewayerrors.ProviderError:
		return e.StatusCode, fmt.Sprintf("provider_error: status=%d message=%s", e.StatusCode, e.Message)
	default:
		return 0, fmt.Sprintf("request_failed: %v", err)
	}
}
