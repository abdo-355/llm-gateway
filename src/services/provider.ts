import { ChatCompletionRequest, ChatCompletionResponse, ProviderAuth, ProviderType, SSEChunk, VertexAIResponse } from '../types';
import { logger } from '../utils/logger';
import { vertexAIAdapter } from './adapters/vertex';
import { ProviderError, TimeoutError, PaymentRequiredError, RateLimitHeaders } from '../errors';

export class ProviderService {
  /**
   * Extract rate limit headers from provider response
   * Handles various header naming conventions used by different providers
   */
  private extractRateLimitHeaders(response: Response): RateLimitHeaders {
    const headers: RateLimitHeaders = {};
    
    // Helper to get header value case-insensitively
    const getHeader = (name: string): string | null => {
      return response.headers.get(name);
    };

    // retry-after (seconds until rate limit resets)
    const retryAfter = getHeader('retry-after');
    if (retryAfter) {
      headers.retryAfter = parseInt(retryAfter, 10);
    }

    // x-ratelimit-* headers (OpenAI, OpenRouter style)
    const limitRequests = getHeader('x-ratelimit-limit-requests');
    if (limitRequests) {
      headers.limitRequests = parseInt(limitRequests, 10);
    }

    const remainingRequests = getHeader('x-ratelimit-remaining-requests');
    if (remainingRequests) {
      headers.remainingRequests = parseInt(remainingRequests, 10);
    }

    const resetRequests = getHeader('x-ratelimit-reset-requests');
    if (resetRequests) {
      headers.resetRequests = resetRequests;
    }

    const limitTokens = getHeader('x-ratelimit-limit-tokens');
    if (limitTokens) {
      headers.limitTokens = parseInt(limitTokens, 10);
    }

    const remainingTokens = getHeader('x-ratelimit-remaining-tokens');
    if (remainingTokens) {
      headers.remainingTokens = parseInt(remainingTokens, 10);
    }

    const resetTokens = getHeader('x-ratelimit-reset-tokens');
    if (resetTokens) {
      headers.resetTokens = resetTokens;
    }

    // Also check for x-ratelimit-* without the dash (some providers)
    if (!headers.limitRequests) {
      const altLimitRequests = getHeader('x-ratelimit-limit');
      if (altLimitRequests) {
        headers.limitRequests = parseInt(altLimitRequests, 10);
      }
    }

    if (!headers.remainingRequests) {
      const altRemaining = getHeader('x-ratelimit-remaining');
      if (altRemaining) {
        headers.remainingRequests = parseInt(altRemaining, 10);
      }
    }

    // Check for RateLimit-Reset (standard HTTP header)
    const rateLimitReset = getHeader('ratelimit-reset');
    if (rateLimitReset && !headers.resetRequests) {
      headers.resetRequests = rateLimitReset;
    }

    return headers;
  }

  /**
   * Determine if an error status code is retryable
   * 429: Rate limited (retryable with backoff)
   * 402: Payment required (non-retryable, needs credit top-up)
   * 5xx: Server errors (retryable)
   */
  private isRetryableStatus(status: number): boolean {
    // 402 Payment Required is NOT retryable - user needs to add credits
    if (status === 402) {
      return false;
    }
    // 429 is retryable with backoff
    // 5xx server errors are retryable
    return status === 429 || status >= 500;
  }

  async callProvider(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    abortSignal?: AbortSignal,
    providerType: ProviderType = 'openai',
    auth?: ProviderAuth
  ): Promise<ChatCompletionResponse> {
    if (providerType === 'vertex') {
      return this.callVertexProvider(
        baseUrl,
        apiKey,
        model,
        request,
        timeoutMs,
        abortSignal,
        auth
      );
    }

    return this.callOpenAIProvider(
      baseUrl,
      apiKey,
      model,
      request,
      timeoutMs,
      abortSignal,
      auth
    );
  }

  private async callOpenAIProvider(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    abortSignal?: AbortSignal,
    auth?: ProviderAuth
  ): Promise<ChatCompletionResponse> {
    const url = `${baseUrl}/chat/completions`;
    const startTime = Date.now();

    const body = {
      ...request,
      model,
    };

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    };

