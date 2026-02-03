import { Request, Response } from "express";
import { loadConfig } from "../config";
import { createRouter } from "../services/router";
import { ChatCompletionRequestSchema } from "../config/schema";
import { getLogicalModel } from "../config/logicalModels";
import { GatewayError, JsonObject } from "../types";
import { logger } from "../utils/logger";
import {
  RateLimitError,
  ModelQuotaExceededError,
  PaymentRequiredError,
  ProviderError,
  GatewayErrorClass,
} from "../errors";

export async function completionsHandler(
  req: Request,
  res: Response,
): Promise<void> {
  const requestId = req.requestId;
  const startTime = Date.now();

  // Validate request
  const parseResult = ChatCompletionRequestSchema.safeParse(req.body);
  if (!parseResult.success) {
    const error: GatewayError = {
      type: "validation_error",
      code: "VALIDATION_FAILED",
      message: "Request validation failed",
      request_id: requestId,
      details: {
        errors: parseResult.error.errors.map((e) => ({
          path: e.path.join("."),
          message: e.message,
        })),
      },
    };
    res.status(400).json({ error });
    return;
  }

  const request = parseResult.data;
  const routerHints = request.router;
  const modelId = request.model;

  // Check if the requested model is a logical model
  const logicalModelConfig = getLogicalModel(modelId);
  const isLogical = !!logicalModelConfig;

  logger.info({
    event: "request_received",
    request_id: requestId,
    model_requested: modelId,
    is_logical_model: isLogical,
    logical_model_id: isLogical ? modelId : undefined,
    stream: request.stream,
    has_router_hints: !!routerHints,
  });

  try {
    const config = loadConfig();
    const router = createRouter(config);

    // Derive requirements
    const requirements = router.deriveRequirements(request, routerHints);

    // Generate candidates
    let candidates;
    if (logicalModelConfig) {
      // Use logical model's candidate list
      candidates =
        await router.generateCandidatesFromLogicalModel(logicalModelConfig);
    } else {
      // Legacy: Generate all candidates from all providers
      candidates = await router.generateCandidates();
    }

    // Filter candidates
    const { eligible, filtered } = await router.filterCandidates(
      candidates,
      requirements,
      request,
      routerHints,
    );

    if (eligible.length === 0) {
      const errorDetails: JsonObject = {
        requirements: requirements as unknown as JsonObject,
        filtered_providers: filtered,
      };

      if (logicalModelConfig) {
        errorDetails.logical_model = modelId;
        errorDetails.task_type = logicalModelConfig.taskType;
      }

      const error: GatewayError = {
        type: "gateway_error",
        code: "NO_ELIGIBLE_PROVIDER",
        message: isLogical
          ? `No eligible provider found for logical model '${modelId}'`
          : "No eligible provider found for the given requirements",
        request_id: requestId,
        details: errorDetails,
      };
      res.status(422).json({ error });
      return;
    }

    // Score and compile plan
    const scored = router.scoreCandidates(eligible, routerHints);

    // Use logical model SLO as defaults if available
    const logicalModelSLO = logicalModelConfig?.slo;
    const plan = router.compilePlan(scored, routerHints, logicalModelSLO);

    logger.info({
      event: "routing_plan",
      request_id: requestId,
      attempts_count: plan.attempts.length,
      providers: plan.attempts.map((a) => a.providerId),
    });

    // Handle streaming
    if (request.stream) {
      res.setHeader("Content-Type", "text/event-stream");
      res.setHeader("Cache-Control", "no-cache");
      res.setHeader("Connection", "keep-alive");

      await router.executeStream(
        plan,
        request,
        requestId,
        (chunk) => {
          res.write(`data: ${JSON.stringify(chunk)}\n\n`);
        },
        () => {
          res.write("data: [DONE]\n\n");
          res.end();
          logger.info({
            event: "stream_complete",
            request_id: requestId,
            latency_ms: Date.now() - startTime,
          });
        },
        (error) => {
          error.request_id = requestId;
          res.write(`event: error\ndata: ${JSON.stringify({ error })}\n\n`);
          res.end();
          logger.error({
            event: "stream_error",
            request_id: requestId,
            error: error.message,
            code: error.code,
          });
        },
      );
      return;
    }

    // Execute non-streaming request
    const result = await router.execute(plan, request, requestId);

    // Add gateway metadata
    result.response.system_fingerprint = `gateway_${requestId}`;

    // Add response headers for observability
    res.setHeader("X-Gateway-Provider", result.providerId);
    res.setHeader("X-Gateway-Model", result.model);
    if (isLogical) {
      res.setHeader("X-Gateway-Logical-Model", modelId);
    }
    res.setHeader("X-Gateway-Attempts", String(result.attempts));

    logger.info({
      event: "request_complete",
      request_id: requestId,
      provider: result.providerId,
      model: result.model,
      logical_model: isLogical ? modelId : undefined,
      attempts: result.attempts,
      latency_ms: Date.now() - startTime,
    });

    res.json(result.response);
  } catch (error) {
    // Build error details including provider/model info for model-specific errors
    const errorDetails: JsonObject = {};

    // Handle ModelQuotaExceededError - includes provider, model, and limit type
    if (error instanceof ModelQuotaExceededError) {
      errorDetails.provider = error.providerId;
      errorDetails.model = error.model;
      errorDetails.limit_type = error.limitType;
    }

    // Handle PaymentRequiredError - includes helpful message about credits
    if (error instanceof PaymentRequiredError) {
      errorDetails.message =
        "Payment required - please add credits to your account";
    }

    // Handle GatewayErrorClass (errors thrown from router)
    let gatewayError: GatewayError;

    if (error instanceof GatewayErrorClass) {
      // Error already has all properties, just add request_id if missing
      gatewayError = {
        type: error.type,
        code: error.code,
        message: error.message,
        request_id: error.request_id || requestId,
        details: error.details,
      };
    } else if (error instanceof Error && "type" in error) {
      gatewayError = error as unknown as GatewayError;
    } else {
      gatewayError = {
        type: "gateway_error",
        code: "INTERNAL_ERROR",
        message: error instanceof Error ? error.message : "Unknown error",
        request_id: requestId,
        details:
          Object.keys(errorDetails).length > 0 ? errorDetails : undefined,
      };
    }

    // Merge error details if the error already has a type (is a known error type)
    if (
      error instanceof Error &&
      "type" in error &&
      Object.keys(errorDetails).length > 0
    ) {
      gatewayError.details = {
        ...(gatewayError.details || {}),
        ...errorDetails,
      };
    }

    // Enhanced error logging with stack trace for debugging
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

    // Determine HTTP status code based on error type
    const statusCode =
      error instanceof PaymentRequiredError
        ? 402
        : error instanceof ModelQuotaExceededError
          ? 429
          : gatewayError.code === "TIMEOUT"
            ? 504
            : gatewayError.code === "RATE_LIMITED"
              ? 429
              : gatewayError.code === "CIRCUIT_BREAKER_OPEN"
                ? 503
                : gatewayError.code === "VALIDATION_ERROR"
                  ? 400
                  : gatewayError.code === "NO_ELIGIBLE_PROVIDER"
                    ? 422
                    : error instanceof ProviderError
                      ? error.statusCode
                      : 502;

    // Set Retry-After header for rate limit errors (both RateLimitError and ModelQuotaExceededError)
    if (error instanceof RateLimitError) {
      res.setHeader("Retry-After", String(error.retryAfter));
    } else if (error instanceof ModelQuotaExceededError) {
      // ModelQuotaExceededError extends ProviderError with statusCode 429
      // Retry-After info may be available via headers if set by the router
      if (error.headers?.retryAfter) {
        res.setHeader("Retry-After", String(error.headers.retryAfter));
      }
    } else if (error instanceof ProviderError && error.headers?.retryAfter) {
      res.setHeader("Retry-After", String(error.headers.retryAfter));
    }

    res.status(statusCode).json({ error: gatewayError });
  }
}
