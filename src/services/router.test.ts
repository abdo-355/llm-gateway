// Mock Redis before importing anything that uses it
jest.mock("../lib/redis", () => ({
  getRedisClient: jest.fn(() => ({
    on: jest.fn(),
    get: jest.fn(),
    set: jest.fn(),
    setex: jest.fn(),
    incr: jest.fn(),
    zadd: jest.fn(),
    zcard: jest.fn(),
    zremrangebyscore: jest.fn(),
    expire: jest.fn(),
    keys: jest.fn(),
    quit: jest.fn(),
  })),
  closeRedis: jest.fn(),
}));

// Mock the config module
jest.mock("../config", () => ({
  getProviderApiKey: jest.fn((providerId: string) => {
    if (providerId === "groq") return "mock-groq-key";
    if (providerId === "openai") return "mock-openai-key";
    return undefined;
  }),
}));

import {
  deriveRequirements,
  scoreCandidates,
  compilePlan,
  shouldRetry,
  createGatewayError,
  RoutingCandidate,
  RoutingPlan,
} from "./router";
import {
  ChatCompletionRequest,
  RouterHints,
  ProviderConfig,
  AppConfig,
} from "../types";
import { ProviderError } from "../errors";

// Test data helpers
const createMockRequest = (
  overrides: Partial<ChatCompletionRequest> = {},
): ChatCompletionRequest => ({
  messages: [{ role: "user", content: "Hello" }],
  model: "test-model",
  ...overrides,
});

const createMockProvider = (id: string): ProviderConfig => ({
  id,
  baseUrl: `https://${id}.com`,
  auth: { type: "none", env: "" },
  models: { mode: "allowlist", list: ["model-1"] },
  capabilities: {
    streaming: true,
    tools: true,
    structuredOutputs: "model_dependent",
  },
  limits: {},
});

const createMockCandidate = (
  id: string,
  model: string,
  score: number = 0,
): RoutingCandidate => ({
  provider: createMockProvider(id),
  model,
  isCertifiedForStrictSchema: false,
  score,
  scoreBreakdown: {},
});

