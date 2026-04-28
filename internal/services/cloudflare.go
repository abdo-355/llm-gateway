package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/abdo-355/llm-gateway/internal/errors"
	"github.com/abdo-355/llm-gateway/internal/types"
)

const (
	cloudflareProviderID              = "cloudflare"
	cloudflareProviderType            = "cloudflare_workers_ai"
	cloudflareAPITokenEnv             = "CLOUDFLARE_API_TOKEN"
	cloudflareAccountIDEnv            = "CLOUDFLARE_ACCOUNT_ID"
	cloudflareFreeDailyNeuronBudget   = 10000
	cloudflareMinRemainingDailyBudget = 300
	cloudflareDefaultMaxTokens        = 512
	cloudflarePaidUSDPerThousand      = 0.011
)

type cloudflareModelPricing struct {
	InputNeuronsPerM       int
	CachedInputNeuronsPerM int
	OutputNeuronsPerM      int
}

var cloudflarePricingTable = map[string]cloudflareModelPricing{
	"@cf/moonshotai/kimi-k2.6":                     {InputNeuronsPerM: 86364, CachedInputNeuronsPerM: 14545, OutputNeuronsPerM: 363636},
	"@cf/moonshotai/kimi-k2.5":                     {InputNeuronsPerM: 54545, CachedInputNeuronsPerM: 9091, OutputNeuronsPerM: 272727},
	"@cf/google/gemma-4-26b-a4b-it":                {InputNeuronsPerM: 9091, CachedInputNeuronsPerM: 9091, OutputNeuronsPerM: 27273},
	"@cf/openai/gpt-oss-120b":                      {InputNeuronsPerM: 31818, CachedInputNeuronsPerM: 31818, OutputNeuronsPerM: 68182},
	"@cf/nvidia/nemotron-3-120b-a12b":              {InputNeuronsPerM: 45455, CachedInputNeuronsPerM: 45455, OutputNeuronsPerM: 136364},
	"@cf/openai/gpt-oss-20b":                       {InputNeuronsPerM: 18182, CachedInputNeuronsPerM: 18182, OutputNeuronsPerM: 27273},
	"@cf/qwen/qwen3-30b-a3b-fp8":                   {InputNeuronsPerM: 4625, CachedInputNeuronsPerM: 4625, OutputNeuronsPerM: 30475},
	"@cf/zai-org/glm-4.7-flash":                    {InputNeuronsPerM: 5500, CachedInputNeuronsPerM: 5500, OutputNeuronsPerM: 36400},
	"@cf/qwen/qwen2.5-coder-32b-instruct":          {InputNeuronsPerM: 60000, CachedInputNeuronsPerM: 60000, OutputNeuronsPerM: 90909},
	"@cf/qwen/qwq-32b":                             {InputNeuronsPerM: 60000, CachedInputNeuronsPerM: 60000, OutputNeuronsPerM: 90909},
	"@cf/deepseek-ai/deepseek-r1-distill-qwen-32b": {InputNeuronsPerM: 45170, CachedInputNeuronsPerM: 45170, OutputNeuronsPerM: 443756},
	"@cf/meta/llama-4-scout-17b-16e-instruct":      {InputNeuronsPerM: 24545, CachedInputNeuronsPerM: 24545, OutputNeuronsPerM: 77273},
	"@cf/mistralai/mistral-small-3.1-24b-instruct": {InputNeuronsPerM: 31876, CachedInputNeuronsPerM: 31876, OutputNeuronsPerM: 50488},
	"@cf/google/gemma-3-12b-it":                    {InputNeuronsPerM: 31371, CachedInputNeuronsPerM: 31371, OutputNeuronsPerM: 50560},
	"@cf/meta/llama-3.3-70b-instruct-fp8-fast":     {InputNeuronsPerM: 26668, CachedInputNeuronsPerM: 26668, OutputNeuronsPerM: 204805},
	"@cf/ibm-granite/granite-4.0-h-micro":          {InputNeuronsPerM: 1542, CachedInputNeuronsPerM: 1542, OutputNeuronsPerM: 10158},
	"@cf/meta/llama-3.2-3b-instruct":               {InputNeuronsPerM: 4625, CachedInputNeuronsPerM: 4625, OutputNeuronsPerM: 30475},
	"@cf/meta/llama-3.2-1b-instruct":               {InputNeuronsPerM: 2457, CachedInputNeuronsPerM: 2457, OutputNeuronsPerM: 18252},
}

type cloudflareRunRequest struct {
	Messages    []types.OpenAIMessage `json:"messages"`
	MaxTokens   *int                  `json:"max_tokens,omitempty"`
	Temperature *float64              `json:"temperature,omitempty"`
	TopP        *float64              `json:"top_p,omitempty"`
	Stop        any                   `json:"stop,omitempty"`
}

type cloudflareRunResponseEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		Response any          `json:"response"`
		Usage    *types.Usage `json:"usage"`
	} `json:"result"`
}

func buildCloudflareRunURL(baseURL, accountID, model string) string {
	trimmedBase := strings.TrimRight(baseURL, "/")
	trimmedAccountID := strings.TrimSpace(accountID)
	trimmedModel := strings.TrimPrefix(strings.TrimSpace(model), "/")
	return fmt.Sprintf("%s/accounts/%s/ai/run/%s", trimmedBase, trimmedAccountID, trimmedModel)
}

