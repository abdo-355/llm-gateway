import { getRedisClient } from "../lib/redis";
import { ModelQuotaExceededError } from "../errors";
import { ChatCompletionRequest, ModelLimits } from "../types";
import { logger } from "../utils/logger";

const QUOTA_PREFIX = "quota:";

// Time window types for quota tracking
type LimitType = "rpm" | "rph" | "rpd" | "tpm" | "tph" | "tpd" | "tpmu";

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

export function estimateTokens(request: ChatCompletionRequest): number {
  let estimatedChars = 50;

  for (const message of request.messages) {
    estimatedChars += 15;

    if (typeof message.content === "string") {
      estimatedChars += message.content.length;
    } else if (Array.isArray(message.content)) {
      for (const item of message.content) {
        if (item.type === "text" && item.text) {
          estimatedChars += item.text.length;
        } else if (item.type === "image_url") {
          estimatedChars += 10;
        }
      }
    }

    if (message.tool_calls && message.tool_calls.length > 0) {
      for (const toolCall of message.tool_calls) {
        estimatedChars += 60;
        if (toolCall.function.name) {
          estimatedChars += toolCall.function.name.length;
        }
        if (toolCall.function.arguments) {
          estimatedChars += toolCall.function.arguments.length;
        }
      }
    }

    if (message.role === "tool") {
      estimatedChars += 30;
    }
  }

  const maxTokens = request.max_tokens || request.max_completion_tokens || 1000;
  estimatedChars += maxTokens * 4;

  return Math.ceil(estimatedChars / 4);
}

export class QuotaService {
  private redis = getRedisClient();

  private getTimeSuffixes(now: Date): {
    hourSuffix: string;
    daySuffix: string;
    monthSuffix: string;
  } {
    const year = now.getUTCFullYear();
    const month = String(now.getUTCMonth() + 1).padStart(2, "0");
    const day = String(now.getUTCDate()).padStart(2, "0");
    const hour = String(now.getUTCHours()).padStart(2, "0");

    return {
      hourSuffix: `${year}-${month}-${day}-${hour}`,
      daySuffix: `${year}-${month}-${day}`,
      monthSuffix: `${year}-${month}`,
    };
  }