describe("deriveRequirements", () => {
  describe("default behavior", () => {
    it("should default to text output when no response_format specified", () => {
      const request = createMockRequest();
      const result = deriveRequirements(request);

      expect(result.output).toBe("text");
      expect(result.streaming).toBe("preferred");
      expect(result.tools).toBe("forbidden");
    });
  });

  describe("output requirement detection", () => {
    it("should detect strict json_schema when strict is true", () => {
      const request = createMockRequest({
        response_format: {
          type: "json_schema",
          json_schema: {
            name: "test",
            strict: true,
            schema: { type: "object" },
          },
        },
      });

      const result = deriveRequirements(request);
      expect(result.output).toBe("json_schema_strict");
    });

    it("should default to text when json_schema strict is false", () => {
      const request = createMockRequest({
        response_format: {
          type: "json_schema",
          json_schema: {
            name: "test",
            strict: false,
            schema: { type: "object" },
          },
        },
      });

      const result = deriveRequirements(request);
      expect(result.output).toBe("text");
    });

    it("should default to text for json_object type", () => {
      const request = createMockRequest({
        response_format: { type: "json_object" },
      });

      const result = deriveRequirements(request);
      expect(result.output).toBe("text");
    });

    it("should default to text for text type", () => {
      const request = createMockRequest({
        response_format: { type: "text" },
      });

      const result = deriveRequirements(request);
      expect(result.output).toBe("text");
    });
  });

  describe("streaming requirement detection", () => {
    it("should detect streaming required when stream is true", () => {
      const request = createMockRequest({ stream: true });
      const result = deriveRequirements(request);
      expect(result.streaming).toBe("required");
    });

    it("should detect streaming forbidden when stream is false", () => {
      const request = createMockRequest({ stream: false });
      const result = deriveRequirements(request);
      expect(result.streaming).toBe("forbidden");
    });

    it("should default to streaming preferred when stream not specified", () => {
      const request = createMockRequest();
      const result = deriveRequirements(request);
      expect(result.streaming).toBe("preferred");
    });
  });

  describe("tools requirement detection", () => {
    it("should detect tools required when tools present and tool_choice is required", () => {
      const request = createMockRequest({
        tools: [
          { type: "function", function: { name: "test", parameters: {} } },
        ],
        tool_choice: "required",
      });

      const result = deriveRequirements(request);
      expect(result.tools).toBe("required");
    });

    it("should detect tools forbidden when tool_choice is none", () => {
      const request = createMockRequest({
        tools: [
          { type: "function", function: { name: "test", parameters: {} } },
        ],
        tool_choice: "none",
      });

      const result = deriveRequirements(request);
      expect(result.tools).toBe("forbidden");
    });

    it("should detect tools allowed when tools present without specific tool_choice", () => {
      const request = createMockRequest({
        tools: [
          { type: "function", function: { name: "test", parameters: {} } },
        ],
      });

      const result = deriveRequirements(request);
      expect(result.tools).toBe("allowed");
    });

    it("should default to tools forbidden when no tools specified", () => {
      const request = createMockRequest();
      const result = deriveRequirements(request);
      expect(result.tools).toBe("forbidden");
    });

    it("should handle empty tools array as forbidden", () => {
      const request = createMockRequest({ tools: [] });
      const result = deriveRequirements(request);
      expect(result.tools).toBe("forbidden");
    });
  });

  describe("router hints override", () => {
    it("should allow hints to override output requirement", () => {
      const request = createMockRequest({
        response_format: {
          type: "json_schema",
          json_schema: {
            name: "test",
            strict: false,
            schema: {},
          },
        },
      });
      const hints: RouterHints = {
        requirements: { output: "json_schema_strict" },
      };

      const result = deriveRequirements(request, hints);
      expect(result.output).toBe("json_schema_strict");
    });

    it("should allow hints to override streaming requirement", () => {
      const request = createMockRequest({ stream: false });
      const hints: RouterHints = {
        requirements: { streaming: "required" },
      };

      const result = deriveRequirements(request, hints);
      expect(result.streaming).toBe("required");
    });

    it("should allow hints to override tools requirement", () => {
      const request = createMockRequest();
      const hints: RouterHints = {
        requirements: { tools: "required" },
      };

      const result = deriveRequirements(request, hints);
      expect(result.tools).toBe("required");
    });

    it("should not override when hints requirements not specified", () => {
      const request = createMockRequest({ stream: true });
      const hints: RouterHints = {
        profile: "cheap_fast",
      };

      const result = deriveRequirements(request, hints);
      expect(result.streaming).toBe("required");
      expect(result.output).toBe("text");
      expect(result.tools).toBe("forbidden");
    });
  });

  describe("complex scenarios", () => {
    it("should handle combination of streaming and strict schema", () => {
      const request = createMockRequest({
        stream: true,
        response_format: {
          type: "json_schema",
          json_schema: {
            name: "test",
            strict: true,
            schema: {},
          },
        },
      });

      const result = deriveRequirements(request);
      expect(result.output).toBe("json_schema_strict");
      expect(result.streaming).toBe("required");
    });

    it("should handle combination of tools and streaming", () => {
      const request = createMockRequest({
        stream: true,
        tools: [
          { type: "function", function: { name: "test", parameters: {} } },
        ],
        tool_choice: "auto",
      });

      const result = deriveRequirements(request);
      expect(result.streaming).toBe("required");
      expect(result.tools).toBe("allowed");
    });
  });
});