func prepareCloudflareRequest(request types.ChatCompletionRequest) ([]byte, error) {
	request.Router = nil
	payload := cloudflareRunRequest{
		Messages:    request.Messages,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stop:        request.Stop,
	}
	if request.MaxTokens != nil {
		payload.MaxTokens = request.MaxTokens
	} else if request.MaxCompletionTokens != nil {
		payload.MaxTokens = request.MaxCompletionTokens
	}
	return json.Marshal(payload)
}

func parseCloudflareRunResponse(body []byte, model string) (*types.ChatCompletionResponse, error) {
	var envelope cloudflareRunResponseEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, errors.NewParseError(
			fmt.Sprintf("Failed to parse response from %s/%s", cloudflareProviderID, model),
			"json",
			cloudflareProviderID,
			model,
			truncateString(string(body), 500),
			err,
		)
	}

	if !envelope.Success && len(envelope.Errors) > 0 {
		return nil, &errors.ProviderError{
			Message:     envelope.Errors[0].Message,
			StatusCode:  502,
			IsRetryable: true,
		}
	}

	text := cloudflareVisibleText(envelope.Result.Response)
	if strings.TrimSpace(text) == "" {
		return nil, errors.NewEmptyResponseError(cloudflareProviderID, model, 200)
	}

	usage := envelope.Result.Usage
	if usage != nil && usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return &types.ChatCompletionResponse{
		ID:      fmt.Sprintf("cf-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ResponseMessage{
				Role:    "assistant",
				Content: cloudflareStringPtr(text),
			},
			FinishReason: "stop",
		}},
		Usage: usage,
	}, nil
}

func (s *ProviderService) callCloudflareProvider(
	baseURL, apiKey, model string,
	request types.ChatCompletionRequest,
	ctx context.Context,
	auth types.ProviderAuth,
) (*types.ChatCompletionResponse, error) {
	accountID := strings.TrimSpace(os.Getenv(cloudflareAccountIDEnv))
	if accountID == "" {
		return nil, fmt.Errorf("%s is required", cloudflareAccountIDEnv)
	}

	reqBody, err := prepareCloudflareRequest(request)
	if err != nil {
		return nil, err
	}

	url := buildCloudflareRunURL(baseURL, accountID, model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if err := s.setAuth(ctx, req, apiKey, auth); err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		if timeoutErr := requestTimeoutError(ctx); timeoutErr != nil {
			return nil, timeoutErr
		}
		return nil, wrapNetworkError(err, detectProvider(baseURL, cloudflareProviderType, auth), baseURL)
	}
	defer resp.Body.Close()

	return s.handleResponse(resp, baseURL, cloudflareProviderType, auth, request, model)
}

func cloudflareVisibleText(value any) string {
	parts := make([]string, 0)
	appendCloudflareVisibleText(value, &parts)
	return strings.Join(parts, "")
}

func appendCloudflareVisibleText(value any, parts *[]string) {
	switch typed := value.(type) {
	case string:
		if typed != "" {
			*parts = append(*parts, typed)
		}
	case []any:
		for _, item := range typed {
			appendCloudflareVisibleText(item, parts)
		}
	case map[string]any:
		if text, ok := typed["text"].(string); ok && text != "" {
			*parts = append(*parts, text)
			return
		}
		if response, ok := typed["response"]; ok {
			appendCloudflareVisibleText(response, parts)
			return
		}
		if content, ok := typed["content"]; ok {
			appendCloudflareVisibleText(content, parts)
		}
	}
}

func cloudflarePricingForModel(model string) (cloudflareModelPricing, bool) {
	pricing, ok := cloudflarePricingTable[model]
	return pricing, ok
}

func cloudflareUsageToNeuronStats(pricing cloudflareModelPricing, promptTokens, cachedTokens, completionTokens int) CloudflareUsageStats {
	nonCachedTokens := promptTokens - cachedTokens
	if nonCachedTokens < 0 {
		nonCachedTokens = 0
	}

	neurons := ((float64(nonCachedTokens) * float64(pricing.InputNeuronsPerM)) / 1_000_000.0) +
		((float64(cachedTokens) * float64(pricing.CachedInputNeuronsPerM)) / 1_000_000.0) +
		((float64(completionTokens) * float64(pricing.OutputNeuronsPerM)) / 1_000_000.0)

	roundedNeurons := int(math.Ceil(neurons))
	if roundedNeurons < 0 {
		roundedNeurons = 0
	}

	return CloudflareUsageStats{
		CachedInputTokens:    cachedTokens,
		NonCachedInputTokens: nonCachedTokens,
		Neurons:              roundedNeurons,
		EstimatedUSDIfPaid:   float64(roundedNeurons) * cloudflarePaidUSDPerThousand / 1000.0,
	}
}

func cloudflareEstimatedCompletionTokens(req types.ChatCompletionRequest) int {
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		return *req.MaxTokens
	}
	if req.MaxCompletionTokens != nil && *req.MaxCompletionTokens > 0 {
		return *req.MaxCompletionTokens
	}
	return cloudflareDefaultMaxTokens
}

func cloudflareStringPtr(s string) *string {
	return &s
}
