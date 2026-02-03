import { QuotaService, estimateTokens } from "./quota";
import { getRedisClient } from "../lib/redis";
import { ChatCompletionRequest } from "../types";

// Mock Redis
jest.mock("../lib/redis");

describe("QuotaService", () => {
  let quotaService: QuotaService;
  let mockRedis: jest.Mocked<any>;

  beforeEach(() => {
    mockRedis = {
      get: jest.fn(),
      set: jest.fn(),
      setex: jest.fn(),
      incr: jest.fn(),
      expire: jest.fn(),
      zremrangebyscore: jest.fn().mockResolvedValue(0),
      zcard: jest.fn().mockResolvedValue(0),
      zadd: jest.fn().mockResolvedValue(1),
      zrangebyscore: jest.fn().mockResolvedValue([]),
      pipeline: jest.fn().mockReturnValue({
        incr: jest.fn().mockReturnThis(),
        incrby: jest.fn().mockReturnThis(),
        expire: jest.fn().mockReturnThis(),
        zadd: jest.fn().mockReturnThis(),
        set: jest.fn().mockReturnThis(),
        exec: jest.fn().mockResolvedValue([]),
      }),
    };
    (getRedisClient as jest.Mock).mockReturnValue(mockRedis);
    quotaService = new QuotaService();
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  describe("estimateTokens", () => {
    it("should estimate tokens for simple string content", () => {
      const request: ChatCompletionRequest = {
        model: "test-model",
        messages: [{ role: "user", content: "Hello world" }],
        max_tokens: 100,
      };

      const tokens = estimateTokens(request);

      // "Hello world" = 11 chars, max_tokens = 100, estimated output = 100 * 4 = 400
      // Total chars = 11 + 400 = 411, tokens = ceil(411/4) = 103
      expect(tokens).toBeGreaterThan(0);
      expect(tokens).toBe(103);
    });

    it("should estimate tokens for array content", () => {
      const request: ChatCompletionRequest = {
        model: "test-model",
        messages: [
          {
            role: "user",
            content: [{ type: "text", text: "Hello" }],
          },
        ],
        max_tokens: 50,
      };

      const tokens = estimateTokens(request);

      // "Hello" = 5 chars, max_tokens = 50, estimated output = 50 * 4 = 200
      // Total chars = 5 + 200 = 205, tokens = ceil(205/4) = 52
      expect(tokens).toBe(52);
    });

    it("should use default max_tokens when not specified", () => {
      const request: ChatCompletionRequest = {
        model: "test-model",
        messages: [{ role: "user", content: "Hi" }],
      };

      const tokens = estimateTokens(request);

      // Default max_tokens = 1000
      expect(tokens).toBeGreaterThan(250); // At least 1000/4
    });

    it("should handle empty messages", () => {
      const request: ChatCompletionRequest = {
        model: "test-model",
        messages: [],
        max_tokens: 100,
      };

      const tokens = estimateTokens(request);

      expect(tokens).toBe(100); // Just the output tokens
    });
  });

  describe("checkModelQuota", () => {
    it("should allow request when no limits are set", async () => {
      mockRedis.get.mockResolvedValue(null);

      await expect(
        quotaService.checkModelQuota("groq", "llama-3.1-8b", {}, 100),
      ).resolves.not.toThrow();
    });

    it("should handle provider rate limit headers gracefully", async () => {
      // The service should not throw when processing rate limit headers
      mockRedis.set.mockResolvedValue("OK");

      await expect(
        quotaService.handleProviderRateLimit("groq", "llama-3.1-8b", {
          headers: {},
          statusCode: 429,
        }),
      ).resolves.not.toThrow();
    });
  });

  describe("getModelQuotaStatus", () => {
    it("should return zeros when no usage", async () => {
      mockRedis.get.mockResolvedValue(null);

      const status = await quotaService.getModelQuotaStatus(
        "groq",
        "llama-3.1-8b",
        { rpm: 30, tpm: 2000 },
      );

      expect(status.rpm).toBe(0);
      expect(status.tpm).toBe(0);
      expect(status.rph).toBe(0);
      expect(status.rpd).toBe(0);
    });
  });
});
