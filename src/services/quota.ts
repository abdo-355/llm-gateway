import { getRedisClient } from '../lib/redis';
import { ModelQuotaExceededError } from '../errors';
import { ChatCompletionRequest, ModelLimits } from '../types';
import { logger } from '../utils/logger';

const QUOTA_PREFIX = 'quota:';

// Time window types for quota tracking
type LimitType = 'rpm' | 'rph' | 'rpd' | 'tpm' | 'tph' | 'tpd' | 'tpmu';

// Quota check result
export interface QuotaCheckResult {
  ok: boolean;
  allowed: boolean;
  current: ModelQuotaStatus;
  estimatedTokens: number;
}

// Current quota status for a model
export interface ModelQuotaStatus {
  rpm: number;
  rph: number;
  rpd: number;
  tpm: number;
  tph: number;
  tpd: number;
  tpmu: number;
  remainingRpm: number;
  remainingRph: number;
  remainingRpd: number;
  remainingTpm: number;
  remainingTph: number;
  remainingTpd: number;
  remainingTpmu: number;
}

// Rate limit response headers from provider
interface RateLimitHeaders {
  limit?: number;
  remaining?: number;
  reset?: number; // Unix timestamp
  retryAfter?: number; // Seconds
  requestsLimit?: number;
  requestsRemaining?: number;
  tokensLimit?: number;
  tokensRemaining?: number;
}

/**
 * Helper function to estimate tokens for a request
 * Rough estimate: 1 token ~ 4 characters
 * Sums message content lengths + max_tokens
 */
export function estimateTokens(request: ChatCompletionRequest): number {
  let estimatedChars = 0;

  // Sum message content lengths
  for (const message of request.messages) {
    if (typeof message.content === 'string') {
      estimatedChars += message.content.length;
    } else if (Array.isArray(message.content)) {
      // Handle array content (e.g., multimodal)
      for (const item of message.content) {
        if (item.type === 'text' && item.text) {
          estimatedChars += item.text.length;
        }
        // Image URLs don't add much to token count for estimation
      }
    }
  }

  // Add estimated completion tokens
  const maxTokens = request.max_tokens || request.max_completion_tokens || 1000;
  estimatedChars += maxTokens * 4; // Assume ~4 chars per token for output

  // Convert to tokens (rough estimate: 1 token ~ 4 characters)
  return Math.ceil(estimatedChars / 4);
}

export class QuotaService {
  private redis = getRedisClient();

  /**
   * Generate time-based suffixes for counter keys
   */
  private getTimeSuffixes(now: Date): {
    hourSuffix: string;
    daySuffix: string;
    monthSuffix: string;
  } {
    const year = now.getUTCFullYear();
    const month = String(now.getUTCMonth() + 1).padStart(2, '0');
    const day = String(now.getUTCDate()).padStart(2, '0');
    const hour = String(now.getUTCHours()).padStart(2, '0');

    return {
      hourSuffix: `${year}-${month}-${day}-${hour}`,
      daySuffix: `${year}-${month}-${day}`,
      monthSuffix: `${year}-${month}`,
    };
  }

  /**
   * Build Redis keys for a provider-model combination
   */
  private buildKeys(
    providerId: string,
    model: string,
    timeSuffixes: { hourSuffix: string; daySuffix: string; monthSuffix: string }
  ): Record<LimitType, string> {
    const base = `${QUOTA_PREFIX}${providerId}:${model}`;
    return {
      rpm: `${base}:rpm`,
      rph: `${base}:rph:${timeSuffixes.hourSuffix}`,
      rpd: `${base}:rpd:${timeSuffixes.daySuffix}`,
      tpm: `${base}:tpm`,
      tph: `${base}:tph:${timeSuffixes.hourSuffix}`,
      tpd: `${base}:tpd:${timeSuffixes.daySuffix}`,
      tpmu: `${base}:tpmu:${timeSuffixes.monthSuffix}`,
    };
  }

