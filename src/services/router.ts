import { ChatCompletionRequest, ChatCompletionResponse, RouterHints, SSEChunk, GatewayError, ProviderConfig, Certification, DerivedRequirements, AppConfig, LogicalModelConfig } from '../types';
import { getProviderApiKey } from '../config';
import { quotaService, estimateTokens } from './quota';
import { healthService } from './health';
import { providerService } from './provider';
import { ProviderError, RateLimitError, CircuitBreakerError, TimeoutError, ValidationError, ModelQuotaExceededError, GatewayErrorClass } from '../errors';
import { logger } from '../utils/logger';

export interface RoutingCandidate {
  provider: ProviderConfig;
  model: string;
  isCertifiedForStrictSchema: boolean;
  score: number;
  scoreBreakdown: Record<string, number>;
}

export interface RoutingPlan {
  attempts: RoutingAttempt[];
  maxAttempts: number;
  hardTimeoutMs: number | undefined;
  retryOn429: boolean;
  retryOnTimeout: boolean;
  retryOn5xx: boolean;
}

export interface RoutingAttempt {
  providerId: string;
  model: string;
  baseUrl: string;
  apiKey: string | undefined;
  score: number;
  timeoutMs: number;
  providerType: 'openai' | 'vertex';
  auth: { type: 'none' | 'bearer' | 'header'; env: string; headerName?: string };
}

export interface ExecutionResult {
  response: ChatCompletionResponse;
  attempts: number;
  providerId: string;
  model: string;
  latencyMs: number;
}

export class RouterService {
  private config: AppConfig;

  constructor(config: AppConfig) {
    this.config = config;
  }

  deriveRequirements(request: ChatCompletionRequest, hints?: RouterHints): DerivedRequirements {
    return deriveRequirements(request, hints);
  }

  async generateCandidates(): Promise<RoutingCandidate[]> {
    const candidates: RoutingCandidate[] = [];

    for (const provider of this.config.providers) {
      for (const model of provider.models.list) {
        const isCertified = this.config.certifications.some(
          (c: Certification) => c.provider === provider.id && c.model === model && c.strictSchema
        );

        candidates.push({
          provider,
          model,
          isCertifiedForStrictSchema: isCertified,
          score: 0,
          scoreBreakdown: {},
        });
      }
    }

    return candidates;
  }

  /**
   * Generate candidates from a logical model configuration
   * Only includes candidates specified in the logical model's candidate list
   */
  async generateCandidatesFromLogicalModel(logicalModelConfig: LogicalModelConfig): Promise<RoutingCandidate[]> {
    const candidates: RoutingCandidate[] = [];

    for (const candidateConfig of logicalModelConfig.candidates) {
      // Find the provider in the config
      const provider = this.config.providers.find(p => p.id === candidateConfig.provider);
      if (!provider) {
        logger.warn({
          event: 'logical_model_provider_not_found',
          provider: candidateConfig.provider,
          model: candidateConfig.model,
          logical_model: logicalModelConfig.id,
        });
        continue;
      }

      // Check if the model exists for this provider
      if (!provider.models.list.includes(candidateConfig.model)) {
        logger.warn({
          event: 'logical_model_model_not_found',
          provider: candidateConfig.provider,
          model: candidateConfig.model,
          logical_model: logicalModelConfig.id,
        });
        continue;
      }

      // Check certification status
      const isCertified = this.config.certifications.some(
        (c: Certification) => c.provider === provider.id && c.model === candidateConfig.model && c.strictSchema
      );

      candidates.push({
        provider,
        model: candidateConfig.model,
        isCertifiedForStrictSchema: isCertified,
        score: candidateConfig.weight ?? 0.5, // Use weight from logical model as base score
        scoreBreakdown: { logicalModelWeight: candidateConfig.weight ?? 0.5 },
      });
    }

    logger.info({
      event: 'logical_model_candidates_generated',
      logical_model: logicalModelConfig.id,
      candidate_count: candidates.length,
    });

    return candidates;
  }

