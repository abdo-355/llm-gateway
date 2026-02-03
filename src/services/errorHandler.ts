import { Response } from "express";
import { GatewayError } from "../types";
import {
  ModelQuotaExceededError,
  PaymentRequiredError,
  ProviderError,
  RateLimitError,
  GatewayErrorClass,
} from "../errors";
import { logger } from "../utils/logger";

/**
 * Service for handling and formatting API errors
 * Centralizes error response logic from the completions route
 */
export class ErrorHandlerService {
  /**
   * Build error details object based on error type
   */
  buildErrorDetails(error: unknown): Record<string, unknown> {
    const details: Record<string, unknown> = {};

    if (error instanceof ModelQuotaExceededError) {
      details.provider = error.providerId;
      details.model = error.model;
      details.limit_type = error.limitType;
    }

    if (error instanceof PaymentRequiredError) {
      details.message = "Payment required - please add credits to your account";
    }

    return details;
  }

  /**
   * Create a GatewayError from any error type
   */
  createGatewayError(
    error: unknown,
    requestId: string,
    details: Record<string, unknown>,
  ): GatewayError {
    if (error instanceof GatewayErrorClass) {
      return {
        type: error.type,
        code: error.code,
        message: error.message,
        request_id: error.request_id || requestId,
        details: error.details,
      };
    }

    if (error instanceof Error && "type" in error) {
      return error as unknown as GatewayError;
    }

    return {
      type: "gateway_error",
      code: "INTERNAL_ERROR",
      message: error instanceof Error ? error.message : "Unknown error",
      request_id: requestId,
      details: Object.keys(details).length > 0 ? details : undefined,
    };
  }

  /**
   * Merge error details into existing gateway error
   */
  mergeErrorDetails(
    gatewayError: GatewayError,
    error: unknown,
    details: Record<string, unknown>,
  ): GatewayError {
    if (
      error instanceof Error &&
      "type" in error &&
      Object.keys(details).length > 0
    ) {
      return {
        ...gatewayError,
        details: {
          ...(gatewayError.details || {}),
          ...details,
        },
      };
    }
    return gatewayError;
  }

  /**
   * Log error with appropriate context
   */
  logError(
    error: unknown,
    gatewayError: GatewayError,
    requestId: string,
  ): void {
    const isDev = process.env.NODE_ENV !== "production";

    logger.error({
      event: "request_failed",
      request_id: requestId,
      error: gatewayError.message,
      code: gatewayError.code,
      error_type: error instanceof Error ? error.constructor.name : "Unknown",
      ...(isDev && error instanceof Error && error.stack
        ? { stack: error.stack }
        : {}),
      ...(error instanceof ModelQuotaExceededError
        ? {
            provider: error.providerId,
            model: error.model,
            limit_type: error.limitType,
          }
        : {}),
      ...(error instanceof PaymentRequiredError
        ? {
            provider: "upstream",
          }
        : {}),
      ...(error instanceof ProviderError &&
      !(error instanceof ModelQuotaExceededError) &&
      !(error instanceof PaymentRequiredError)
        ? {
            status_code: error.statusCode,
            is_retryable: error.isRetryable,
          }
        : {}),
      ...(gatewayError.details
        ? { details: String(gatewayError.details) }
        : {}),
    });
  }

  /**
   * Determine HTTP status code from error
   */
  getStatusCode(error: unknown, gatewayError: GatewayError): number {
    if (error instanceof PaymentRequiredError) {
      return 402;
    }
    if (error instanceof ModelQuotaExceededError) {
      return 429;
    }

    switch (gatewayError.code) {
      case "TIMEOUT":
        return 504;
      case "RATE_LIMITED":
        return 429;
      case "CIRCUIT_BREAKER_OPEN":
        return 503;
      case "VALIDATION_ERROR":
        return 400;
      case "NO_ELIGIBLE_PROVIDER":
        return 422;
      default:
        if (error instanceof ProviderError) {
          return error.statusCode;
        }
        return 502;
    }
  }

  /**
   * Set appropriate headers for error response
   */
  setErrorHeaders(res: Response, error: unknown): void {
    if (error instanceof RateLimitError) {
      res.setHeader("Retry-After", String(error.retryAfter));
    } else if (error instanceof ModelQuotaExceededError) {
      if (error.headers?.retryAfter) {
        res.setHeader("Retry-After", String(error.headers.retryAfter));
      }
    } else if (error instanceof ProviderError && error.headers?.retryAfter) {
      res.setHeader("Retry-After", String(error.headers.retryAfter));
    }
  }

  /**
   * Handle complete error flow: build, log, and send response
   */
  handleError(error: unknown, requestId: string, res: Response): void {
    // Build error details
    const details = this.buildErrorDetails(error);

    // Create gateway error
    let gatewayError = this.createGatewayError(error, requestId, details);

    // Merge additional details
    gatewayError = this.mergeErrorDetails(gatewayError, error, details);

    // Log the error
    this.logError(error, gatewayError, requestId);

    // Get status code
    const statusCode = this.getStatusCode(error, gatewayError);

    // Set headers
    this.setErrorHeaders(res, error);

    // Send response
    res.status(statusCode).json({ error: gatewayError });
  }
}

// Singleton instance
export const errorHandler = new ErrorHandlerService();
