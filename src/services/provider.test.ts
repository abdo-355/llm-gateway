import { ProviderService, providerService } from "./provider";
import {
  ProviderError,
  PaymentRequiredError,
  RateLimitHeaders,
} from "../errors";
import { ChatCompletionRequest } from "../types";
import { logger } from "../utils/logger";

// Mock logger
jest.mock("../utils/logger", () => ({
  logger: {
    info: jest.fn(),
    error: jest.fn(),
    warn: jest.fn(),
  },
}));

// Helper to create mock Headers
const createMockHeaders = (headerMap: Record<string, string> = {}): Headers => {
  return {
    get: (name: string): string | null => {
      const key = Object.keys(headerMap).find(
        (k) => k.toLowerCase() === name.toLowerCase(),
      );
      return key ? headerMap[key] : null;
    },
  } as Headers;
};

describe("ProviderError", () => {
  it("should create ProviderError with message, statusCode, and isRetryable", () => {
    const error = new ProviderError("Test error", 500, true);

    expect(error.message).toBe("Test error");
    expect(error.statusCode).toBe(500);
    expect(error.isRetryable).toBe(true);
    expect(error.name).toBe("ProviderError");
  });

  it("should default isRetryable to false", () => {
    const error = new ProviderError("Test error", 400);

    expect(error.isRetryable).toBe(false);
  });

  it("should be an instance of Error", () => {
    const error = new ProviderError("Test", 500);

    expect(error).toBeInstanceOf(Error);
    expect(error).toBeInstanceOf(ProviderError);
  });

  it("should include headers when provided", () => {
    const headers: RateLimitHeaders = {
      retryAfter: 60,
      remainingRequests: 100,
    };
    const error = new ProviderError("Rate limited", 429, true, headers);

    expect(error.headers).toEqual(headers);
    expect(error.headers?.retryAfter).toBe(60);
    expect(error.headers?.remainingRequests).toBe(100);
  });
});

describe("PaymentRequiredError", () => {
  it("should be a non-retryable 402 error", () => {
    const error = new PaymentRequiredError("Payment required");

    expect(error.statusCode).toBe(402);
    expect(error.isRetryable).toBe(false);
    expect(error.name).toBe("PaymentRequiredError");
    expect(error).toBeInstanceOf(ProviderError);
  });

  it("should include rate limit headers", () => {
    const headers: RateLimitHeaders = {
      retryAfter: 0,
      remainingRequests: 0,
    };
    const error = new PaymentRequiredError("Payment required", headers);

    expect(error.headers).toEqual(headers);
  });
});