  async filterCandidates(
    candidates: RoutingCandidate[],
    requirements: DerivedRequirements,
    request: ChatCompletionRequest,
    hints?: RouterHints
  ): Promise<{ eligible: RoutingCandidate[]; filtered: Array<{ provider: string; model: string; reason: string }> }> {
    const eligible: RoutingCandidate[] = [];
    const filtered: Array<{ provider: string; model: string; reason: string }> = [];

    // Estimate tokens once for all candidates
    const estimatedTokens = estimateTokens(request);

    logger.debug({
      event: 'filter_candidates_start',
      candidate_count: candidates.length,
      requirements_output: requirements.output,
      requirements_streaming: requirements.streaming,
      requirements_tools: requirements.tools,
    });

    for (const candidate of candidates) {
      const { provider, model } = candidate;

      // Check allow/deny lists
      if (hints?.providers?.allow && !hints.providers.allow.includes(provider.id)) {
        logger.debug({
          event: 'candidate_filtered',
          provider: provider.id,
          model,
          reason: 'not_in_allowlist',
          allowlist: hints.providers.allow,
        });
        filtered.push({ provider: provider.id, model, reason: 'not_in_allowlist' });
        continue;
      }

      if (hints?.providers?.deny?.includes(provider.id)) {
        logger.debug({
          event: 'candidate_filtered',
          provider: provider.id,
          model,
          reason: 'in_denylist',
        });
        filtered.push({ provider: provider.id, model, reason: 'in_denylist' });
        continue;
      }

      // Check budget mode
      if (hints?.budget?.mode === 'free_only') {
        // This would need a flag on providers - skipping for now
      }

      // Check strict schema requirement
      if (requirements.output === 'json_schema_strict' && !candidate.isCertifiedForStrictSchema) {
        if (provider.capabilities.structuredOutputs !== 'json_schema_strict') {
          logger.debug({
            event: 'candidate_filtered',
            provider: provider.id,
            model,
            reason: 'not_certified_for_strict_schema',
            is_certified: candidate.isCertifiedForStrictSchema,
            structured_outputs: provider.capabilities.structuredOutputs,
          });
          filtered.push({ provider: provider.id, model, reason: 'not_certified_for_strict_schema' });
          continue;
        }
      }

      // Check streaming requirement
      if (requirements.streaming === 'required' && !provider.capabilities.streaming) {
        logger.debug({
          event: 'candidate_filtered',
          provider: provider.id,
          model,
          reason: 'streaming_not_supported',
          supports_streaming: provider.capabilities.streaming,
        });
        filtered.push({ provider: provider.id, model, reason: 'streaming_not_supported' });
        continue;
      }

      // Check tools requirement
      if (requirements.tools === 'required' && !provider.capabilities.tools) {
        logger.debug({
          event: 'candidate_filtered',
          provider: provider.id,
          model,
          reason: 'tools_not_supported',
          supports_tools: provider.capabilities.tools,
        });
        filtered.push({ provider: provider.id, model, reason: 'tools_not_supported' });
        continue;
      }

      // Check circuit breaker
      const canExecute = await healthService.canExecute(provider.id);
      if (!canExecute) {
        logger.debug({
          event: 'candidate_filtered',
          provider: provider.id,
          model,
          reason: 'circuit_breaker_open',
        });
        filtered.push({ provider: provider.id, model, reason: 'circuit_breaker_open' });
        continue;
      }

      // Check per-model quota
      const modelLimits = provider.models.limits?.[model] || {};
      try {
        await quotaService.checkModelQuota(provider.id, model, modelLimits, estimatedTokens);
        // Quota check passed, candidate is eligible
      } catch (error) {
        if (error instanceof ModelQuotaExceededError) {
          logger.debug({
            event: 'candidate_filtered',
            provider: provider.id,
            model,
            reason: `quota_exceeded_${error.limitType}`,
            limit_type: error.limitType,
          });
          filtered.push({ 
            provider: provider.id, 
            model, 
            reason: `quota_exceeded_${error.limitType}` 
          });
          continue;
        }
        // Re-throw other errors
        throw error;
      }

      eligible.push(candidate);
    }

    return { eligible, filtered };
  }

  scoreCandidates(candidates: RoutingCandidate[], hints?: RouterHints): RoutingCandidate[] {
    return scoreCandidates(candidates, hints);
  }

  compilePlan(
    candidates: RoutingCandidate[], 
    hints?: RouterHints, 
    logicalModelSLO?: { maxLatencyMs?: number; maxAttempts?: number }
  ): RoutingPlan {
    return compilePlan(candidates, this.config, hints, logicalModelSLO);
  }

