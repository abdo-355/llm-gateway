import { Request, Response } from "express";
import { loadConfig } from "../config";
import { createRouter } from "../services/router";
import { ChatCompletionRequestSchema } from "../config/schema";
import { getLogicalModel } from "../config/logicalModels";
import { logger } from "../utils/logger";
import { errorHandler } from "../services/errorHandler";
import { GatewayError, JsonObject } from "../types";

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
    errorHandler.handleError(error, requestId, res);
  }
}
