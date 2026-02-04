import { logger } from "../utils/logger";
import { ProviderError, TimeoutError } from "../errors";
import { RateLimitHeaders } from "../errors";

export interface HttpClientOptions {
  timeoutMs: number;
  abortSignal?: AbortSignal;
}

export interface HttpClientResponse<T = unknown> {
  ok: boolean;
  status: number;
  data: T;
  headers: Record<string, string>;
  latencyMs: number;
}

export interface HttpClient {
  get<T = unknown>(
    url: string,
    options?: HttpClientOptions,
  ): Promise<HttpClientResponse<T>>;
  post<T = unknown>(
    url: string,
    body: unknown,
    options?: HttpClientOptions,
  ): Promise<HttpClientResponse<T>>;
}

function sanitizeUrlForLogging(url: string): string {
  try {
    const urlObj = new URL(url);
    return `${urlObj.protocol}//${urlObj.hostname}${urlObj.pathname}`;
  } catch {
    return "[invalid-url]";
  }
}

function extractRateLimitHeaders(response: Response): RateLimitHeaders {
  const headers: RateLimitHeaders = {};

  const getHeader = (name: string): string | null => {
    return response.headers.get(name);
  };

  const retryAfter = getHeader("retry-after");
  if (retryAfter) {
    headers.retryAfter = parseInt(retryAfter, 10);
  }

  const limitRequests = getHeader("x-ratelimit-limit-requests");
  if (limitRequests) {
    headers.limitRequests = parseInt(limitRequests, 10);
  }

  const remainingRequests = getHeader("x-ratelimit-remaining-requests");
  if (remainingRequests) {
    headers.remainingRequests = parseInt(remainingRequests, 10);
  }

  const resetRequests = getHeader("x-ratelimit-reset-requests");
  if (resetRequests) {
    headers.resetRequests = resetRequests;
  }

  const limitTokens = getHeader("x-ratelimit-limit-tokens");
  if (limitTokens) {
    headers.limitTokens = parseInt(limitTokens, 10);
  }

  const remainingTokens = getHeader("x-ratelimit-remaining-tokens");
  if (remainingTokens) {
    headers.remainingTokens = parseInt(remainingTokens, 10);
  }

  const resetTokens = getHeader("x-ratelimit-reset-tokens");
  if (resetTokens) {
    headers.resetTokens = resetTokens;
  }

  if (!headers.limitRequests) {
    const altLimitRequests = getHeader("x-ratelimit-limit");
    if (altLimitRequests) {
      headers.limitRequests = parseInt(altLimitRequests, 10);
    }
  }

  if (!headers.remainingRequests) {
    const altRemaining = getHeader("x-ratelimit-remaining");
    if (altRemaining) {
      headers.remainingRequests = parseInt(altRemaining, 10);
    }
  }

  const rateLimitReset = getHeader("ratelimit-reset");
  if (rateLimitReset && !headers.resetRequests) {
    headers.resetRequests = rateLimitReset;
  }

  return headers;
}

function isRetryableStatus(status: number): boolean {
  if (status === 402) {
    return false;
  }
  return status === 429 || status >= 500;
}

export function createHttpClient(): HttpClient {
  return {
    async get<T = unknown>(
      url: string,
      options?: HttpClientOptions,
    ): Promise<HttpClientResponse<T>> {
      const startTime = Date.now();

      try {
        const response = await fetch(url, {
          method: "GET",
          signal:
            options?.abortSignal ||
            AbortSignal.timeout(options?.timeoutMs || 30000),
        });

        const latencyMs = Date.now() - startTime;
        const data = (await response.json()) as T;

        const rateLimitHeaders = extractRateLimitHeaders(response);

        if (!response.ok) {
          const isRetryable = isRetryableStatus(response.status);
          logger.error({
            event: "http_get_error",
            url: sanitizeUrlForLogging(url),
            status: response.status,
            latency_ms: latencyMs,
            rate_limit_headers: rateLimitHeaders,
          });

          throw new ProviderError(
            `HTTP GET failed with status ${response.status}`,
            response.status,
            isRetryable,
            rateLimitHeaders,
          );
        }

        logger.info({
          event: "http_get_success",
          url: sanitizeUrlForLogging(url),
          latency_ms: latencyMs,
        });

        return {
          ok: true,
          status: response.status,
          data,
          headers: {},
          latencyMs,
        };
      } catch (error) {
        if (error instanceof ProviderError) {
          throw error;
        }

        if (error instanceof Error && error.name === "AbortError") {
          throw new TimeoutError("Request timed out", "request");
        }

        logger.error({
          event: "http_get_exception",
          url: sanitizeUrlForLogging(url),
          error: error instanceof Error ? error.message : String(error),
        });

        throw new ProviderError(`HTTP GET failed: ${error}`, 500, true);
      }
    },

    async post<T = unknown>(
      url: string,
      body: unknown,
      options?: HttpClientOptions,
    ): Promise<HttpClientResponse<T>> {
      const startTime = Date.now();

      try {
        const response = await fetch(url, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Accept: "application/json",
          },
          body: JSON.stringify(body),
          signal:
            options?.abortSignal ||
            AbortSignal.timeout(options?.timeoutMs || 30000),
        });

        const latencyMs = Date.now() - startTime;
        const data = (await response.json()) as T;
        const rateLimitHeaders = extractRateLimitHeaders(response);

        if (!response.ok) {
          const isRetryable = isRetryableStatus(response.status);
          logger.error({
            event: "http_post_error",
            url: sanitizeUrlForLogging(url),
            status: response.status,
            latency_ms: latencyMs,
            rate_limit_headers: rateLimitHeaders,
          });

          throw new ProviderError(
            `HTTP POST failed with status ${response.status}`,
            response.status,
            isRetryable,
            rateLimitHeaders,
          );
        }

        logger.info({
          event: "http_post_success",
          url: sanitizeUrlForLogging(url),
          latency_ms: latencyMs,
        });

        return {
          ok: true,
          status: response.status,
          data,
          headers: {},
          latencyMs,
        };
      } catch (error) {
        if (error instanceof ProviderError) {
          throw error;
        }

        if (error instanceof Error && error.name === "AbortError") {
          throw new TimeoutError("Request timed out", "request");
        }

        logger.error({
          event: "http_post_exception",
          url: sanitizeUrlForLogging(url),
          error: error instanceof Error ? error.message : String(error),
        });

        throw new ProviderError(`HTTP POST failed: ${error}`, 500, true);
      }
    },
  };
}

export { sanitizeUrlForLogging, extractRateLimitHeaders, isRetryableStatus };