  shouldRetry(error: ProviderError, plan: RoutingPlan, attemptIndex: number): boolean {
    return shouldRetry(error, plan, attemptIndex);
  }

  async execute(
    plan: RoutingPlan,
    request: ChatCompletionRequest,
    requestId: string
  ): Promise<ExecutionResult> {
    const startTime = Date.now();
    let lastError: Error | undefined;

    for (let i = 0; i < plan.attempts.length; i++) {
      const attempt = plan.attempts[i];
      
      logger.info({
        event: 'attempt_start',
        request_id: requestId,
        attempt: i + 1,
        provider: attempt.providerId,
        model: attempt.model,
      });

      try {
        const result = await this.executeAttempt(attempt, request, requestId, startTime);
        return result;
      } catch (error) {
        lastError = error instanceof Error ? error : new Error(String(error));
        const shouldContinue = await this.handleAttemptError(error, attempt, plan, i, requestId);
        if (!shouldContinue) {
          throw this.createGatewayError(lastError, i + 1);
        }
      }
    }

    throw this.createGatewayError(
      lastError || new Error('All attempts failed'),
      plan.attempts.length
    );
  }

  /**
   * Execute a single provider attempt
   * @returns ExecutionResult on success
   * @throws ProviderError on failure
   */
  private async executeAttempt(
    attempt: RoutingAttempt,
    request: ChatCompletionRequest,
    requestId: string,
    startTime: number
  ): Promise<ExecutionResult> {
    const attemptStartTime = Date.now();
    
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), attempt.timeoutMs);

    try {
      const response = await providerService.callProvider(
        attempt.baseUrl,
        attempt.apiKey,
        attempt.model,
        request,
        attempt.timeoutMs,
        controller.signal,
        attempt.providerType,
        attempt.auth
      );

      const latencyMs = Date.now() - attemptStartTime;

      // Record success
      await healthService.recordSuccess(attempt.providerId, latencyMs);
      const tokensUsed = response.usage?.total_tokens || 0;
      await quotaService.recordModelUsage(attempt.providerId, attempt.model, tokensUsed);

      // Check for refusal
      const hasRefusal = response.choices?.some(c => c.message?.refusal);
      if (hasRefusal) {
        logger.info({
          event: 'refusal_detected',
          request_id: requestId,
          provider: attempt.providerId,
          model: attempt.model,
        });
      }

      logger.info({
        event: 'attempt_success',
        request_id: requestId,
        attempt: 1,
        provider: attempt.providerId,
        model: attempt.model,
        latency_ms: latencyMs,
        tokens_used: tokensUsed,
      });

      return {
        response,
        attempts: 1,
        providerId: attempt.providerId,
        model: attempt.model,
        latencyMs: Date.now() - startTime,
      };
    } finally {
      clearTimeout(timeoutId);
    }
  }

  /**
   * Handle an error from a provider attempt
   * @returns true if we should continue to next attempt, false if we should throw
   */
  private async handleAttemptError(
    error: unknown,
    attempt: RoutingAttempt,
    plan: RoutingPlan,
    attemptIndex: number,
    requestId: string
  ): Promise<boolean> {
    const errorMessage = error instanceof Error ? error.message : String(error);

    logger.error({
      event: 'attempt_failed',
      request_id: requestId,
      attempt: attemptIndex + 1,
      provider: attempt.providerId,
      model: attempt.model,
      error: errorMessage,
    });

    // Record failure
    await healthService.recordFailure(attempt.providerId);

    // Handle specific error types
    if (error instanceof ModelQuotaExceededError) {
      logger.warn({
        event: 'model_quota_exceeded',
        request_id: requestId,
        provider: attempt.providerId,
        model: attempt.model,
        limit_type: error.limitType,
      });
      return true; // Continue to next attempt
    }

    if (error instanceof ProviderError) {
      // Handle 402 Payment Required - non-retryable
      if (error.statusCode === 402) {
        logger.error({
          event: 'payment_required',
          request_id: requestId,
          provider: attempt.providerId,
          model: attempt.model,
        });
        return false; // Don't continue, will throw
      }

      // Handle 429 rate limit
      if (error.statusCode === 429) {
        await this.syncRateLimitWithQuota(error, attempt, requestId);
      }

      return this.shouldRetry(error, plan, attemptIndex);
    }

    return true; // Continue for non-ProviderErrors
  }

  /**
   * Sync rate limit headers with quota service
   */
  private async syncRateLimitWithQuota(
    providerError: ProviderError,
    attempt: RoutingAttempt,
    requestId: string
  ): Promise<void> {
    try {
      const rateLimitHeaders = this.extractRateLimitHeadersFromError(providerError);
      await quotaService.handleProviderRateLimit(
        attempt.providerId,
        attempt.model,
        { headers: rateLimitHeaders, statusCode: 429 }
      );
    } catch (syncError) {
      logger.warn({
        event: 'rate_limit_sync_failed',
        request_id: requestId,
        provider: attempt.providerId,
        model: attempt.model,
        error: syncError instanceof Error ? syncError.message : String(syncError),
      });
    }
  }

  async executeStream(
    plan: RoutingPlan,
    request: ChatCompletionRequest,
    requestId: string,
    onChunk: (chunk: SSEChunk) => void,
    onComplete: () => void,
    onError: (error: GatewayError) => void
  ): Promise<void> {
    // Estimate tokens for quota tracking
    const estimatedTokens = estimateTokens(request);

    for (let i = 0; i < plan.attempts.length; i++) {
      const attempt = plan.attempts[i];

      try {
        const controller = new AbortController();

        await providerService.streamProvider(
          attempt.baseUrl,
          attempt.apiKey,
          attempt.model,
          request,
          attempt.timeoutMs,
          onChunk,
          controller.signal,
          attempt.providerType,
          attempt.auth
        );

        await healthService.recordSuccess(attempt.providerId, attempt.timeoutMs);
        
        // Record streaming usage - use estimated tokens since we don't have actual usage
        await quotaService.recordModelUsage(attempt.providerId, attempt.model, estimatedTokens);
        
        logger.info({
          event: 'streaming_attempt_success',
          request_id: requestId,
          attempt: i + 1,
          provider: attempt.providerId,
          model: attempt.model,
          estimated_tokens: estimatedTokens,
        });
        
        onComplete();
        return;
      } catch (error) {
        await healthService.recordFailure(attempt.providerId);

        // Handle specific error types
        if (error instanceof ModelQuotaExceededError) {
          logger.warn({
            event: 'streaming_model_quota_exceeded',
            request_id: requestId,
            provider: attempt.providerId,
            model: attempt.model,
            limit_type: error.limitType,
          });
          // Continue to next attempt
          continue;
        }

        if (error instanceof ProviderError) {
          const providerError = error as ProviderError;
          
          // Handle 402 Payment Required (OpenRouter specific) - non-retryable
          if (providerError.statusCode === 402) {
            logger.error({
              event: 'streaming_payment_required',
              request_id: requestId,
              provider: attempt.providerId,
              model: attempt.model,
            });
            onError(this.createGatewayError(error, i + 1));
            return;
          }

          // Handle 429 rate limit
          if (providerError.statusCode === 429) {
            try {
              const rateLimitHeaders = this.extractRateLimitHeadersFromError(providerError);
              await quotaService.handleProviderRateLimit(
                attempt.providerId,
                attempt.model,
                { headers: rateLimitHeaders, statusCode: 429 }
              );
            } catch (syncError) {
              logger.warn({
                event: 'streaming_rate_limit_sync_failed',
                request_id: requestId,
                provider: attempt.providerId,
                model: attempt.model,
                error: syncError instanceof Error ? syncError.message : String(syncError),
              });
            }
          }

          const shouldRetry = this.shouldRetry(error, plan, i);
          if (!shouldRetry) {
            onError(this.createGatewayError(error, i + 1));
            return;
          }
        }
      }
    }

    onError(this.createGatewayError(new Error('All streaming attempts failed'), plan.attempts.length));
  }

  private createGatewayError(error: Error, attempts: number): GatewayErrorClass {
    return createGatewayError(error, attempts);
  }

  /**
   * Extract rate limit headers from a ProviderError for quota synchronization
   * Converts internal RateLimitHeaders to the format expected by handleProviderRateLimit
   */
  private extractRateLimitHeadersFromError(
    providerError: ProviderError
  ): Record<string, string | string[] | undefined> {
    const rateLimitHeaders: Record<string, string | string[] | undefined> = {};
    
    if (providerError.headers) {
      if (providerError.headers.retryAfter !== undefined) {
        rateLimitHeaders['retry-after'] = String(providerError.headers.retryAfter);
      }
      if (providerError.headers.limitRequests !== undefined) {
        rateLimitHeaders['x-ratelimit-limit-requests'] = String(providerError.headers.limitRequests);
      }
      if (providerError.headers.remainingRequests !== undefined) {
        rateLimitHeaders['x-ratelimit-remaining-requests'] = String(providerError.headers.remainingRequests);
      }
      if (providerError.headers.resetRequests !== undefined) {
        rateLimitHeaders['x-ratelimit-reset-requests'] = providerError.headers.resetRequests;
      }
      if (providerError.headers.limitTokens !== undefined) {
        rateLimitHeaders['x-ratelimit-limit-tokens'] = String(providerError.headers.limitTokens);
      }
      if (providerError.headers.remainingTokens !== undefined) {
        rateLimitHeaders['x-ratelimit-remaining-tokens'] = String(providerError.headers.remainingTokens);
      }
      if (providerError.headers.resetTokens !== undefined) {
        rateLimitHeaders['x-ratelimit-reset-tokens'] = providerError.headers.resetTokens;
      }
    }
    
    return rateLimitHeaders;
  }
}

