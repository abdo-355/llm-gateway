import { EventEmitter } from 'events';
import { ChatCompletionRequest, ChatCompletionResponse, GatewayError, SSEChunk } from '../../core/openai/types';
import { RoutingPlan, RoutingAttempt } from '../../core/router/plan';
import { ProviderClient, ProviderError } from '../../providers/openaiCompatible/client';
import { QuotaManager } from '../../core/quota/manager';
import { HealthMonitor } from '../../core/health/monitor';
import { CircuitBreakerRegistry } from '../../core/health/breaker';
import { Logger } from '../../logging';

export interface ExecutionResult {
  response: ChatCompletionResponse;
  attempts: number;
  providerId: string;
  model: string;
  latencyMs: number;
}

export interface StreamExecutionCallbacks {
  onChunk: (chunk: SSEChunk) => void;
  onComplete: () => void;
  onError: (error: GatewayError) => void;
}

export class ExecutionEngine extends EventEmitter {
  private providerClient: ProviderClient;
  private quotaManager: QuotaManager;
  private healthMonitor: HealthMonitor;
  private circuitBreakers: CircuitBreakerRegistry;
  private logger: Logger;

  constructor(
    providerClient: ProviderClient,
    quotaManager: QuotaManager,
    healthMonitor: HealthMonitor,
    circuitBreakers: CircuitBreakerRegistry,
    logger: Logger
  ) {
    super();
    this.providerClient = providerClient;
    this.quotaManager = quotaManager;
    this.healthMonitor = healthMonitor;
    this.circuitBreakers = circuitBreakers;
    this.logger = logger;
  }