  /**
   * Calculate retry-after time based on which limit was exceeded
   */
  private calculateRetryAfter(limitType: LimitType, now: Date = new Date()): number {
    const currentSecond = Math.floor(now.getTime() / 1000) % 60;

    switch (limitType) {
      case 'rpm':
      case 'tpm':
        // Seconds until end of current minute
        return 60 - currentSecond;

      case 'rph':
      case 'tph':
        // Seconds until end of current hour
        return (60 - currentSecond) + (59 - now.getUTCMinutes()) * 60;

      case 'rpd':
      case 'tpd':
        // Seconds until midnight UTC
        const tomorrow = new Date(now);
        tomorrow.setUTCHours(24, 0, 0, 0);
        return Math.ceil((tomorrow.getTime() - now.getTime()) / 1000);

      case 'tpmu':
        // Seconds until end of calendar month
        const nextMonth = new Date(now.getUTCFullYear(), now.getUTCMonth() + 1, 1);
        nextMonth.setUTCHours(0, 0, 0, 0);
        return Math.ceil((nextMonth.getTime() - now.getTime()) / 1000);

      default:
        return 60;
    }
  }

  /**
   * Get count from sliding window (sorted set)
   * For RPM: count entries
   * For TPM: sum token counts from member values
   */
  private async getSlidingWindowCount(
    key: string,
    windowSeconds: number,
    isTokenWindow: boolean = false
  ): Promise<number> {
    const now = Date.now();
    const windowStart = now - windowSeconds * 1000;

    try {
      // Remove entries outside window
      await this.redis.zremrangebyscore(key, 0, windowStart);

      if (isTokenWindow) {
        // For TPM, we need to sum the token counts from member values
        // Members are stored as `${tokenCount}-${random}` with score=timestamp
        const members = await this.redis.zrangebyscore(key, windowStart, now);
        let totalTokens = 0;
        for (const member of members) {
          const tokenCount = parseInt(member.split('-')[0], 10);
          if (!isNaN(tokenCount)) {
            totalTokens += tokenCount;
          }
        }
        return totalTokens;
      } else {
        // For RPM, just count entries
        return await this.redis.zcard(key);
      }
    } catch (error) {
      logger.error({ event: 'quota_sliding_window_error', key, error: error instanceof Error ? error.message : String(error) });
      return 0;
    }
  }

  /**
   * Get current value of a counter
   */
  private async getCounter(key: string): Promise<number> {
    try {
      const value = await this.redis.get(key);
      return value ? parseInt(value, 10) : 0;
    } catch (error) {
      logger.error({ event: 'quota_get_counter_error', key, error: error instanceof Error ? error.message : String(error) });
      return 0;
    }
  }


  /**
   * Set counter to a specific value (used for syncing with provider)
   */
  private async setCounter(key: string, value: number, ttlSeconds: number = 86400): Promise<void> {
    try {
      const pipeline = this.redis.pipeline();
      pipeline.set(key, value.toString());
      pipeline.expire(key, ttlSeconds);
      await pipeline.exec();
    } catch (error) {
      logger.error({ event: 'quota_set_counter_error', key, error: error instanceof Error ? error.message : String(error) });
    }
  }