// Export pure functions for testing
export function deriveRequirements(request: ChatCompletionRequest, hints?: RouterHints): DerivedRequirements {
  let output: 'text' | 'json_schema_strict' = 'text';
  let streaming: 'required' | 'preferred' | 'forbidden' = 'preferred';
  let tools: 'required' | 'allowed' | 'forbidden' = 'forbidden';

  // Check for strict schema requirement
  if (request.response_format?.type === 'json_schema' && request.response_format.json_schema?.strict) {
    output = 'json_schema_strict';
  }

  // Streaming requirement
  if (request.stream === true) {
    streaming = 'required';
  } else if (request.stream === false) {
    streaming = 'forbidden';
  }

  // Tools requirement
  if (request.tools && request.tools.length > 0) {
    if (request.tool_choice === 'required') {
      tools = 'required';
    } else if (request.tool_choice === 'none') {
      tools = 'forbidden';
    } else {
      tools = 'allowed';
    }
  }

  // Override with hints if provided
  if (hints?.requirements?.output) {
    output = hints.requirements.output;
  }
  if (hints?.requirements?.streaming) {
    streaming = hints.requirements.streaming;
  }
  if (hints?.requirements?.tools) {
    tools = hints.requirements.tools;
  }

  return { output, streaming, tools };
}