describe("ProviderService.callProvider", () => {
  let service: ProviderService;

  beforeEach(() => {
    service = new ProviderService();
    jest.clearAllMocks();
    global.fetch = jest.fn();
  });

  afterEach(() => {
    jest.resetAllMocks();
  });

  const mockRequest: ChatCompletionRequest = {
    messages: [{ role: "user", content: "Hello" }],
    model: "gpt-4",
  };

  const mockResponse = {
    id: "resp-123",
    object: "chat.completion",
    created: 1234567890,
    model: "gpt-4",
    choices: [
      {
        index: 0,
        message: { role: "assistant", content: "Hi!" },
        finish_reason: "stop",
      },
    ],
    usage: {
      prompt_tokens: 10,
      completion_tokens: 5,
      total_tokens: 15,
    },
  };

  describe("success cases", () => {
    it("should return parsed JSON response on HTTP 200", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        json: jest.fn().mockResolvedValueOnce(mockResponse),
      });

      const result = await service.callProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
      );

      expect(result).toEqual(mockResponse);
    });

    it("should include Authorization header when API key provided", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        json: jest.fn().mockResolvedValueOnce(mockResponse),
      });

      await service.callProvider(
        "https://api.openai.com/v1",
        "sk-test-key",
        "gpt-4",
        mockRequest,
        30000,
      );

      const callArgs = (global.fetch as jest.Mock).mock.calls[0];
      expect(callArgs[1].headers.Authorization).toBe("Bearer sk-test-key");
    });

    it("should not include Authorization header when no API key", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        json: jest.fn().mockResolvedValueOnce(mockResponse),
      });

      await service.callProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
      );

      const callArgs = (global.fetch as jest.Mock).mock.calls[0];
      expect(callArgs[1].headers.Authorization).toBeUndefined();
    });

    it("should log success with latency and tokens", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        json: jest.fn().mockResolvedValueOnce(mockResponse),
      });

      await service.callProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
      );

      expect(logger.info).toHaveBeenCalledWith(
        expect.objectContaining({
          event: "provider_success",
          baseUrl: "https://api.openai.com/v1",
          model: "gpt-4",
          latency_ms: expect.any(Number),
          tokens_used: 15,
        }),
      );
    });

    it("should use provided abort signal", async () => {
      const abortSignal = new AbortController().signal;
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        json: jest.fn().mockResolvedValueOnce(mockResponse),
      });

      await service.callProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
        abortSignal,
      );

      const callArgs = (global.fetch as jest.Mock).mock.calls[0];
      expect(callArgs[1].signal).toBe(abortSignal);
    });
  });

  describe("error cases", () => {
    it("should throw ProviderError on HTTP 4xx (non-retryable)", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 400,
        headers: createMockHeaders(),
        text: jest.fn().mockResolvedValueOnce("Bad Request"),
      });

      try {
        await service.callProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
        );
      } catch (error) {
        expect(error).toBeInstanceOf(ProviderError);
        expect((error as ProviderError).statusCode).toBe(400);
        expect((error as ProviderError).isRetryable).toBe(false);
      }
    });

    it("should throw retryable ProviderError on HTTP 429", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 429,
        headers: createMockHeaders({
          "retry-after": "60",
          "x-ratelimit-remaining-requests": "0",
        }),
        text: jest.fn().mockResolvedValueOnce("Rate Limited"),
      });

      try {
        await service.callProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
        );
      } catch (error) {
        expect(error).toBeInstanceOf(ProviderError);
        expect((error as ProviderError).statusCode).toBe(429);
        expect((error as ProviderError).isRetryable).toBe(true);
        expect((error as ProviderError).headers?.retryAfter).toBe(60);
        expect((error as ProviderError).headers?.remainingRequests).toBe(0);
      }
    });

    it("should throw PaymentRequiredError on HTTP 402", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 402,
        headers: createMockHeaders(),
        text: jest.fn().mockResolvedValueOnce("Payment Required"),
      });

      try {
        await service.callProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
        );
      } catch (error) {
        expect(error).toBeInstanceOf(PaymentRequiredError);
        expect(error).toBeInstanceOf(ProviderError);
        expect((error as ProviderError).statusCode).toBe(402);
        expect((error as ProviderError).isRetryable).toBe(false);
      }
    });

    it("should throw retryable ProviderError on HTTP 5xx", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 503,
        headers: createMockHeaders(),
        text: jest.fn().mockResolvedValueOnce("Service Unavailable"),
      });

      try {
        await service.callProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
        );
      } catch (error) {
        expect(error).toBeInstanceOf(ProviderError);
        expect((error as ProviderError).statusCode).toBe(503);
        expect((error as ProviderError).isRetryable).toBe(true);
      }
    });

    it("should log provider_error on HTTP error", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 500,
        headers: createMockHeaders(),
        text: jest.fn().mockResolvedValueOnce("Server Error"),
      });

      try {
        await service.callProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
        );
      } catch (error) {
        // expected
      }

      expect(logger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          event: "provider_error",
          status: 500,
          error: "Server Error",
        }),
      );
    });

    it("should throw ProviderError with status 504 on AbortError", async () => {
      const abortError = new Error("The operation was aborted");
      abortError.name = "AbortError";
      (global.fetch as jest.Mock).mockRejectedValueOnce(abortError);

      try {
        await service.callProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
        );
      } catch (error) {
        expect(error).toBeInstanceOf(ProviderError);
        expect((error as ProviderError).statusCode).toBe(504);
        expect((error as ProviderError).isRetryable).toBe(true);
        expect((error as ProviderError).message).toBe("Request timed out");
      }
    });

    it("should rethrow existing ProviderError", async () => {
      const existingError = new ProviderError(
        "Already a ProviderError",
        500,
        true,
      );
      (global.fetch as jest.Mock).mockRejectedValueOnce(existingError);

      await expect(
        service.callProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
        ),
      ).rejects.toBe(existingError);
    });

    it("should throw ProviderError on network errors", async () => {
      (global.fetch as jest.Mock).mockRejectedValueOnce(
        new Error("Network error"),
      );

      try {
        await service.callProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
        );
      } catch (error) {
        expect(error).toBeInstanceOf(ProviderError);
        expect((error as ProviderError).statusCode).toBe(500);
        expect((error as ProviderError).isRetryable).toBe(true);
      }

      expect(logger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          event: "provider_exception",
          error: "Network error",
        }),
      );
    });
  });
});