  async execute(plan: RoutingPlan, request: ChatCompletionRequest): Promise<ExecutionResult> {
    const startTime = Date.now();
    let lastError: Error | undefined;

    for (let i = 0; i < plan.attempts.length; i++) {
      const attempt = plan.attempts[i];
      const attemptStartTime = Date.now();

      this.logger.info({
        event: 'attempt_start',
        attempt: i + 1,
        provider: attempt.providerId,
        model: attempt.model,
      });

      try {
        // Check circuit breaker before attempting
        if (!this.circuitBreakers.canExecute(attempt.providerId)) {
          this.logger.warn({
            event: 'circuit_breaker_blocked',
            provider: attempt.providerId,
          });
          continue;
        }

        // Create abort controller for timeout
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), attempt.timeoutMs);

        const response = await this.providerClient.callChatCompletions(
          attempt.baseUrl,
          attempt.apiKey,
          attempt.model,
          request,
          attempt.timeoutMs,
          controller.signal
        );

        clearTimeout(timeoutId);

        const attemptLatencyMs = Date.now() - attemptStartTime;

        // Record success
        this.healthMonitor.recordSuccess(attempt.providerId, attemptLatencyMs);
        this.circuitBreakers.recordSuccess(attempt.providerId);
        this.quotaManager.recordRequest(attempt.providerId, response.usage?.total_tokens);

        // Check for refusal (for strict schema validation)
        const hasRefusal = response.choices?.some(c => c.message?.refusal);
        if (hasRefusal) {
          this.logger.info({
            event: 'refusal_detected',
            provider: attempt.providerId,
            model: attempt.model,
          });
          // Pass through refusals - don't retry
          return {
            response,
            attempts: i + 1,
            providerId: attempt.providerId,
            model: attempt.model,
            latencyMs: Date.now() - startTime,
          };
        }

        // Check for incomplete schema generation (for strict schema)
        const isIncomplete = this.checkIncompleteSchema(request, response);
        if (isIncomplete) {
          this.logger.warn({
            event: 'incomplete_schema_response',
            provider: attempt.providerId,
            model: attempt.model,
          });
          lastError = new Error('Incomplete schema response');
          continue; // Retry
        }

        this.logger.info({
          event: 'attempt_success',
          attempt: i + 1,
          provider: attempt.providerId,
          model: attempt.model,
          latency_ms: attemptLatencyMs,
        });

        return {
          response,
          attempts: i + 1,
          providerId: attempt.providerId,
          model: attempt.model,
          latencyMs: Date.now() - startTime,
        };

      } catch (error) {
        const attemptLatencyMs = Date.now() - attemptStartTime;
        lastError = error instanceof Error ? error : new Error(String(error));

        this.logger.error({
          event: 'attempt_failed',
          attempt: i + 1,
          provider: attempt.providerId,
          model: attempt.model,
          latency_ms: attemptLatencyMs,
          error: lastError.message,
        });

        // Record failure
        const isTimeout = lastError.message.includes('timeout') || lastError.message.includes('abort');
        this.healthMonitor.recordError(attempt.providerId, isTimeout);

        // Check if we should retry
        if (error instanceof ProviderError) {
          const shouldRetry = this.shouldRetry(error, plan, i);
          if (!shouldRetry) {
            throw this.createGatewayError(error, i + 1);
          }
        }
      }
    }

    // All attempts exhausted
    throw this.createGatewayError(
      lastError || new Error('All attempts failed'),
      plan.attempts.length
    );
  }

  async executeStream(
    plan: RoutingPlan,
    request: ChatCompletionRequest,
    callbacks: StreamExecutionCallbacks
  ): Promise<void> {
    for (let i = 0; i < plan.attempts.length; i++) {
      const attempt = plan.attempts[i];

      this.logger.info({
        event: 'stream_attempt_start',
        attempt: i + 1,
        provider: attempt.providerId,
        model: attempt.model,
      });

      try {
        if (!this.circuitBreakers.canExecute(attempt.providerId)) {
          continue;
        }

        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), attempt.timeoutMs);

        await this.providerClient.streamChatCompletions(
          attempt.baseUrl,
          attempt.apiKey,
          attempt.model,
          request,
          attempt.timeoutMs,
          (chunk) => {
            callbacks.onChunk(chunk);
          },
          controller.signal
        );

        clearTimeout(timeoutId);

        this.circuitBreakers.recordSuccess(attempt.providerId);
        callbacks.onComplete();
        return;

      } catch (error) {
        const isTimeout = error instanceof Error && 
          (error.message.includes('timeout') || error.message.includes('abort'));
        
        this.healthMonitor.recordError(attempt.providerId, isTimeout);

        if (error instanceof ProviderError) {
          const shouldRetry = this.shouldRetry(error, plan, i);
          if (!shouldRetry) {
            callbacks.onError(this.createGatewayError(error, i + 1));
            return;
          }
        }
      }
    }

    callbacks.onError(this.createGatewayError(
      new Error('All streaming attempts failed'),
      plan.attempts.length
    ));
  }

  private shouldRetry(error: ProviderError, plan: RoutingPlan, attemptIndex: number): boolean {
    // Check if we have more attempts available
    if (attemptIndex >= plan.attempts.length - 1) {
      return false;
    }

    const statusCode = error.statusCode;

    // 429 Too Many Requests
    if (statusCode === 429 && plan.retryOn429) {
      return true;
    }

    // 5xx Server Errors
    if (statusCode >= 500 && plan.retryOn5xx) {
      return true;
    }

    // Timeout / Abort (499)
    if ((statusCode === 499 || error.message.includes('timeout')) && plan.retryOnTimeout) {
      return true;
    }

    return false;
  }

  private checkIncompleteSchema(
    request: ChatCompletionRequest,
    response: ChatCompletionResponse
  ): boolean {
    // Check if strict schema was requested
    if (request.response_format?.type !== 'json_schema') {
      return false;
    }
    if (request.response_format.json_schema?.strict !== true) {
      return false;
    }

    // Check if any choice has a finish_reason indicating incomplete generation
    for (const choice of response.choices || []) {
      if (choice.finish_reason === 'length') {
        return true; // Truncated by max_tokens
      }
    }

    return false;
  }

  private createGatewayError(error: Error, attempts: number): GatewayError {
    let code = 'UPSTREAM_ERROR';
    let statusCode = 502;

    if (error instanceof ProviderError) {
      if (error.statusCode === 429) {
        code = 'RATE_LIMITED';
        statusCode = 429;
      } else if (error.statusCode >= 500) {
        code = 'UPSTREAM_ERROR';
        statusCode = 502;
      } else if (error.statusCode === 400) {
        code = 'INVALID_REQUEST';
        statusCode = 400;
      } else if (error.statusCode === 499 || error.message.includes('timeout')) {
        code = 'TIMEOUT';
        statusCode = 504;
      }
    }

    return {
      type: 'gateway_error',
      code,
      message: error.message,
      request_id: '', // Will be set by caller
      details: {
        attempts,
      },
    };
  }
}