export function scoreCandidates(candidates: RoutingCandidate[], hints?: RouterHints): RoutingCandidate[] {
  const weights = {
    base: 1.0,
    prefer: 0.5,
    quota: 0.3,
    health: 0.5,
  };

  const scored = candidates.map(candidate => {
    const breakdown: Record<string, number> = {};

    // Base weight
    breakdown.base = weights.base;

    // Preference bonus
    if (hints?.providers?.prefer) {
      const index = hints.providers.prefer.indexOf(candidate.provider.id);
      if (index !== -1) {
        breakdown.prefer = (1 - index / hints.providers.prefer.length) * weights.prefer;
      }
    }

    // Health score
    breakdown.health = weights.health * 0.5; // Default to neutral

    // Calculate total (preserve existing candidate score)
    const calculatedScore = Object.values(breakdown).reduce((sum, val) => sum + val, 0);
    const score = candidate.score + calculatedScore;

    return {
      ...candidate,
      score,
      scoreBreakdown: breakdown,
    };
  });

  // Sort by score descending
  scored.sort((a, b) => b.score - a.score);

  return scored;
}

export function compilePlan(
  candidates: RoutingCandidate[],
  config: AppConfig,
  hints?: RouterHints,
  logicalModelSLO?: { maxLatencyMs?: number; maxAttempts?: number }
): RoutingPlan {
  // Use logical model SLO as defaults, override with explicit hints
  const maxAttempts = hints?.fallback?.max_attempts ?? logicalModelSLO?.maxAttempts ?? 3;
  const hardTimeoutMs = hints?.slo?.hard_timeout_ms;

  const attempts: RoutingAttempt[] = candidates.slice(0, maxAttempts).map(candidate => {
    const timeoutMs = hints?.slo?.max_latency_ms ?? logicalModelSLO?.maxLatencyMs ?? 30000;

    return {
      providerId: candidate.provider.id,
      model: candidate.model,
      baseUrl: candidate.provider.baseUrl,
      apiKey: getProviderApiKey(candidate.provider.id, config),
      score: candidate.score,
      timeoutMs,
      providerType: candidate.provider.providerType ?? 'openai',
      auth: candidate.provider.auth,
    };
  });

  return {
    attempts,
    maxAttempts,
    hardTimeoutMs,
    retryOn429: hints?.fallback?.on_429 ?? true,
    retryOnTimeout: hints?.fallback?.on_timeout ?? true,
    retryOn5xx: hints?.fallback?.on_5xx ?? true,
  };
}

