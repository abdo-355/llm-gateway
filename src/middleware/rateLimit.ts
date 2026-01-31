import { Request, Response, NextFunction } from 'express';
import { getRedisClient } from '../lib/redis';
import { GatewayError } from '../types';

const RATE_LIMIT_PREFIX = 'rate_limit:';

export async function rateLimitMiddleware(
  req: Request,
  res: Response,
  next: NextFunction
): Promise<void> {
  const maxRequests = parseInt(process.env.RATE_LIMIT_PER_IP || '100', 10);
  const windowMs = parseInt(process.env.RATE_LIMIT_WINDOW_MS || '60000', 10);
  
  // Get client IP
  const ip = req.ip || req.socket.remoteAddress || 'unknown';
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
        type: 'rate_limit_error',
        code: 'RATE_LIMIT_EXCEEDED',
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
    res.setHeader('X-RateLimit-Limit', maxRequests.toString());
    res.setHeader('X-RateLimit-Remaining', (maxRequests - count - 1).toString());
    
    next();
  } catch (err) {
    // If Redis fails, allow the request but log it
    console.error('Rate limit check failed:', err);
    next();
  }
}
