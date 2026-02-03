/**
 * Custom error classes for the LLM Gateway
 * All errors extend ProviderError for backward compatibility
 */

/**
 * Rate limit headers extracted from provider responses
 */
export interface RateLimitHeaders {
  retryAfter?: number;
  limitRequests?: number;
  remainingRequests?: number;
  resetRequests?: string;
  limitTokens?: number;
  remainingTokens?: number;
  resetTokens?: string;
  [key: string]: string | number | undefined;
}

export class ProviderError extends Error {
  constructor(
    message: string,
    public statusCode: number,
    public isRetryable: boolean = false,
    public headers?: RateLimitHeaders,
  ) {
    super(message);
    this.name = "ProviderError";
  }
}

export class ValidationError extends ProviderError {
  constructor(
    message: string,
    public details: Array<{ path: string; message: string }>,
  ) {
    super(message, 400, false);
    this.name = "ValidationError";
  }
}

export class RateLimitError extends ProviderError {
  constructor(
    message: string,
    public retryAfter: number, // seconds until reset
    public limitType: "rpm" | "tpm" | "daily",
  ) {
    super(message, 429, true);
    this.name = "RateLimitError";
  }
}

export class PaymentRequiredError extends ProviderError {
  constructor(message: string, headers?: RateLimitHeaders) {
    super(message, 402, false, headers);
    this.name = "PaymentRequiredError";
  }
}

export class CircuitBreakerError extends ProviderError {
  constructor(
    message: string,
    public providerId: string,
    public state: "OPEN" | "HALF_OPEN",
  ) {
    super(message, 503, true);
    this.name = "CircuitBreakerError";
  }
}

export class TimeoutError extends ProviderError {
  constructor(
    message: string,
    public timeoutType: "request" | "inactivity",
  ) {
    super(message, 504, true);
    this.name = "TimeoutError";
  }
}

export class ModelQuotaExceededError extends ProviderError {
  constructor(
    message: string,
    public providerId: string,
    public model: string,
    public limitType: "rpm" | "rph" | "rpd" | "tpm" | "tph" | "tpd" | "tpmu",
  ) {
    super(message, 429, true);
    this.name = "ModelQuotaExceededError";
  }
}

/**
 * GatewayError class - extends Error so instanceof checks work properly
 * This replaces the plain GatewayError interface for runtime errors
 */
export class GatewayErrorClass extends Error {
  public type: string;
  public code: string;
  public request_id?: string;
  public details?: { [key: string]: unknown };

  constructor(
    type: string,
    code: string,
    message: string,
    request_id?: string,
    details?: { [key: string]: unknown },
  ) {
    super(message);
    this.name = "GatewayError";
    this.type = type;
    this.code = code;
    this.request_id = request_id;
    this.details = details;
  }
}