  private buildKeys(
    providerId: string,
    model: string,
    timeSuffixes: {
      hourSuffix: string;
      daySuffix: string;
      monthSuffix: string;
    },
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

  private calculateRetryAfter(
    limitType: LimitType,
    now: Date = new Date(),
  ): number {
    const currentSecond = Math.floor(now.getTime() / 1000) % 60;

    switch (limitType) {
      case "rpm":
      case "tpm":
        return 60 - currentSecond;

      case "rph":
      case "tph":
        return 60 - currentSecond + (59 - now.getUTCMinutes()) * 60;

      case "rpd":
      case "tpd":
        const tomorrow = new Date(now);
        tomorrow.setUTCHours(24, 0, 0, 0);
        return Math.ceil((tomorrow.getTime() - now.getTime()) / 1000);

      case "tpmu":
        const nextMonth = new Date(
          now.getUTCFullYear(),
          now.getUTCMonth() + 1,
          1,
        );
        nextMonth.setUTCHours(0, 0, 0, 0);
        return Math.ceil((nextMonth.getTime() - now.getTime()) / 1000);

      default:
        return 60;
    }
  }

  private async getSlidingWindowCount(
    key: string,
    windowSeconds: number,
    isTokenWindow: boolean = false,
  ): Promise<number> {
    const now = Date.now();
    const windowStart = now - windowSeconds * 1000;

    try {
      const pipeline = this.redis.pipeline();
      pipeline.zremrangebyscore(key, 0, windowStart);

      if (isTokenWindow) {
        pipeline.zrangebyscore(key, windowStart, now);
        pipeline.zcard(key);
        const results = await pipeline.exec();

        if (!results) {
          return 0;
        }

        const members = (results[1] as any)?.[1] || [];
        let totalTokens = 0;
        for (const member of members) {
          const tokenCount = parseInt(member.split("-")[0], 10);
          if (!isNaN(tokenCount)) {
            totalTokens += tokenCount;
          }
        }
        return totalTokens;
      } else {
        pipeline.zcard(key);
        const results = await pipeline.exec();

        if (!results) {
          return 0;
        }

        return (results[1] as any)?.[1] || 0;
      }
    } catch (error) {
      logger.error({
        event: "quota_sliding_window_error",
        key,
        error: error instanceof Error ? error.message : String(error),
      });
      return 0;
    }
  }

  private async getCounter(key: string): Promise<number> {
    try {
      const value = await this.redis.get(key);
      return value ? parseInt(value, 10) : 0;
    } catch (error) {
      logger.error({
        event: "quota_get_counter_error",
        key,
        error: error instanceof Error ? error.message : String(error),
      });
      return 0;
    }
  }

  private async setCounter(
    key: string,
    value: number,
    ttlSeconds: number = 86400,
  ): Promise<void> {
    try {
      const pipeline = this.redis.pipeline();
      pipeline.set(key, value.toString());
      pipeline.expire(key, ttlSeconds);
      await pipeline.exec();
    } catch (error) {
      logger.error({
        event: "quota_set_counter_error",
        key,
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }

  async checkModelQuota(
    providerId: string,
    model: string,
    limits: ModelLimits,
    estimatedTokens: number,
  ): Promise<QuotaCheckResult> {
    const now = new Date();
    const timeSuffixes = this.getTimeSuffixes(now);
    const keys = this.buildKeys(providerId, model, timeSuffixes);

    const [rpm, rph, rpd, tpm, tph, tpd, tpmu] = await Promise.all([
      this.getSlidingWindowCount(keys.rpm, 60, false),
      this.getCounter(keys.rph),
      this.getCounter(keys.rpd),
      this.getSlidingWindowCount(keys.tpm, 60, true),
      this.getCounter(keys.tph),
      this.getCounter(keys.tpd),
      this.getCounter(keys.tpmu),
    ]);

    if (limits.rpm !== undefined && rpm + 1 > limits.rpm) {
      const retryAfter = this.calculateRetryAfter("rpm", now);
      throw new ModelQuotaExceededError(
        `RPM limit exceeded for ${providerId}/${model}: ${rpm}/${limits.rpm}. Retry after: ${retryAfter}s`,
        providerId,
        model,
        "rpm",
      );
    }

    if (limits.rph !== undefined && rph + 1 > limits.rph) {
      const retryAfter = this.calculateRetryAfter("rph", now);
      throw new ModelQuotaExceededError(
        `RPH limit exceeded for ${providerId}/${model}: ${rph}/${limits.rph}. Retry after: ${retryAfter}s`,
        providerId,
        model,
        "rph",
      );
    }

    if (limits.rpd !== undefined && rpd + 1 > limits.rpd) {
      const retryAfter = this.calculateRetryAfter("rpd", now);
      throw new ModelQuotaExceededError(
        `RPD limit exceeded for ${providerId}/${model}: ${rpd}/${limits.rpd}. Retry after: ${retryAfter}s`,
        providerId,
        model,
        "rpd",
      );
    }

    if (limits.tpm !== undefined && tpm + estimatedTokens > limits.tpm) {
      const retryAfter = this.calculateRetryAfter("tpm", now);
      throw new ModelQuotaExceededError(
        `TPM limit exceeded for ${providerId}/${model}: ${tpm}/${limits.tpm} (estimated: ${estimatedTokens}). Retry after: ${retryAfter}s`,
        providerId,
        model,
        "tpm",
      );
    }

    if (limits.tph !== undefined && tph + estimatedTokens > limits.tph) {
      const retryAfter = this.calculateRetryAfter("tph", now);
      throw new ModelQuotaExceededError(
        `TPH limit exceeded for ${providerId}/${model}: ${tph}/${limits.tph} (estimated: ${estimatedTokens}). Retry after: ${retryAfter}s`,
        providerId,
        model,
        "tph",
      );
    }

    if (limits.tpd !== undefined && tpd + estimatedTokens > limits.tpd) {
      const retryAfter = this.calculateRetryAfter("tpd", now);
      throw new ModelQuotaExceededError(
        `TPD limit exceeded for ${providerId}/${model}: ${tpd}/${limits.tpd} (estimated: ${estimatedTokens}). Retry after: ${retryAfter}s`,
        providerId,
        model,
        "tpd",
      );
    }

    if (limits.tpmu !== undefined && tpmu + estimatedTokens > limits.tpmu) {
      const retryAfter = this.calculateRetryAfter("tpmu", now);
      throw new ModelQuotaExceededError(
        `TPMU limit exceeded for ${providerId}/${model}: ${tpmu}/${limits.tpmu} (estimated: ${estimatedTokens}). Retry after: ${retryAfter}s`,
        providerId,
        model,
        "tpmu",
      );
    }

    const current: ModelQuotaStatus = {
      rpm,
      rph,
      rpd,
      tpm,
      tph,
      tpd,
      tpmu,
      remainingRpm:
        limits.rpm !== undefined ? Math.max(0, limits.rpm - rpm) : Infinity,
      remainingRph:
        limits.rph !== undefined ? Math.max(0, limits.rph - rph) : Infinity,
      remainingRpd:
        limits.rpd !== undefined ? Math.max(0, limits.rpd - rpd) : Infinity,
      remainingTpm:
        limits.tpm !== undefined ? Math.max(0, limits.tpm - tpm) : Infinity,
      remainingTph:
        limits.tph !== undefined ? Math.max(0, limits.tph - tph) : Infinity,
      remainingTpd:
        limits.tpd !== undefined ? Math.max(0, limits.tpd - tpd) : Infinity,
      remainingTpmu:
        limits.tpmu !== undefined ? Math.max(0, limits.tpmu - tpmu) : Infinity,
    };

    return {
      ok: true,
      allowed: true,
      current,
      estimatedTokens,
    };
  }

  async recordModelUsage(
    providerId: string,
    model: string,
    tokensUsed: number,
  ): Promise<void> {
    const now = new Date();
    const timestamp = now.getTime();
    const timeSuffixes = this.getTimeSuffixes(now);
    const keys = this.buildKeys(providerId, model, timeSuffixes);

    const pipeline = this.redis.pipeline();

    const rpmMember = `${timestamp}-${Math.random().toString(36).substring(2, 11)}`;
    pipeline.zadd(keys.rpm, timestamp, rpmMember);
    pipeline.expire(keys.rpm, 60);

    pipeline.incr(keys.rph);
    pipeline.expire(keys.rph, 7200);

    pipeline.incr(keys.rpd);
    pipeline.expire(keys.rpd, 90000);

    const tpmMember = `${tokensUsed}-${Math.random().toString(36).substring(2, 11)}`;
    pipeline.zadd(keys.tpm, timestamp, tpmMember);
    pipeline.expire(keys.tpm, 60);

    pipeline.incrby(keys.tph, tokensUsed);
    pipeline.expire(keys.tph, 7200);

    pipeline.incrby(keys.tpd, tokensUsed);
    pipeline.expire(keys.tpd, 90000);

    pipeline.incrby(keys.tpmu, tokensUsed);
    pipeline.expire(keys.tpmu, 2678400);

    try {
      await pipeline.exec();
    } catch (error) {
      logger.error({
        event: "quota_record_usage_error",
        provider: providerId,
        model,
        error: error instanceof Error ? error.message : String(error),
      });
    }
  }

  async handleProviderRateLimit(
    providerId: string,
    model: string,
    response: {
      headers: Record<string, string | string[] | undefined>;
      statusCode: number;
    },
  ): Promise<{
    isRateLimited: boolean;
    retryAfter?: number;
    isPaymentRequired?: boolean;
  }> {
    const { headers, statusCode } = response;

    if (statusCode === 402) {
      return {
        isRateLimited: true,
        isPaymentRequired: true,
        retryAfter: undefined,
      };
    }

    if (statusCode !== 429) {
      return { isRateLimited: false };
    }

    const now = new Date();
    const timeSuffixes = this.getTimeSuffixes(now);
    const keys = this.buildKeys(providerId, model, timeSuffixes);

    const rateLimitHeaders = this.parseRateLimitHeaders(headers);

    if (rateLimitHeaders.requestsRemaining !== undefined) {
      const rphUsed = rateLimitHeaders.requestsLimit
        ? rateLimitHeaders.requestsLimit - rateLimitHeaders.requestsRemaining
        : rateLimitHeaders.requestsLimit || 0;
      await this.setCounter(keys.rph, rphUsed, 7200);
    }

    if (rateLimitHeaders.tokensRemaining !== undefined) {
      const tphUsed = rateLimitHeaders.tokensLimit
        ? rateLimitHeaders.tokensLimit - rateLimitHeaders.tokensRemaining
        : rateLimitHeaders.tokensLimit || 0;
      await this.setCounter(keys.tph, tphUsed, 7200);
    }

    let retryAfter: number | undefined;
    if (rateLimitHeaders.retryAfter !== undefined) {
      retryAfter = rateLimitHeaders.retryAfter;
    } else if (rateLimitHeaders.reset !== undefined) {
      retryAfter = Math.max(
        0,
        Math.ceil(rateLimitHeaders.reset - Date.now() / 1000),
      );
    }

    return {
      isRateLimited: true,
      retryAfter,
    };
  }

  private parseRateLimitHeaders(
    headers: Record<string, string | string[] | undefined>,
  ): RateLimitHeaders {
    const result: RateLimitHeaders = {};

    const getHeader = (name: string): string | undefined => {
      const key = Object.keys(headers).find(
        (k) => k.toLowerCase() === name.toLowerCase(),
      );
      return key ? (headers[key] as string) : undefined;
    };

    const limit = getHeader("x-ratelimit-limit");
    const remaining = getHeader("x-ratelimit-remaining");
    const reset = getHeader("x-ratelimit-reset");
    const retryAfter = getHeader("retry-after");

    const requestsLimit = getHeader("x-ratelimit-limit-requests");
    const requestsRemaining = getHeader("x-ratelimit-remaining-requests");
    const tokensLimit = getHeader("x-ratelimit-limit-tokens");
    const tokensRemaining = getHeader("x-ratelimit-remaining-tokens");

    if (limit) result.limit = parseInt(limit, 10);
    if (remaining) result.remaining = parseInt(remaining, 10);
    if (reset) result.reset = parseInt(reset, 10);
    if (retryAfter) result.retryAfter = parseInt(retryAfter, 10);
    if (requestsLimit) result.requestsLimit = parseInt(requestsLimit, 10);
    if (requestsRemaining)
      result.requestsRemaining = parseInt(requestsRemaining, 10);
    if (tokensLimit) result.tokensLimit = parseInt(tokensLimit, 10);
    if (tokensRemaining) result.tokensRemaining = parseInt(tokensRemaining, 10);

    return result;
  }

  async getModelQuotaStatus(
    providerId: string,
    model: string,
    limits: ModelLimits,
  ): Promise<ModelQuotaStatus> {
    const now = new Date();
    const timeSuffixes = this.getTimeSuffixes(now);
    const keys = this.buildKeys(providerId, model, timeSuffixes);

    const [rpm, rph, rpd, tpm, tph, tpd, tpmu] = await Promise.all([
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
      remainingRpm:
        limits.rpm !== undefined ? Math.max(0, limits.rpm - rpm) : Infinity,
      remainingRph:
        limits.rph !== undefined ? Math.max(0, limits.rph - rph) : Infinity,
      remainingRpd:
        limits.rpd !== undefined ? Math.max(0, limits.rpd - rpd) : Infinity,
      remainingTpm:
        limits.tpm !== undefined ? Math.max(0, limits.tpm - tpm) : Infinity,
      remainingTph:
        limits.tph !== undefined ? Math.max(0, limits.tph - tph) : Infinity,
      remainingTpd:
        limits.tpd !== undefined ? Math.max(0, limits.tpd - tpd) : Infinity,
      remainingTpmu:
        limits.tpmu !== undefined ? Math.max(0, limits.tpmu - tpmu) : Infinity,
    };
  }
}

export const quotaService = new QuotaService();
