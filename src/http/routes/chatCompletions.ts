import { Router, Request, Response } from 'express';
import { ChatCompletionRequestSchema, ChatCompletionResponse, GatewayError, SSEChunk } from '../../core/openai/types';
import { RouterHints } from '../../core/router/types';
import { deriveRequirements } from '../../core/router/deriveRequirements';
import { generateCandidates, filterCandidates, Candidate } from '../../core/router/candidates';
import { scoreCandidates } from '../../core/router/score';
import { compilePlan } from '../../core/router/plan';
import { ExecutionEngine, ExecutionResult, StreamExecutionCallbacks } from '../../core/router/execute';
import { LoadedConfig, loadProviderEnvVars } from '../../config/loader';
import { QuotaManager } from '../../core/quota/manager';
import { HealthMonitor } from '../../core/health/monitor';
import { CircuitBreakerRegistry } from '../../core/health/breaker';
import { ProviderClient } from '../../providers/openaiCompatible/client';
import { Logger } from '../../logging';
import { RequestWithId } from '../middleware/requestId';

export function createChatCompletionsRouter(
  config: LoadedConfig,
  providerClient: ProviderClient,
  quotaManager: QuotaManager,
  healthMonitor: HealthMonitor,
  circuitBreakers: CircuitBreakerRegistry,
  logger: Logger
): Router {
  const router = Router();
  const executionEngine = new ExecutionEngine(
    providerClient,
    quotaManager,
    healthMonitor,
    circuitBreakers,
    logger
  );

  router.post('/chat/completions', async (req: Request, res: Response) => {
    const requestId = (req as RequestWithId).requestId;
    const requestLogger = logger.child({ request_id: requestId });

    const startTime = Date.now();

    // Stage A: Validate request
    const parseResult = ChatCompletionRequestSchema.safeParse(req.body);
    if (!parseResult.success) {
      requestLogger.warn({
        event: 'validation_error',
        errors: parseResult.error.errors,
      });
      return res.status(400).json({
        error: {
          type: 'invalid_request_error',
          code: 'validation_failed',
          message: 'Request validation failed',
          request_id: requestId,
          details: {
            errors: parseResult.error.errors,
          },
        },
      });
    }

    const request = parseResult.data;
    const routerHints: RouterHints | undefined = request.router;

    requestLogger.info({
      event: 'request_received',
      model_requested: request.model,
      stream: request.stream,
      has_router_hints: !!routerHints,
    });

    // Stage B: Derive hard requirements
    const requirements = deriveRequirements(request, routerHints);

    requestLogger.debug({
      event: 'derived_requirements',
      requirements,
    });

    // Stage C: Generate candidates
    const allCandidates = generateCandidates(config.providers, config.certifications);

    // Stage D: Hard filtering
    const quotaState = quotaManager.getState();
    const circuitBreakerState = new Map<string, boolean>();
    for (const provider of config.providers) {
      circuitBreakerState.set(provider.id, circuitBreakers.isOpen(provider.id));
    }

    const { candidates, filtered } = filterCandidates(
      allCandidates,
      requirements,
      routerHints,
      quotaState,
      circuitBreakerState
    );

    requestLogger.debug({
      event: 'candidates_filtered',
      eligible_count: candidates.length,
      filtered_count: filtered.length,
      filtered,
    });

    // Check if we have any eligible candidates
    if (candidates.length === 0) {
      requestLogger.error({
        event: 'no_eligible_provider',
        requirements,
        filtered,
      });
      return res.status(422).json({
        error: {
          type: 'gateway_error',
          code: 'NO_ELIGIBLE_PROVIDER',
          message: 'No eligible provider found for the given requirements',
          request_id: requestId,
          details: {
            requirements,
            filtered_providers: filtered,
          },
        },
      });
    }

    // Stage E: Soft scoring
    const healthScores = healthMonitor.getMetricsAll();
    const latencyScores = healthMonitor.getLatencyAll();
    const scoredCandidates = scoreCandidates(
      candidates,
      routerHints,
      quotaState,
      healthScores,
      latencyScores
    );

    requestLogger.debug({
      event: 'candidates_scored',
      top_candidate: scoredCandidates[0]?.provider.id,
      top_score: scoredCandidates[0]?.score,
    });

    // Stage F: Compile plan
    const plan = compilePlan(scoredCandidates, routerHints, (providerId) => {
      const provider = config.providers.find(p => p.id === providerId);
      return provider ? loadProviderEnvVars(provider) : undefined;
    });

    requestLogger.info({
      event: 'routing_plan',
      attempts_count: plan.attempts.length,
      providers: plan.attempts.map(a => a.providerId),
    });

    // Stage G: Execute
    try {
      if (request.stream) {
        await handleStreamingRequest(
          request,
          plan,
          executionEngine,
          requestId,
          res,
          requestLogger,
          startTime
        );
      } else {
        const result = await executionEngine.execute(plan, request);
        
        // Add gateway metadata to response
        result.response.system_fingerprint = `gateway_${requestId}`;
        
        requestLogger.info({
          event: 'request_complete',
          provider: result.providerId,
          model: result.model,
          attempts: result.attempts,
          latency_ms: Date.now() - startTime,
        });

        res.json(result.response);
      }
    } catch (error) {
      const gatewayError = error as GatewayError;
      gatewayError.request_id = requestId;

      requestLogger.error({
        event: 'request_failed',
        error: gatewayError.message,
        code: gatewayError.code,
      });

      const statusCode = gatewayError.code === 'TIMEOUT' ? 504 :
                         gatewayError.code === 'RATE_LIMITED' ? 429 :
                         gatewayError.code === 'INVALID_REQUEST' ? 400 : 502;

      res.status(statusCode).json({
        error: gatewayError,
      });
    }
  });

  return router;
}

async function handleStreamingRequest(
  request: any,
  plan: any,
  executionEngine: ExecutionEngine,
  requestId: string,
  res: Response,
  logger: Logger,
  startTime: number
): Promise<void> {
  res.setHeader('Content-Type', 'text/event-stream');
  res.setHeader('Cache-Control', 'no-cache');
  res.setHeader('Connection', 'keep-alive');

  const callbacks: StreamExecutionCallbacks = {
    onChunk: (chunk: SSEChunk) => {
      res.write(`data: ${JSON.stringify(chunk)}\n\n`);
    },
    onComplete: () => {
      res.write('data: [DONE]\n\n');
      res.end();
      
      logger.info({
        event: 'stream_complete',
        latency_ms: Date.now() - startTime,
      });
    },
    onError: (error: GatewayError) => {
      error.request_id = requestId;
      res.write(`event: error\ndata: ${JSON.stringify({ error })}\n\n`);
      res.end();
      
      logger.error({
        event: 'stream_error',
        error: error.message,
        code: error.code,
      });
    },
  };

  await executionEngine.executeStream(plan, request, callbacks);
}