    if (apiKey) {
      if (auth?.type === 'header' && auth.headerName) {
        headers[auth.headerName] = apiKey;
      } else {
        headers['Authorization'] = `Bearer ${apiKey}`;
      }
    }

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers,
        body: JSON.stringify(body),
        signal: abortSignal || AbortSignal.timeout(timeoutMs),
      });

      const latencyMs = Date.now() - startTime;

      if (!response.ok) {
        const errorText = await response.text();
        const isRetryable = this.isRetryableStatus(response.status);
        const rateLimitHeaders = this.extractRateLimitHeaders(response);
        
        logger.error({
          event: 'provider_error',
          baseUrl,
          model,
          status: response.status,
          latency_ms: latencyMs,
          error: errorText,
          rate_limit_headers: rateLimitHeaders,
        });

        // Handle 402 Payment Required specially
        if (response.status === 402) {
          throw new PaymentRequiredError(
            `Provider returned 402 (Payment Required): ${errorText}`,
            rateLimitHeaders
          );
        }

        // For 429, include the rate limit headers so the quota service can sync
        throw new ProviderError(
          `Provider returned ${response.status}: ${errorText}`,
          response.status,
          isRetryable,
          rateLimitHeaders
        );
      }

      const data = await response.json() as ChatCompletionResponse;

      logger.info({
        event: 'provider_success',
        baseUrl,
        model,
        latency_ms: latencyMs,
        tokens_used: data.usage?.total_tokens,
      });

      return data;
    } catch (error) {
      if (error instanceof ProviderError) {
        throw error;
      }

      if (error instanceof Error && error.name === 'AbortError') {
        throw new TimeoutError('Request timed out', 'request');
      }

      logger.error({
        event: 'provider_exception',
        baseUrl,
        model,
        error: error instanceof Error ? error.message : String(error),
      });

      throw new ProviderError(
        `Failed to call provider: ${error}`,
        500,
        true
      );
    }
  }

  private async callVertexProvider(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    abortSignal?: AbortSignal,
    auth?: ProviderAuth
  ): Promise<ChatCompletionResponse> {
    const startTime = Date.now();
    const requestId = `vertex-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;

    const vertexRequest = vertexAIAdapter.transformRequest(request);
    const url = vertexAIAdapter.buildEndpointUrl(baseUrl, model, false);

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    };

    if (apiKey) {
      if (auth?.headerName) {
        headers[auth.headerName] = apiKey;
      } else {
        headers['x-goog-api-key'] = apiKey;
      }
    }

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers,
        body: JSON.stringify(vertexRequest),
        signal: abortSignal || AbortSignal.timeout(timeoutMs),
      });

      const latencyMs = Date.now() - startTime;

      if (!response.ok) {
        const errorText = await response.text();
        const isRetryable = this.isRetryableStatus(response.status);
        const rateLimitHeaders = this.extractRateLimitHeaders(response);
        
        logger.error({
          event: 'provider_error',
          baseUrl,
          model,
          status: response.status,
          latency_ms: latencyMs,
          error: errorText,
          provider_type: 'vertex',
          rate_limit_headers: rateLimitHeaders,
        });

        // Handle 402 Payment Required specially
        if (response.status === 402) {
          throw new PaymentRequiredError(
            `Provider returned 402 (Payment Required): ${errorText}`,
            rateLimitHeaders
          );
        }

        throw new ProviderError(
          `Provider returned ${response.status}: ${errorText}`,
          response.status,
          isRetryable,
          rateLimitHeaders
        );
      }

      const vertexResponse = await response.json() as VertexAIResponse;
      const data = vertexAIAdapter.transformResponse(vertexResponse, model, requestId);

      logger.info({
        event: 'provider_success',
        baseUrl,
        model,
        latency_ms: latencyMs,
        tokens_used: data.usage?.total_tokens,
        provider_type: 'vertex',
      });

      return data;
    } catch (error) {
      if (error instanceof ProviderError) {
        throw error;
      }

      if (error instanceof Error && error.name === 'AbortError') {
        throw new TimeoutError('Request timed out', 'request');
      }

      logger.error({
        event: 'provider_exception',
        baseUrl,
        model,
        error: error instanceof Error ? error.message : String(error),
        provider_type: 'vertex',
      });

      throw new ProviderError(
        `Failed to call provider: ${error}`,
        500,
        true
      );
    }
  }

  async streamProvider(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    onChunk: (chunk: SSEChunk) => void,
    abortSignal?: AbortSignal,
    providerType: ProviderType = 'openai',
    auth?: ProviderAuth
  ): Promise<void> {
    if (providerType === 'vertex') {
      return this.streamVertexProvider(
        baseUrl,
        apiKey,
        model,
        request,
        timeoutMs,
        onChunk,
        abortSignal,
        auth
      );
    }

    return this.streamOpenAIProvider(
      baseUrl,
      apiKey,
      model,
      request,
      timeoutMs,
      onChunk,
      abortSignal,
      auth
    );
  }

  private async streamOpenAIProvider(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    onChunk: (chunk: SSEChunk) => void,
    abortSignal?: AbortSignal,
    auth?: ProviderAuth
  ): Promise<void> {
    const url = `${baseUrl}/chat/completions`;
    
    const body = {
      ...request,
      model,
      stream: true,
    };

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    };

    if (apiKey) {
      if (auth?.type === 'header' && auth.headerName) {
        headers[auth.headerName] = apiKey;
      } else {
        headers['Authorization'] = `Bearer ${apiKey}`;
      }
    }

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers,
        body: JSON.stringify(body),
        signal: abortSignal || AbortSignal.timeout(timeoutMs),
      });

      if (!response.ok) {
        const errorText = await response.text();
        const isRetryable = this.isRetryableStatus(response.status);
        const rateLimitHeaders = this.extractRateLimitHeaders(response);

        // Handle 402 Payment Required specially
        if (response.status === 402) {
          throw new PaymentRequiredError(
            `Provider returned 402 (Payment Required): ${errorText}`,
            rateLimitHeaders
          );
        }

        throw new ProviderError(
          `Provider returned ${response.status}: ${errorText}`,
          response.status,
          isRetryable,
          rateLimitHeaders
        );
      }

      if (!response.body) {
        throw new ProviderError('No response body for streaming', 500);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let lastChunkTime = Date.now();
      const inactivityTimeoutMs = 60000; // 60 seconds

      try {
        while (true) {
          // Check for inactivity timeout before reading
          const timeSinceLastChunk = Date.now() - lastChunkTime;
          if (timeSinceLastChunk > inactivityTimeoutMs) {
            throw new TimeoutError(
              `Streaming inactivity timeout: no data received for ${inactivityTimeoutMs}ms`,
              'inactivity'
            );
          }

          const { done, value } = await reader.read();
          if (done) break;

          // Update last chunk time
          lastChunkTime = Date.now();

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed || trimmed === 'data: [DONE]') continue;

            if (trimmed.startsWith('data: ')) {
              try {
                const json = JSON.parse(trimmed.slice(6));
                onChunk(json);
              } catch (e) {
                logger.warn({
                  event: 'sse_parse_error',
                  line: trimmed,
                });
              }
            }
          }
        }
      } finally {
        reader.releaseLock();
      }
    } catch (error) {
      if (error instanceof ProviderError) {
        throw error;
      }

      if (error instanceof Error && error.name === 'AbortError') {
        throw new TimeoutError('Streaming request timed out', 'request');
      }

      throw new ProviderError(`Streaming failed: ${error}`, 500, true);
    }
  }

  private async streamVertexProvider(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    onChunk: (chunk: SSEChunk) => void,
    abortSignal?: AbortSignal,
    auth?: ProviderAuth
  ): Promise<void> {
    const vertexRequest = vertexAIAdapter.transformRequest(request);
    const url = vertexAIAdapter.buildEndpointUrl(baseUrl, model, true);

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    };

    if (apiKey) {
      if (auth?.headerName) {
        headers[auth.headerName] = apiKey;
      } else {
        headers['x-goog-api-key'] = apiKey;
      }
    }

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers,
        body: JSON.stringify(vertexRequest),
        signal: abortSignal || AbortSignal.timeout(timeoutMs),
      });

      if (!response.ok) {
        const errorText = await response.text();
        const isRetryable = this.isRetryableStatus(response.status);
        const rateLimitHeaders = this.extractRateLimitHeaders(response);

        // Handle 402 Payment Required specially
        if (response.status === 402) {
          throw new PaymentRequiredError(
            `Provider returned 402 (Payment Required): ${errorText}`,
            rateLimitHeaders
          );
        }

        throw new ProviderError(
          `Provider returned ${response.status}: ${errorText}`,
          response.status,
          isRetryable,
          rateLimitHeaders
        );
      }

      const responseText = await response.text();
      
      // Vertex AI streaming returns newline-delimited JSON objects
      const lines = responseText.split('\n').filter(line => line.trim());
      
      for (const line of lines) {
        try {
          const json: VertexAIResponse = JSON.parse(line);
          const chunk = vertexAIAdapter.transformStreamingChunk(json);
          if (chunk) {
            onChunk(chunk);
          }
        } catch (e) {
          logger.warn({
            event: 'vertex_streaming_parse_error',
            line: line.substring(0, 200),
          });
        }
      }
    } catch (error) {
      if (error instanceof ProviderError) {
        throw error;
      }

      if (error instanceof Error && error.name === 'AbortError') {
        throw new TimeoutError('Streaming request timed out', 'request');
      }

      throw new ProviderError(`Streaming failed: ${error}`, 500, true);
    }
  }
}

export const providerService = new ProviderService();
