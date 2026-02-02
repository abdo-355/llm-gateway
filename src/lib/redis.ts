import Redis from 'ioredis';
import { logger } from '../utils/logger';

let redisClient: Redis | null = null;

export function getRedisClient(): Redis {
  if (!redisClient) {
    const redisUrl = process.env.REDIS_URL || 'redis://localhost:6379';
    const keyPrefix = process.env.REDIS_KEY_PREFIX || 'llm_gateway';

    redisClient = new Redis(redisUrl, {
      keyPrefix: keyPrefix + ':',
      retryStrategy: (times) => {
        const delay = Math.min(times * 50, 2000);
        return delay;
      },
      maxRetriesPerRequest: 3,
    });

    redisClient.on('error', (err) => {
      logger.error({ event: 'redis_error', error: err.message });
    });

    redisClient.on('connect', () => {
      logger.info({ event: 'redis_connected' });
    });
  }

  return redisClient;
}

export async function closeRedis(): Promise<void> {
  if (redisClient) {
    await redisClient.quit();
    redisClient = null;
  }
}