describe("scoreCandidates", () => {
  describe("base weight", () => {
    it("should assign base weight to all candidates", () => {
      const candidates = [
        createMockCandidate("provider-a", "model-a"),
        createMockCandidate("provider-b", "model-b"),
      ];

      const scored = scoreCandidates(candidates, undefined);

      expect(scored[0].scoreBreakdown.base).toBe(1.0);
      expect(scored[1].scoreBreakdown.base).toBe(1.0);
    });
  });

  describe("preference bonus", () => {
    it("should give highest bonus to first preferred provider", () => {
      const candidates = [
        createMockCandidate("provider-a", "model-a"),
        createMockCandidate("provider-b", "model-b"),
      ];

      const hints: RouterHints = {
        providers: { prefer: ["provider-a", "provider-b"] },
      };

      const scored = scoreCandidates(candidates, hints);

      // First provider gets 0.5 bonus
      expect(scored[0].scoreBreakdown.prefer).toBe(0.5);
    });

    it("should decrease bonus for lower preference rank", () => {
      const candidates = [
        createMockCandidate("provider-a", "model-a"),
        createMockCandidate("provider-b", "model-b"),
        createMockCandidate("provider-c", "model-c"),
      ];

      const hints: RouterHints = {
        providers: { prefer: ["provider-a", "provider-b", "provider-c"] },
      };

      const scored = scoreCandidates(candidates, hints);

      // Second provider gets 0.5 * (1 - 1/3) = 0.333...
      const secondProvider = scored.find((c) => c.provider.id === "provider-b");
      expect(secondProvider?.scoreBreakdown.prefer).toBeCloseTo(0.333, 2);
    });

    it("should not give bonus if provider not in prefer list", () => {
      const candidates = [createMockCandidate("provider-a", "model-a")];

      const hints: RouterHints = {
        providers: { prefer: ["provider-b"] },
      };

      const scored = scoreCandidates(candidates, hints);

      expect(scored[0].scoreBreakdown.prefer).toBeUndefined();
    });

    it("should not give bonus if no prefer list", () => {
      const candidates = [createMockCandidate("provider-a", "model-a")];

      const scored = scoreCandidates(candidates, undefined);

      expect(scored[0].scoreBreakdown.prefer).toBeUndefined();
    });
  });

  describe("sorting", () => {
    it("should sort candidates by score descending", () => {
      const candidates = [
        { ...createMockCandidate("low", "model-a", 1.0) },
        { ...createMockCandidate("high", "model-b", 2.0) },
        { ...createMockCandidate("mid", "model-c", 1.5) },
      ];

      const scored = scoreCandidates(candidates, undefined);

      expect(scored[0].provider.id).toBe("high");
      expect(scored[1].provider.id).toBe("mid");
      expect(scored[2].provider.id).toBe("low");
    });

    it("should preserve existing scores in breakdown", () => {
      const candidates = [{ ...createMockCandidate("a", "model-a", 5.0) }];

      const scored = scoreCandidates(candidates, undefined);

      expect(scored[0].score).toBe(6.25); // 5.0 existing + 1.0 base + 0.25 health
    });
  });

  describe("health score", () => {
    it("should include health score in breakdown", () => {
      const candidates = [createMockCandidate("provider-a", "model-a")];

      const scored = scoreCandidates(candidates, undefined);

      expect(scored[0].scoreBreakdown.health).toBe(0.25); // 0.5 * 0.5
    });
  });

  describe("edge cases", () => {
    it("should handle empty candidates array", () => {
      const scored = scoreCandidates([], undefined);
      expect(scored).toEqual([]);
    });

    it("should handle single candidate", () => {
      const candidates = [createMockCandidate("solo", "model-a")];

      const scored = scoreCandidates(candidates, undefined);

      expect(scored).toHaveLength(1);
      expect(scored[0].provider.id).toBe("solo");
    });
  });
});