describe("ProviderService.streamProvider", () => {
  let service: ProviderService;

  beforeEach(() => {
    service = new ProviderService();
    jest.clearAllMocks();
    global.fetch = jest.fn();
  });

  const mockRequest: ChatCompletionRequest = {
    messages: [{ role: "user", content: "Hello" }],
    model: "gpt-4",
  };

  const createMockReader = (chunks: string[]) => {
    let index = 0;
    return {
      read: jest.fn().mockImplementation(() => {
        if (index >= chunks.length) {
          return Promise.resolve({ done: true, value: undefined });
        }
        const value = new TextEncoder().encode(chunks[index]);
        index++;
        return Promise.resolve({ done: false, value });
      }),
      releaseLock: jest.fn(),
    };
  };

  describe("success cases", () => {
    it("should call onChunk for each SSE data line", async () => {
      const onChunk = jest.fn();
      const chunks = [
        'data: {"id":"1"}\n\n',
        'data: {"id":"2"}\n\n',
        "data: [DONE]\n\n",
      ];

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        body: {
          getReader: () => createMockReader(chunks),
        },
      });

      await service.streamProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
        onChunk,
      );

      expect(onChunk).toHaveBeenCalledTimes(2);
      expect(onChunk).toHaveBeenNthCalledWith(1, { id: "1" });
      expect(onChunk).toHaveBeenNthCalledWith(2, { id: "2" });
    });

    it("should set stream: true in request body", async () => {
      const onChunk = jest.fn();
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        body: {
          getReader: () => createMockReader([]),
        },
      });

      await service.streamProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
        onChunk,
      );

      const callArgs = (global.fetch as jest.Mock).mock.calls[0];
      const body = JSON.parse(callArgs[1].body);
      expect(body.stream).toBe(true);
    });

    it("should set Accept header to text/event-stream", async () => {
      const onChunk = jest.fn();
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        body: {
          getReader: () => createMockReader([]),
        },
      });

      await service.streamProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
        onChunk,
      );

      const callArgs = (global.fetch as jest.Mock).mock.calls[0];
      expect(callArgs[1].headers.Accept).toBe("text/event-stream");
    });

    it("should include Authorization header when API key provided", async () => {
      const onChunk = jest.fn();
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        body: {
          getReader: () => createMockReader([]),
        },
      });

      await service.streamProvider(
        "https://api.openai.com/v1",
        "sk-test",
        "gpt-4",
        mockRequest,
        30000,
        onChunk,
      );

      const callArgs = (global.fetch as jest.Mock).mock.calls[0];
      expect(callArgs[1].headers.Authorization).toBe("Bearer sk-test");
    });

    it("should handle SSE across chunk boundaries", async () => {
      const onChunk = jest.fn();
      // First chunk has partial line, second completes it
      const chunks = ['data: {"id":"1"}\ndata: {"i', 'd":"2"}\n\n'];

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        body: {
          getReader: () => createMockReader(chunks),
        },
      });

      await service.streamProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
        onChunk,
      );

      expect(onChunk).toHaveBeenCalledTimes(2);
    });

    it("should skip non-data lines", async () => {
      const onChunk = jest.fn();
      const chunks = [
        ": comment\n\n",
        "event: message\n\n",
        'data: {"id":"1"}\n\n',
      ];

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        body: {
          getReader: () => createMockReader(chunks),
        },
      });

      await service.streamProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
        onChunk,
      );

      expect(onChunk).toHaveBeenCalledTimes(1);
      expect(onChunk).toHaveBeenCalledWith({ id: "1" });
    });
  });

  describe("error cases", () => {
    it("should throw ProviderError on HTTP error", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 500,
        headers: createMockHeaders(),
        text: jest.fn().mockResolvedValueOnce("Error"),
      });

      await expect(
        service.streamProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
          jest.fn(),
        ),
      ).rejects.toThrow(ProviderError);
    });

    it("should throw PaymentRequiredError on HTTP 402", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 402,
        headers: createMockHeaders(),
        text: jest.fn().mockResolvedValueOnce("Payment Required"),
      });

      await expect(
        service.streamProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
          jest.fn(),
        ),
      ).rejects.toThrow(PaymentRequiredError);
    });

    it("should throw ProviderError when response.body is null", async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        body: null,
      });

      try {
        await service.streamProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
          jest.fn(),
        );
      } catch (error) {
        expect(error).toBeInstanceOf(ProviderError);
        expect((error as ProviderError).statusCode).toBe(500);
        expect((error as ProviderError).message).toBe(
          "No response body for streaming",
        );
      }
    });

    it("should throw ProviderError with status 504 on AbortError", async () => {
      const abortError = new Error("Aborted");
      abortError.name = "AbortError";
      (global.fetch as jest.Mock).mockRejectedValueOnce(abortError);

      try {
        await service.streamProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
          jest.fn(),
        );
      } catch (error) {
        expect(error).toBeInstanceOf(ProviderError);
        expect((error as ProviderError).statusCode).toBe(504);
        expect((error as ProviderError).isRetryable).toBe(true);
      }
    });

    it("should throw retryable ProviderError on network errors", async () => {
      (global.fetch as jest.Mock).mockRejectedValueOnce(
        new Error("Network failed"),
      );

      try {
        await service.streamProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
          jest.fn(),
        );
      } catch (error) {
        expect(error).toBeInstanceOf(ProviderError);
        expect((error as ProviderError).statusCode).toBe(500);
        expect((error as ProviderError).isRetryable).toBe(true);
      }
    });

    it("should log warning on SSE parse errors", async () => {
      const onChunk = jest.fn();
      const chunks = ["data: invalid json\n\n", 'data: {"id":"1"}\n\n'];

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        body: {
          getReader: () => createMockReader(chunks),
        },
      });

      await service.streamProvider(
        "https://api.openai.com/v1",
        undefined,
        "gpt-4",
        mockRequest,
        30000,
        onChunk,
      );

      expect(logger.warn).toHaveBeenCalledWith(
        expect.objectContaining({
          event: "sse_parse_error",
          line: "data: invalid json",
        }),
      );
      expect(onChunk).toHaveBeenCalledTimes(1);
    });

    it("should release reader lock on error", async () => {
      const onChunk = jest.fn();
      const reader = {
        read: jest.fn().mockRejectedValueOnce(new Error("Read failed")),
        releaseLock: jest.fn(),
      };

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        headers: createMockHeaders(),
        body: {
          getReader: () => reader,
        },
      });

      try {
        await service.streamProvider(
          "https://api.openai.com/v1",
          undefined,
          "gpt-4",
          mockRequest,
          30000,
          onChunk,
        );
      } catch (error) {
        // expected
      }

      expect(reader.releaseLock).toHaveBeenCalled();
    });
  });
});

describe("providerService singleton", () => {
  it("should be an instance of ProviderService", () => {
    expect(providerService).toBeInstanceOf(ProviderService);
  });
});
