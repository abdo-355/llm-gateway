import { validateAndLoadEnv, getEnv } from "./env";

describe("Env Config", () => {
  const originalEnv = process.env;

  beforeEach(() => {
    jest.resetModules();
    process.env = { ...originalEnv };
  });

  afterAll(() => {
    process.env = originalEnv;
  });

  describe("validateAndLoadEnv", () => {
    it("should return config when all required env vars are present", () => {
      process.env.GATEWAY_API_KEY = "test-gateway-key";
      process.env.GROQ_API_KEY = "test-groq-key";
      process.env.CEREBRAS_API_KEY = "test-cerebras-key";
      process.env.MISTRAL_API_KEY = "test-mistral-key";
      process.env.GOOGLE_VERTEX_API_KEY = "test-vertex-key";

      const result = validateAndLoadEnv();

      expect(result.GATEWAY_API_KEY).toBe("test-gateway-key");
      expect(result.GROQ_API_KEY).toBe("test-groq-key");
      expect(result.CEREBRAS_API_KEY).toBe("test-cerebras-key");
      expect(result.MISTRAL_API_KEY).toBe("test-mistral-key");
      expect(result.GOOGLE_VERTEX_API_KEY).toBe("test-vertex-key");
    });

    it("should use default values for optional env vars", () => {
      process.env.GATEWAY_API_KEY = "test-key";
      process.env.GROQ_API_KEY = "test-groq-key";
      process.env.CEREBRAS_API_KEY = "test-cerebras-key";
      process.env.MISTRAL_API_KEY = "test-mistral-key";
      process.env.GOOGLE_VERTEX_API_KEY = "test-vertex-key";
      delete process.env.PORT;
      delete process.env.NODE_ENV;
      delete process.env.REDIS_URL;
      delete process.env.REDIS_KEY_PREFIX;
      delete process.env.LOG_LEVEL;
      delete process.env.RATE_LIMIT_PER_IP;
      delete process.env.RATE_LIMIT_WINDOW_MS;
      delete process.env.CORS_ORIGINS;

      const result = validateAndLoadEnv();

      expect(result.PORT).toBe(8080);
      expect(result.NODE_ENV).toBe("development");
      expect(result.REDIS_URL).toBe("redis://localhost:6379");
      expect(result.REDIS_KEY_PREFIX).toBe("llm_gateway");
      expect(result.LOG_LEVEL).toBe("info");
      expect(result.RATE_LIMIT_PER_IP).toBe(100);
      expect(result.RATE_LIMIT_WINDOW_MS).toBe(60000);
      expect(result.CORS_ORIGINS).toBe("");
    });

    it("should use custom values from env vars", () => {
      process.env.GATEWAY_API_KEY = "test-key";
      process.env.GROQ_API_KEY = "test-groq-key";
      process.env.CEREBRAS_API_KEY = "test-cerebras-key";
      process.env.MISTRAL_API_KEY = "test-mistral-key";
      process.env.GOOGLE_VERTEX_API_KEY = "test-vertex-key";
      process.env.PORT = "3000";
      process.env.NODE_ENV = "production";
      process.env.REDIS_URL = "redis://redis.example.com:6379";
      process.env.REDIS_KEY_PREFIX = "my_prefix";
      process.env.LOG_LEVEL = "debug";
      process.env.RATE_LIMIT_PER_IP = "200";
      process.env.RATE_LIMIT_WINDOW_MS = "120000";
      process.env.CORS_ORIGINS = "https://example.com";

      const result = validateAndLoadEnv();

      expect(result.PORT).toBe(3000);
      expect(result.NODE_ENV).toBe("production");
      expect(result.REDIS_URL).toBe("redis://redis.example.com:6379");
      expect(result.REDIS_KEY_PREFIX).toBe("my_prefix");
      expect(result.LOG_LEVEL).toBe("debug");
      expect(result.RATE_LIMIT_PER_IP).toBe(200);
      expect(result.RATE_LIMIT_WINDOW_MS).toBe(120000);
      expect(result.CORS_ORIGINS).toBe("https://example.com");
    });

    it("should throw error when GATEWAY_API_KEY is missing", () => {
      delete process.env.GATEWAY_API_KEY;
      process.env.GROQ_API_KEY = "test-key";
      process.env.CEREBRAS_API_KEY = "test-key";
      process.env.MISTRAL_API_KEY = "test-key";
      process.env.GOOGLE_VERTEX_API_KEY = "test-key";

      expect(() => validateAndLoadEnv()).toThrow(
        "Missing required environment variables",
      );
      expect(() => validateAndLoadEnv()).toThrow("GATEWAY_API_KEY");
    });

    it("should throw error when GROQ_API_KEY is missing", () => {
      process.env.GATEWAY_API_KEY = "test-key";
      delete process.env.GROQ_API_KEY;
      process.env.CEREBRAS_API_KEY = "test-key";
      process.env.MISTRAL_API_KEY = "test-key";
      process.env.GOOGLE_VERTEX_API_KEY = "test-key";

      expect(() => validateAndLoadEnv()).toThrow(
        "Missing required environment variables",
      );
      expect(() => validateAndLoadEnv()).toThrow("GROQ_API_KEY");
    });

    it("should throw error when multiple required vars are missing", () => {
      process.env.GATEWAY_API_KEY = "test-key";
      delete process.env.GROQ_API_KEY;
      delete process.env.CEREBRAS_API_KEY;
      process.env.MISTRAL_API_KEY = "test-key";
      process.env.GOOGLE_VERTEX_API_KEY = "test-key";

      expect(() => validateAndLoadEnv()).toThrow(
        "Missing required environment variables",
      );
      expect(() => validateAndLoadEnv()).toThrow("GROQ_API_KEY");
      expect(() => validateAndLoadEnv()).toThrow("CEREBRAS_API_KEY");
    });

    it("should return correct type for numeric values", () => {
      process.env.GATEWAY_API_KEY = "test-key";
      process.env.GROQ_API_KEY = "test-groq-key";
      process.env.CEREBRAS_API_KEY = "test-cerebras-key";
      process.env.MISTRAL_API_KEY = "test-mistral-key";
      process.env.GOOGLE_VERTEX_API_KEY = "test-vertex-key";
      process.env.PORT = "8080";
      process.env.RATE_LIMIT_PER_IP = "100";
      process.env.RATE_LIMIT_WINDOW_MS = "60000";

      const result = validateAndLoadEnv();

      expect(typeof result.PORT).toBe("number");
      expect(typeof result.RATE_LIMIT_PER_IP).toBe("number");
      expect(typeof result.RATE_LIMIT_WINDOW_MS).toBe("number");
    });
  });

  describe("getEnv", () => {
    it("should return singleton instance", () => {
      process.env.GATEWAY_API_KEY = "test-key";
      process.env.GROQ_API_KEY = "test-groq-key";
      process.env.CEREBRAS_API_KEY = "test-cerebras-key";
      process.env.MISTRAL_API_KEY = "test-mistral-key";
      process.env.GOOGLE_VERTEX_API_KEY = "test-vertex-key";

      const result1 = getEnv();
      const result2 = getEnv();

      expect(result1).toBe(result2);
    });

    it("should cache the result", () => {
      process.env.GATEWAY_API_KEY = "test-key";
      process.env.GROQ_API_KEY = "test-groq-key";
      process.env.CEREBRAS_API_KEY = "test-cerebras-key";
      process.env.MISTRAL_API_KEY = "test-mistral-key";
      process.env.GOOGLE_VERTEX_API_KEY = "test-vertex-key";

      const result1 = getEnv();
      process.env.GATEWAY_API_KEY = "changed-key";
      const result2 = getEnv();

      expect(result1.GATEWAY_API_KEY).toBe("test-key");
      expect(result2.GATEWAY_API_KEY).toBe("test-key");
    });
  });
});