  /**
   * Check all quota limits for a specific provider-model combination
   * Throws RateLimitError if any limit is exceeded
   */
  async checkModelQuota(
    providerId: string,
    model: string,
    limits: ModelLimits,
    estimatedTokens: number
  ): Promise<QuotaCheckResult> {
    const now = new Date();
    const timeSuffixes = this.getTimeSuffixes(now);
    const keys = this.buildKeys(providerId, model, timeSuffixes);

    // Get current values for all limits
    const [
      rpm,
      rph,
      rpd,
      tpm,
      tph,
      tpd,
      tpmu,
    ] = await Promise.all([
      this.getSlidingWindowCount(keys.rpm, 60, false),
      this.getCounter(keys.rph),
      this.getCounter(keys.rpd),
      this.getSlidingWindowCount(keys.tpm, 60, true),
      this.getCounter(keys.tph),
      this.getCounter(keys.tpd),
      this.getCounter(keys.tpmu),
    ]);

    // Check RPM limit
    if (limits.rpm !== undefined && rpm + 1 > limits.rpm) {
      const retryAfter = this.calculateRetryAfter('rpm', now);
      throw new ModelQuotaExceededError(
        `RPM limit exceeded for ${providerId}/${model}: ${rpm}/${limits.rpm}. Retry after: ${retryAfter}s`,
        providerId,
        model,
        'rpm'
      );
    }

    // Check RPH limit
    if (limits.rph !== undefined && rph + 1 > limits.rph) {
      const retryAfter = this.calculateRetryAfter('rph', now);
      throw new ModelQuotaExceededError(
        `RPH limit exceeded for ${providerId}/${model}: ${rph}/${limits.rph}. Retry after: ${retryAfter}s`,
        providerId,
        model,
        'rph'
      );
    }

    // Check RPD limit
    if (limits.rpd !== undefined && rpd + 1 > limits.rpd) {
      const retryAfter = this.calculateRetryAfter('rpd', now);
      throw new ModelQuotaExceededError(
        `RPD limit exceeded for ${providerId}/${model}: ${rpd}/${limits.rpd}. Retry after: ${retryAfter}s`,
        providerId,
        model,
        'rpd'
      );
    }

    // Check TPM limit (using estimated tokens)
    if (limits.tpm !== undefined && tpm + estimatedTokens > limits.tpm) {
      const retryAfter = this.calculateRetryAfter('tpm', now);
      throw new ModelQuotaExceededError(
        `TPM limit exceeded for ${providerId}/${model}: ${tpm}/${limits.tpm} (estimated: ${estimatedTokens}). Retry after: ${retryAfter}s`,
        providerId,
        model,
        'tpm'
      );
    }

    // Check TPH limit
    if (limits.tph !== undefined && tph + estimatedTokens > limits.tph) {
      const retryAfter = this.calculateRetryAfter('tph', now);
      throw new ModelQuotaExceededError(
        `TPH limit exceeded for ${providerId}/${model}: ${tph}/${limits.tph} (estimated: ${estimatedTokens}). Retry after: ${retryAfter}s`,
        providerId,
        model,
        'tph'
      );
    }

    // Check TPD limit
    if (limits.tpd !== undefined && tpd + estimatedTokens > limits.tpd) {
      const retryAfter = this.calculateRetryAfter('tpd', now);
      throw new ModelQuotaExceededError(
        `TPD limit exceeded for ${providerId}/${model}: ${tpd}/${limits.tpd} (estimated: ${estimatedTokens}). Retry after: ${retryAfter}s`,
        providerId,
        model,
        'tpd'
      );
    }

    // Check TPMU limit
    if (limits.tpmu !== undefined && tpmu + estimatedTokens > limits.tpmu) {
      const retryAfter = this.calculateRetryAfter('tpmu', now);
      throw new ModelQuotaExceededError(
        `TPMU limit exceeded for ${providerId}/${model}: ${tpmu}/${limits.tpmu} (estimated: ${estimatedTokens}). Retry after: ${retryAfter}s`,
        providerId,
        model,
        'tpmu'
      );
    }

    // Calculate remaining quotas
    const current: ModelQuotaStatus = {
      rpm,
      rph,
      rpd,
      tpm,
      tph,
      tpd,
      tpmu,
      remainingRpm: limits.rpm !== undefined ? Math.max(0, limits.rpm - rpm) : Infinity,
      remainingRph: limits.rph !== undefined ? Math.max(0, limits.rph - rph) : Infinity,
      remainingRpd: limits.rpd !== undefined ? Math.max(0, limits.rpd - rpd) : Infinity,
      remainingTpm: limits.tpm !== undefined ? Math.max(0, limits.tpm - tpm) : Infinity,
      remainingTph: limits.tph !== undefined ? Math.max(0, limits.tph - tph) : Infinity,
      remainingTpd: limits.tpd !== undefined ? Math.max(0, limits.tpd - tpd) : Infinity,
      remainingTpmu: limits.tpmu !== undefined ? Math.max(0, limits.tpmu - tpmu) : Infinity,
    };

    return {
      ok: true,
      allowed: true,
      current,
      estimatedTokens,
    };
  }