describe("compilePlan", () => {
  const createMockConfig = (): AppConfig => ({
    providers: [
      {
        id: "groq",
        baseUrl: "https://api.groq.com",
        auth: { type: "bearer", env: "GROQ_API_KEY" },
        models: { mode: "allowlist", list: ["model-1"] },
        capabilities: {
          streaming: true,
          tools: true,
          structuredOutputs: "model_dependent",
        },
        limits: {},
      },
    ],
    certifications: [],
  });

  describe("candidate selection", () => {
    it("should select top N candidates based on max_attempts", () => {
      const candidates = [
        { ...createMockCandidate("a", "model-a"), score: 3.0 },
        { ...createMockCandidate("b", "model-b"), score: 2.0 },
        { ...createMockCandidate("c", "model-c"), score: 1.0 },
        { ...createMockCandidate("d", "model-d"), score: 0.5 },
      ];

      const hints: RouterHints = {
        fallback: { max_attempts: 2 },
      };

      const plan = compilePlan(candidates, createMockConfig(), hints);

      expect(plan.attempts).toHaveLength(2);
      expect(plan.attempts[0].providerId).toBe("a");
      expect(plan.attempts[1].providerId).toBe("b");
    });

    it("should use default max_attempts of 3 when not specified", () => {
      const candidates = [
        { ...createMockCandidate("a", "model-a"), score: 3.0 },
        { ...createMockCandidate("b", "model-b"), score: 2.0 },
        { ...createMockCandidate("c", "model-c"), score: 1.0 },
        { ...createMockCandidate("d", "model-d"), score: 0.5 },
      ];

      const plan = compilePlan(candidates, createMockConfig(), undefined);

      expect(plan.attempts).toHaveLength(3);
      expect(plan.maxAttempts).toBe(3);
    });

    it("should handle fewer candidates than max_attempts", () => {
      const candidates = [
        { ...createMockCandidate("a", "model-a"), score: 1.0 },
      ];

      const hints: RouterHints = {
        fallback: { max_attempts: 5 },
      };

      const plan = compilePlan(candidates, createMockConfig(), hints);

      expect(plan.attempts).toHaveLength(1);
    });
  });

  describe("timeout configuration", () => {
    it("should use max_latency_ms from hints", () => {
      const candidates = [createMockCandidate("a", "model-a")];

      const hints: RouterHints = {
        slo: { max_latency_ms: 5000 },
      };

      const plan = compilePlan(candidates, createMockConfig(), hints);

      expect(plan.attempts[0].timeoutMs).toBe(5000);
    });

    it("should use default timeout of 30000ms when not specified", () => {
      const candidates = [createMockCandidate("a", "model-a")];

      const plan = compilePlan(candidates, createMockConfig(), undefined);

      expect(plan.attempts[0].timeoutMs).toBe(30000);
    });
  });

  describe("hard timeout", () => {
    it("should set hard_timeout_ms from hints", () => {
      const candidates = [createMockCandidate("a", "model-a")];

      const hints: RouterHints = {
        slo: { hard_timeout_ms: 10000 },
      };

      const plan = compilePlan(candidates, createMockConfig(), hints);

      expect(plan.hardTimeoutMs).toBe(10000);
    });

    it("should have undefined hardTimeoutMs when not specified", () => {
      const candidates = [createMockCandidate("a", "model-a")];

      const plan = compilePlan(candidates, createMockConfig(), undefined);

      expect(plan.hardTimeoutMs).toBeUndefined();
    });
  });

  describe("retry policy", () => {
    it("should set retry flags from hints when all disabled", () => {
      const candidates = [createMockCandidate("a", "model-a")];

      const hints: RouterHints = {
        fallback: {
          on_429: false,
          on_timeout: false,
          on_5xx: false,
        },
      };

      const plan = compilePlan(candidates, createMockConfig(), hints);

      expect(plan.retryOn429).toBe(false);
      expect(plan.retryOnTimeout).toBe(false);
      expect(plan.retryOn5xx).toBe(false);
    });

    it("should use default retry flags of true when not specified", () => {
      const candidates = [createMockCandidate("a", "model-a")];

      const plan = compilePlan(candidates, createMockConfig(), undefined);

      expect(plan.retryOn429).toBe(true);
      expect(plan.retryOnTimeout).toBe(true);
      expect(plan.retryOn5xx).toBe(true);
    });

    it("should allow partial retry flag override", () => {
      const candidates = [createMockCandidate("a", "model-a")];

      const hints: RouterHints = {
        fallback: {
          on_429: false,
        },
      };

      const plan = compilePlan(candidates, createMockConfig(), hints);

      expect(plan.retryOn429).toBe(false);
      expect(plan.retryOnTimeout).toBe(true);
      expect(plan.retryOn5xx).toBe(true);
    });
  });

  describe("attempt structure", () => {
    it("should include all required attempt fields", () => {
      const candidates = [
        {
          ...createMockCandidate("groq", "llama-3.3-70b"),
          score: 1.5,
        },
      ];
      const config = createMockConfig();

      const plan = compilePlan(candidates, config, undefined);

      expect(plan.attempts[0]).toMatchObject({
        providerId: "groq",
        model: "llama-3.3-70b",
        baseUrl: "https://groq.com",
        apiKey: "mock-groq-key",
        score: 1.5,
        timeoutMs: 30000,
        providerType: "openai",
        auth: { type: "none", env: "" },
      });
    });
  });

  describe("edge cases", () => {
    it("should handle empty candidates array", () => {
      const plan = compilePlan([], createMockConfig(), undefined);

      expect(plan.attempts).toHaveLength(0);
      expect(plan.maxAttempts).toBe(3);
    });
  });
});

