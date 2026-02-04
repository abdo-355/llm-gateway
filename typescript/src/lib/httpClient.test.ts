import { createHttpClient } from "./httpClient";

describe("HttpClient", () => {
  let httpClient: ReturnType<typeof createHttpClient>;

  beforeEach(() => {
    httpClient = createHttpClient();
  });

  describe("createHttpClient", () => {
    it("should create an httpClient with get and post methods", () => {
      expect(httpClient).toHaveProperty("get");
      expect(httpClient).toHaveProperty("post");
      expect(typeof httpClient.get).toBe("function");
      expect(typeof httpClient.post).toBe("function");
    });
  });

  describe("sanitizeUrlForLogging", () => {
    it("should remove query parameters and auth info from URL", () => {
      const { sanitizeUrlForLogging } = require("./httpClient");
      expect(
        sanitizeUrlForLogging(
          "https://api.example.com/chat/completions?api_key=secret",
        ),
      ).toBe("https://api.example.com/chat/completions");
      expect(
        sanitizeUrlForLogging("https://user:pass@api.example.com/path"),
      ).toBe("https://api.example.com/path");
    });

    it("should return safe placeholder for invalid URLs", () => {
      const { sanitizeUrlForLogging } = require("./httpClient");
      expect(sanitizeUrlForLogging("not-a-url")).toBe("[invalid-url]");
    });
  });

  describe("extractRateLimitHeaders", () => {
    it("should return empty object for missing headers", () => {
      const { extractRateLimitHeaders } = require("./httpClient");
      const mockResponse = {
        headers: {
          get: jest.fn().mockReturnValue(null),
        },
      } as unknown as Response;

      const result = extractRateLimitHeaders(mockResponse);
      expect(result).toEqual({});
    });

    it("should extract retry-after header", () => {
      const { extractRateLimitHeaders } = require("./httpClient");
      const mockResponse = {
        headers: {
          get: jest.fn((name: string) => {
            if (name === "retry-after") return "30";
            return null;
          }),
        },
      } as unknown as Response;

      const result = extractRateLimitHeaders(mockResponse);
      expect(result.retryAfter).toBe(30);
    });
  });

  describe("isRetryableStatus", () => {
    it("should return false for 402 (payment required)", () => {
      const { isRetryableStatus } = require("./httpClient");
      expect(isRetryableStatus(402)).toBe(false);
    });

    it("should return true for 429 (rate limited)", () => {
      const { isRetryableStatus } = require("./httpClient");
      expect(isRetryableStatus(429)).toBe(true);
    });

    it("should return true for 5xx errors", () => {
      const { isRetryableStatus } = require("./httpClient");
      expect(isRetryableStatus(500)).toBe(true);
      expect(isRetryableStatus(503)).toBe(true);
    });

    it("should return false for 4xx errors except 429", () => {
      const { isRetryableStatus } = require("./httpClient");
      expect(isRetryableStatus(400)).toBe(false);
      expect(isRetryableStatus(401)).toBe(false);
      expect(isRetryableStatus(404)).toBe(false);
    });
  });
});
