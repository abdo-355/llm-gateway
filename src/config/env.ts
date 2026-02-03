/**
 * Centralized environment configuration
 * All environment variables are loaded and validated here
 */
import { config } from "dotenv";

config();

export interface EnvConfig {
  // Required API Keys
  GATEWAY_API_KEY: string;
  GROQ_API_KEY: string;
  CEREBRAS_API_KEY: string;
  MISTRAL_API_KEY: string;
  GOOGLE_VERTEX_API_KEY: string;

  // Server configuration
  PORT: number;
  NODE_ENV: string;

  // Redis configuration
  REDIS_URL: string;
  REDIS_KEY_PREFIX: string;

  // Logging
  LOG_LEVEL: string;

  // Rate limiting
  RATE_LIMIT_PER_IP: number;
  RATE_LIMIT_WINDOW_MS: number;

  // CORS
  CORS_ORIGINS: string;
}

/**
 * Validates all required environment variables
 * Throws an error with detailed message if any are missing
 * Does NOT log any sensitive values
 */
export function validateAndLoadEnv(): EnvConfig {
  const required = [
    "GATEWAY_API_KEY",
    "GROQ_API_KEY",
    "CEREBRAS_API_KEY",
    "MISTRAL_API_KEY",
    "GOOGLE_VERTEX_API_KEY",
  ];

  const missing = required.filter((key) => !process.env[key]);

  if (missing.length > 0) {
    throw new Error(
      `❌ Environment validation failed - Server will not start\n\n` +
        `Missing required environment variables:\n${missing.map((m) => `  ✗ ${m}`).join("\n")}\n\n` +
        `Please set all required variables and restart the server.`,
    );
  }

  return {
    // Required (validated above)
    GATEWAY_API_KEY: process.env.GATEWAY_API_KEY!,
    GROQ_API_KEY: process.env.GROQ_API_KEY!,
    CEREBRAS_API_KEY: process.env.CEREBRAS_API_KEY!,
    MISTRAL_API_KEY: process.env.MISTRAL_API_KEY!,
    GOOGLE_VERTEX_API_KEY: process.env.GOOGLE_VERTEX_API_KEY!,

    // Optional with defaults
    PORT: parseInt(process.env.PORT || "8080", 10),
    NODE_ENV: process.env.NODE_ENV || "development",
    REDIS_URL: process.env.REDIS_URL || "redis://localhost:6379",
    REDIS_KEY_PREFIX: process.env.REDIS_KEY_PREFIX || "llm_gateway",
    LOG_LEVEL: process.env.LOG_LEVEL || "info",
    RATE_LIMIT_PER_IP: parseInt(process.env.RATE_LIMIT_PER_IP || "100", 10),
    RATE_LIMIT_WINDOW_MS: parseInt(
      process.env.RATE_LIMIT_WINDOW_MS || "60000",
      10,
    ),
    CORS_ORIGINS: process.env.CORS_ORIGINS || "",
  };
}

// Singleton instance - call validateAndLoadEnv() once at startup
let envInstance: EnvConfig | null = null;

export function getEnv(): EnvConfig {
  if (!envInstance) {
    envInstance = validateAndLoadEnv();
  }
  return envInstance;
}