describe("shouldRetry", () => {
  const createMockPlan = (
    overrides: Partial<RoutingPlan> = {},
  ): RoutingPlan => ({
    attempts: [
      {
        providerId: "a",
        model: "m1",
        baseUrl: "http://a.com",
        apiKey: "key1",
        score: 1.0,
        timeoutMs: 30000,
        providerType: "openai",
        auth: { type: "bearer", env: "TEST_KEY1" },
      },
      {
        providerId: "b",
        model: "m2",
        baseUrl: "http://b.com",
        apiKey: "key2",
        score: 0.8,
        timeoutMs: 30000,
        providerType: "openai",
        auth: { type: "bearer", env: "TEST_KEY2" },
      },
    ],
    maxAttempts: 2,
    hardTimeoutMs: undefined,
    retryOn429: true,
    retryOnTimeout: true,
    retryOn5xx: true,
    ...overrides,
  });

  const createMockError = (
    statusCode: number,
    isRetryable: boolean,
  ): ProviderError => {
    const error = new Error(`Error ${statusCode}`) as ProviderError;
    error.statusCode = statusCode;
    error.isRetryable = isRetryable;
    return error;
  };

  describe("retryable errors", () => {
    it("should retry on 429 when retryOn429 is true", () => {
      const error = createMockError(429, true);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(true);
    });

    it("should retry on 503 when retryOn5xx is true", () => {
      const error = createMockError(503, true);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(true);
    });

    it("should retry on 504 timeout when retryOnTimeout is true", () => {
      const error = createMockError(504, true);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(true);
    });

    it("should retry on 499 abort when retryOnTimeout is true", () => {
      const error = createMockError(499, true);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(true);
    });

    it("should retry on 500 internal error", () => {
      const error = createMockError(500, true);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(true);
    });

    it("should retry on 502 bad gateway", () => {
      const error = createMockError(502, true);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(true);
    });
  });

  describe("non-retryable errors", () => {
    it("should not retry on 400 bad request", () => {
      const error = createMockError(400, false);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });

    it("should not retry on 401 unauthorized", () => {
      const error = createMockError(401, false);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });

    it("should not retry on 403 forbidden", () => {
      const error = createMockError(403, false);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });

    it("should not retry on 404 not found", () => {
      const error = createMockError(404, false);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });

    it("should not retry on 422 validation error", () => {
      const error = createMockError(422, false);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });
  });

  describe("retry policy overrides", () => {
    it("should not retry on 429 when retryOn429 is false", () => {
      const error = createMockError(429, true);
      const plan = createMockPlan({ retryOn429: false });

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });

    it("should not retry on 503 when retryOn5xx is false", () => {
      const error = createMockError(503, true);
      const plan = createMockPlan({ retryOn5xx: false });

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });

    it("should not retry on 504 when retryOnTimeout is false", () => {
      const error = createMockError(504, true);
      const plan = createMockPlan({ retryOnTimeout: false });

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });
  });

  describe("attempt limits", () => {
    it("should not retry on last attempt", () => {
      const error = createMockError(503, true);
      const plan = createMockPlan();

      // Index 1 is the last of 2 attempts
      expect(shouldRetry(error, plan, 1)).toBe(false);
    });

    it("should not retry when no more attempts", () => {
      const error = createMockError(503, true);
      const plan = createMockPlan({ attempts: [] });

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });

    it("should retry on first attempt when multiple remain", () => {
      const error = createMockError(503, true);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(true);
    });
  });

  describe("edge cases", () => {
    it("should handle negative status codes", () => {
      const error = createMockError(-1, true);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(false);
    });

    it("should handle status code 599 (edge of 5xx range)", () => {
      const error = createMockError(599, true);
      const plan = createMockPlan();

      expect(shouldRetry(error, plan, 0)).toBe(true);
    });
  });
});