  /**
   * Record actual usage after a request completes
   */
  async recordModelUsage(providerId: string, model: string, tokensUsed: number): Promise<void> {
    const now = new Date();
    const timestamp = now.getTime();
    const timeSuffixes = this.getTimeSuffixes(now);
    const keys = this.buildKeys(providerId, model, timeSuffixes);

    // Use pipeline for atomic operations
    const pipeline = this.redis.pipeline();

    // Add to RPM sliding window
    const rpmMember = `${timestamp}-${Math.random().toString(36).substring(2, 11)}`;
    pipeline.zadd(keys.rpm, timestamp, rpmMember);
    pipeline.expire(keys.rpm, 60);

    // Increment RPH counter (TTL: 2 hours)
    pipeline.incr(keys.rph);
    pipeline.expire(keys.rph, 7200);

    // Increment RPD counter (TTL: 25 hours)
    pipeline.incr(keys.rpd);
    pipeline.expire(keys.rpd, 90000);

    // Add to TPM sliding window with token count in member
    const tpmMember = `${tokensUsed}-${Math.random().toString(36).substring(2, 11)}`;
    pipeline.zadd(keys.tpm, timestamp, tpmMember);
    pipeline.expire(keys.tpm, 60);

    // Increment TPH counter
    pipeline.incrby(keys.tph, tokensUsed);
    pipeline.expire(keys.tph, 7200);

    // Increment TPD counter
    pipeline.incrby(keys.tpd, tokensUsed);
    pipeline.expire(keys.tpd, 90000);

    // Increment TPMU counter
    pipeline.incrby(keys.tpmu, tokensUsed);
    pipeline.expire(keys.tpmu, 2678400); // ~31 days

    try {
      await pipeline.exec();
    } catch (error) {
      logger.error({ event: 'quota_record_usage_error', provider: providerId, model, error: error instanceof Error ? error.message : String(error) });
      // Don't throw - recording usage is best-effort
    }
  }

  /**
   * Handle 429 rate limit response from provider
   * Syncs local counters with provider-reported state
   */
  async handleProviderRateLimit(
    providerId: string,
    model: string,
    response: { headers: Record<string, string | string[] | undefined>; statusCode: number }
  ): Promise<{ isRateLimited: boolean; retryAfter?: number; isPaymentRequired?: boolean }> {
    const { headers, statusCode } = response;

    // Handle 402 Payment Required (OpenRouter specific)
    if (statusCode === 402) {
      return {
        isRateLimited: true,
        isPaymentRequired: true,
        retryAfter: undefined, // Non-retryable
      };
    }

    // Only handle 429 responses
    if (statusCode !== 429) {
      return { isRateLimited: false };
    }

    const now = new Date();
    const timeSuffixes = this.getTimeSuffixes(now);
    const keys = this.buildKeys(providerId, model, timeSuffixes);

    // Parse rate limit headers
    const rateLimitHeaders = this.parseRateLimitHeaders(headers);

    // Sync counters with provider state if available
    if (rateLimitHeaders.requestsRemaining !== undefined) {
      // Provider reports remaining requests - sync our counters
      const rphUsed = rateLimitHeaders.requestsLimit
        ? rateLimitHeaders.requestsLimit - rateLimitHeaders.requestsRemaining
        : rateLimitHeaders.requestsLimit || 0;
      await this.setCounter(keys.rph, rphUsed, 7200);
    }

    if (rateLimitHeaders.tokensRemaining !== undefined) {
      // Provider reports remaining tokens - sync our counters
      const tphUsed = rateLimitHeaders.tokensLimit
        ? rateLimitHeaders.tokensLimit - rateLimitHeaders.tokensRemaining
        : rateLimitHeaders.tokensLimit || 0;
      await this.setCounter(keys.tph, tphUsed, 7200);
    }

    // Determine retry-after from headers or calculate
    let retryAfter: number | undefined;
    if (rateLimitHeaders.retryAfter !== undefined) {
      retryAfter = rateLimitHeaders.retryAfter;
    } else if (rateLimitHeaders.reset !== undefined) {
      retryAfter = Math.max(0, Math.ceil(rateLimitHeaders.reset - Date.now() / 1000));
    }

    return {
      isRateLimited: true,
      retryAfter,
    };
  }

