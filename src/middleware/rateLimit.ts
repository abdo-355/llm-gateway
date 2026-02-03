import { Request, Response, NextFunction } from "express";
import { getRedisClient } from "../lib/redis";
import { getEnv } from "../config/env";
import { logger } from "../utils/logger";
import { GatewayError } from "../types";

const RATE_LIMIT_PREFIX = "rate_limit:";

export async function rateLimitMiddleware(
  req: Request,
  res: Response,
  next: NextFunction,
): Promise<void> {
  const env = getEnv();
  const maxRequests = env.RATE_LIMIT_PER_IP;
  const windowMs = env.RATE_LIMIT_WINDOW_MS;

  // Get client IP
  const ip = req.ip || req.socket.remoteAddress || "unknown";
  const key = `${RATE_LIMIT_PREFIX}${ip}`;

  const redis = getRedisClient();
  const now = Date.now();
  const windowStart = now - windowMs;

  try {
    // Remove old entries outside the window
    await redis.zremrangebyscore(key, 0, windowStart);

    // Count current requests in window
    const count = await redis.zcard(key);

    if (count >= maxRequests) {
      const error: GatewayError = {
        type: "rate_limit_error",
        code: "RATE_LIMIT_EXCEEDED",
        message: `Rate limit exceeded. Maximum ${maxRequests} requests per ${windowMs / 1000} seconds.`,
        request_id: req.requestId,
      };
      res.status(429).json({ error });
      return;
    }

    // Add current request
    await redis.zadd(key, now, `${now}-${Math.random()}`);

    // Set expiry on the key
    await redis.expire(key, Math.ceil(windowMs / 1000));

    // Add rate limit headers
    res.setHeader("X-RateLimit-Limit", maxRequests.toString());
    res.setHeader(
      "X-RateLimit-Remaining",
      (maxRequests - count - 1).toString(),
    );

    next();
  } catch (err) {
    // If Redis fails, allow the request but log it
    logger.error({
      event: "rate_limit_check_failed",
      error: err instanceof Error ? err.message : String(err),
    });
    next();
  }
}
