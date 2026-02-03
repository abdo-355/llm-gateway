import { AppConfig } from "../types";
import { config } from "./providers";
import { getEnv } from "./env";

/**
 * Get the application configuration.
 * Environment variables are already validated at startup by validateAndLoadEnv().
 * This function just returns the provider config.
 */
export function loadConfig(): AppConfig {
  // Environment variables are validated in server.ts before this is called
  return config;
}

/**
 * Get the API key for a specific provider from environment variables.
 */
export function getProviderApiKey(
  providerId: string,
  config: AppConfig,
): string | undefined {
  const provider = config.providers.find((p) => p.id === providerId);
  if (!provider) return undefined;

  if (provider.auth.type === "none") {
    return undefined;
  }

  const env = getEnv();
  switch (provider.auth.env) {
    case "GROQ_API_KEY":
      return env.GROQ_API_KEY;
    case "CEREBRAS_API_KEY":
      return env.CEREBRAS_API_KEY;
    case "MISTRAL_API_KEY":
      return env.MISTRAL_API_KEY;
    case "GOOGLE_VERTEX_API_KEY":
      return env.GOOGLE_VERTEX_API_KEY;
    default:
      return undefined;
  }
}