  /**
   * Parse rate limit headers from provider response
   */
  private parseRateLimitHeaders(
    headers: Record<string, string | string[] | undefined>
  ): RateLimitHeaders {
    const result: RateLimitHeaders = {};

    // Helper to get header value (case-insensitive)
    const getHeader = (name: string): string | undefined => {
      const key = Object.keys(headers).find((k) => k.toLowerCase() === name.toLowerCase());
      return key ? (headers[key] as string) : undefined;
    };

    // Standard headers
    const limit = getHeader('x-ratelimit-limit');
    const remaining = getHeader('x-ratelimit-remaining');
    const reset = getHeader('x-ratelimit-reset');
    const retryAfter = getHeader('retry-after');

    // OpenAI-style headers
    const requestsLimit = getHeader('x-ratelimit-limit-requests');
    const requestsRemaining = getHeader('x-ratelimit-remaining-requests');
    const tokensLimit = getHeader('x-ratelimit-limit-tokens');
    const tokensRemaining = getHeader('x-ratelimit-remaining-tokens');

    if (limit) result.limit = parseInt(limit, 10);
    if (remaining) result.remaining = parseInt(remaining, 10);
    if (reset) result.reset = parseInt(reset, 10);
    if (retryAfter) result.retryAfter = parseInt(retryAfter, 10);
    if (requestsLimit) result.requestsLimit = parseInt(requestsLimit, 10);
    if (requestsRemaining) result.requestsRemaining = parseInt(requestsRemaining, 10);
    if (tokensLimit) result.tokensLimit = parseInt(tokensLimit, 10);
    if (tokensRemaining) result.tokensRemaining = parseInt(tokensRemaining, 10);

    return result;
  }

  /**
   * Get current quota status for a provider-model
   */
  async getModelQuotaStatus(providerId: string, model: string, limits: ModelLimits): Promise<ModelQuotaStatus> {
    const now = new Date();
    const timeSuffixes = this.getTimeSuffixes(now);
    const keys = this.buildKeys(providerId, model, timeSuffixes);

    const [
      rpm,
      rph,
      rpd,
      tpm,
      tph,
      tpd,
      tpmu,
    ] = await Promise.all([
      this.getSlidingWindowCount(keys.rpm, 60, false),
      this.getCounter(keys.rph),
      this.getCounter(keys.rpd),
      this.getSlidingWindowCount(keys.tpm, 60, true),
      this.getCounter(keys.tph),
      this.getCounter(keys.tpd),
      this.getCounter(keys.tpmu),
    ]);

    return {
      rpm,
      rph,
      rpd,
      tpm,
      tph,
      tpd,
      tpmu,
      remainingRpm: limits.rpm !== undefined ? Math.max(0, limits.rpm - rpm) : Infinity,
      remainingRph: limits.rph !== undefined ? Math.max(0, limits.rph - rph) : Infinity,
      remainingRpd: limits.rpd !== undefined ? Math.max(0, limits.rpd - rpd) : Infinity,
      remainingTpm: limits.tpm !== undefined ? Math.max(0, limits.tpm - tpm) : Infinity,
      remainingTph: limits.tph !== undefined ? Math.max(0, limits.tph - tph) : Infinity,
      remainingTpd: limits.tpd !== undefined ? Math.max(0, limits.tpd - tpd) : Infinity,
      remainingTpmu: limits.tpmu !== undefined ? Math.max(0, limits.tpmu - tpmu) : Infinity,
    };
  }
}

export const quotaService = new QuotaService();