describe("createGatewayError", () => {
  const createMockProviderError = (
    statusCode: number,
    message: string,
  ): ProviderError => {
    const error = new Error(message) as ProviderError;
    error.statusCode = statusCode;
    error.isRetryable = true;
    return error;
  };

  describe("status code mapping", () => {
    it("should map 429 to RATE_LIMITED", () => {
      const error = createMockProviderError(429, "Rate limited");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.code).toBe("RATE_LIMITED");
      expect(gatewayError.type).toBe("gateway_error");
      expect(gatewayError.message).toBe("Rate limited");
    });

    it("should map 504 to TIMEOUT", () => {
      const error = createMockProviderError(504, "Gateway timeout");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.code).toBe("TIMEOUT");
    });

    it("should map 499 to TIMEOUT", () => {
      const error = createMockProviderError(499, "Client closed");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.code).toBe("TIMEOUT");
    });

    it("should map 400 to INVALID_REQUEST", () => {
      const error = createMockProviderError(400, "Bad request");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.code).toBe("INVALID_REQUEST");
    });

    it("should map 500 to UPSTREAM_ERROR", () => {
      const error = createMockProviderError(500, "Internal error");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.code).toBe("UPSTREAM_ERROR");
    });

    it("should map 502 to UPSTREAM_ERROR", () => {
      const error = createMockProviderError(502, "Bad gateway");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.code).toBe("UPSTREAM_ERROR");
    });

    it("should map 503 to UPSTREAM_ERROR", () => {
      const error = createMockProviderError(503, "Service unavailable");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.code).toBe("UPSTREAM_ERROR");
    });
  });

  describe("generic error handling", () => {
    it("should handle non-ProviderError as UPSTREAM_ERROR", () => {
      const error = new Error("Generic error");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.code).toBe("UPSTREAM_ERROR");
      expect(gatewayError.type).toBe("gateway_error");
    });

    it("should preserve error message", () => {
      const error = new Error("Custom error message");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.message).toBe("Custom error message");
    });
  });

  describe("attempts tracking", () => {
    it("should include attempts in details", () => {
      const error = new Error("Error");

      const gatewayError = createGatewayError(error, 3);

      expect(gatewayError.details).toEqual({ attempts: 3 });
    });

    it("should handle zero attempts", () => {
      const error = new Error("Error");

      const gatewayError = createGatewayError(error, 0);

      expect(gatewayError.details).toEqual({ attempts: 0 });
    });

    it("should handle single attempt", () => {
      const error = createMockProviderError(500, "Error");

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.details).toEqual({ attempts: 1 });
    });
  });

  describe("edge cases", () => {
    it("should handle error with no message", () => {
      const error = new Error();

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.message).toBe("");
    });

    it("should handle ProviderError with missing statusCode", () => {
      const error = new Error("Error") as ProviderError;
      error.isRetryable = false;

      const gatewayError = createGatewayError(error, 1);

      expect(gatewayError.code).toBe("UPSTREAM_ERROR");
    });
  });
});
