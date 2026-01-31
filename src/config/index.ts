import { AppConfig } from '../types';
import { config } from './providers';

/**
 * Get the application configuration.
 * This now uses TypeScript objects instead of JSON for better type safety.
 */
export function loadConfig(): AppConfig {
  // Validate that required environment variables are set
  for (const provider of config.providers) {
    if (provider.auth.type !== 'none') {
      const envValue = process.env[provider.auth.env];
      if (!envValue) {
        throw new Error(
          `Provider ${provider.id}: Environment variable ${provider.auth.env} is not set`
        );
      }
    }
  }

  return config;
}

/**
 * Get the API key for a specific provider from environment variables.
 */
export function getProviderApiKey(
  providerId: string,
  config: AppConfig
): string | undefined {
  const provider = config.providers.find((p) => p.id === providerId);
  if (!provider) return undefined;

  if (provider.auth.type === 'none') {
    return undefined;
  }

  return process.env[provider.auth.env];
}
