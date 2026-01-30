import { Pool } from 'undici';
import { ChatCompletionRequest, ChatCompletionResponse, SSEChunk } from '../../core/openai/types';
import { Logger } from '../logging';

export interface ProviderClient {
  callChatCompletions(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    abortSignal?: AbortSignal
  ): Promise<ChatCompletionResponse>;

  streamChatCompletions(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    onChunk: (chunk: SSEChunk) => void,
    abortSignal?: AbortSignal
  ): Promise<void>;
}

export class OpenAICompatibleClient implements ProviderClient {
  private pool: Pool;
  private logger: Logger;

  constructor(logger: Logger) {
    this.pool = new Pool('http://localhost', {
      connections: 10,
      keepAliveTimeout: 30000,
    });
    this.logger = logger;
  }

  private buildHeaders(apiKey: string | undefined): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    };
    if (apiKey) {
      headers['Authorization'] = `Bearer ${apiKey}`;
    }
    return headers;
  }

  private buildRequestBody(
    model: string,
    request: ChatCompletionRequest
  ): Record<string, any> {
    // Clone request and override model
    const body: Record<string, any> = {
      ...request,
      model,
    };
    // Remove router hints (internal only)
    delete body.router;
    return body;
  }

  async callChatCompletions(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    abortSignal?: AbortSignal
  ): Promise<ChatCompletionResponse> {
    const url = `${baseUrl}/chat/completions`;
    const body = this.buildRequestBody(model, request);
    const headers = this.buildHeaders(apiKey);

    const startTime = Date.now();

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers,
        body: JSON.stringify(body),
        signal: abortSignal,
      });

      const latencyMs = Date.now() - startTime;

      if (!response.ok) {
        const errorText = await response.text();
        this.logger.error({
          event: 'provider_error',
          provider: new URL(baseUrl).hostname,
          model,
          status: response.status,
          latency_ms: latencyMs,
          error: errorText,
        });
        throw new ProviderError(`Provider returned ${response.status}: ${errorText}`, response.status);
      }

      const data = await response.json();
      
      this.logger.info({
        event: 'provider_success',
        provider: new URL(baseUrl).hostname,
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
        throw new ProviderError('Request aborted', 499);
      }
      
      this.logger.error({
        event: 'provider_exception',
        provider: new URL(baseUrl).hostname,
        model,
        error: error instanceof Error ? error.message : String(error),
      });
      
      throw new ProviderError(`Failed to call provider: ${error}`, 500);
    }
  }

  async streamChatCompletions(
    baseUrl: string,
    apiKey: string | undefined,
    model: string,
    request: ChatCompletionRequest,
    timeoutMs: number,
    onChunk: (chunk: SSEChunk) => void,
    abortSignal?: AbortSignal
  ): Promise<void> {
    const url = `${baseUrl}/chat/completions`;
    const body = this.buildRequestBody(model, { ...request, stream: true });
    const headers = this.buildHeaders(apiKey);

    try {
      const response = await fetch(url, {
        method: 'POST',
        headers,
        body: JSON.stringify(body),
        signal: abortSignal,
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new ProviderError(`Provider returned ${response.status}: ${errorText}`, response.status);
      }

      if (!response.body) {
        throw new ProviderError('No response body for streaming', 500);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

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
                this.logger.warn({
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
        throw new ProviderError('Request aborted', 499);
      }
      
      throw new ProviderError(`Failed to stream from provider: ${error}`, 500);
    }
  }
}

export class ProviderError extends Error {
  public statusCode: number;
  public isRetryable: boolean;

  constructor(message: string, statusCode: number) {
    super(message);
    this.name = 'ProviderError';
    this.statusCode = statusCode;
    // Determine if error is retryable
    this.isRetryable = statusCode === 429 || statusCode >= 500 || statusCode === 499;
  }
}