export function shouldRetry(error: ProviderError, plan: RoutingPlan, attemptIndex: number): boolean {
  if (attemptIndex >= plan.attempts.length - 1) {
    return false;
  }

  // Handle new error types
  if (error instanceof RateLimitError) {
    return plan.retryOn429;
  }
  if (error instanceof TimeoutError) {
    return plan.retryOnTimeout;
  }
  if (error instanceof CircuitBreakerError) {
    return true; // Circuit breaker errors are retryable (try another provider)
  }
  if (error instanceof ValidationError) {
    return false; // Validation errors are not retryable
  }
  // ModelQuotaExceededError is handled separately in execute methods
  // but if it reaches here, it should be retryable (try another provider)
  if (error instanceof ModelQuotaExceededError) {
    return true;
  }

  // Legacy handling based on statusCode
  const { statusCode } = error;
  const isTimeoutError = statusCode === 504 || statusCode === 499;

  // Handle 402 Payment Required - non-retryable
  if (statusCode === 402) {
    return false;
  }

  if (statusCode === 429 && plan.retryOn429) return true;
  // Timeout errors (504, 499) use only retryOnTimeout, not retryOn5xx
  if (isTimeoutError && plan.retryOnTimeout) return true;
  // General 5xx errors (excluding timeout errors) use retryOn5xx
  if (statusCode >= 500 && !isTimeoutError && plan.retryOn5xx) return true;

  return false;
}

export function createGatewayError(error: Error, attempts: number, requestId?: string): GatewayErrorClass {
  let code = 'UPSTREAM_ERROR';
  let details: { attempts: number; retryAfter?: number; limitType?: string; providerId?: string; state?: string; timeoutType?: string; validationErrors?: Array<{ path: string; message: string }>; } = { attempts };

  // Handle new error types
  if (error instanceof RateLimitError) {
    code = 'RATE_LIMITED';
    details.retryAfter = error.retryAfter;
    details.limitType = error.limitType;
  } else if (error instanceof CircuitBreakerError) {
    code = 'CIRCUIT_BREAKER_OPEN';
    details.providerId = error.providerId;
    details.state = error.state;
  } else if (error instanceof TimeoutError) {
    code = 'TIMEOUT';
    details.timeoutType = error.timeoutType;
  } else if (error instanceof ValidationError) {
    code = 'VALIDATION_ERROR';
    details.validationErrors = error.details;
  } else if (error instanceof ModelQuotaExceededError) {
    code = 'QUOTA_EXCEEDED';
    details.providerId = error.providerId;
    details.limitType = error.limitType;
  } else {
    // Legacy handling based on statusCode property
    const providerError = error as ProviderError;
    if (providerError.statusCode !== undefined) {
      if (providerError.statusCode === 429) {
        code = 'RATE_LIMITED';
      } else if (providerError.statusCode === 504 || providerError.statusCode === 499) {
        code = 'TIMEOUT';
      } else if (providerError.statusCode === 400) {
        code = 'INVALID_REQUEST';
      } else if (providerError.statusCode === 402) {
        code = 'PAYMENT_REQUIRED';
      }
    }
  }

  return new GatewayErrorClass(
    'gateway_error',
    code,
    error.message,
    requestId,
    details
  );
}

export function createRouter(config: AppConfig): RouterService {
  return new RouterService(config);
}
