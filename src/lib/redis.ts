import Redis from 'ioredis';
import { logger } from '../utils/logger';
import { getEnv } from '../config/env';

let redisClient: Redis | null = null;

export function getRedisClient(): Redis {
  if (!redisClient) {
    const env = getEnv();
    const redisUrl = env.REDIS_URL;
    const keyPrefix = env.REDIS_KEY_PREFIX;

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
