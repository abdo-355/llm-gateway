import { HealthService } from "./health";
import { CircuitBreakerError } from "../errors";
import { getRedisClient } from "../lib/redis";

// Mock Redis
jest.mock("../lib/redis");

describe("HealthService", () => {
  let healthService: HealthService;
  let mockRedis: jest.Mocked<any>;

  beforeEach(() => {
    mockRedis = {
      get: jest.fn(),
      set: jest.fn(),
      setex: jest.fn(),
      incr: jest.fn(),
      scan: jest.fn(),
    };
    (getRedisClient as jest.Mock).mockReturnValue(mockRedis);
    healthService = new HealthService();
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  describe("getCircuitState", () => {
    it("should return CLOSED when no state is set", async () => {
      mockRedis.get.mockResolvedValue(null);

      const state = await healthService.getCircuitState("groq");

      expect(state).toBe("CLOSED");
      expect(mockRedis.get).toHaveBeenCalledWith("circuit:groq:state");
    });

    it("should return the stored state", async () => {
      mockRedis.get.mockResolvedValue("OPEN");

      const state = await healthService.getCircuitState("groq");

      expect(state).toBe("OPEN");
    });
  });

  describe("canExecute", () => {
    it("should return true when circuit is CLOSED", async () => {
      mockRedis.get.mockResolvedValue("CLOSED");

      const canExecute = await healthService.canExecute("groq");

      expect(canExecute).toBe(true);
    });

    it("should return false when circuit is OPEN and timeout has not passed", async () => {
      mockRedis.get
        .mockResolvedValueOnce("OPEN")
        .mockResolvedValueOnce(Date.now().toString());

      const canExecute = await healthService.canExecute("groq");

      expect(canExecute).toBe(false);
    });

    it("should transition to HALF_OPEN and return true when timeout has passed", async () => {
      const oldTimestamp = (Date.now() - 31000).toString(); // 31 seconds ago
      mockRedis.get
        .mockResolvedValueOnce("OPEN")
        .mockResolvedValueOnce(oldTimestamp);
      mockRedis.set.mockResolvedValue("OK");

      const canExecute = await healthService.canExecute("groq");

      expect(canExecute).toBe(true);
      expect(mockRedis.set).toHaveBeenCalledWith(
        "circuit:groq:state",
        "HALF_OPEN",
      );
    });

    it("should allow probe request in HALF_OPEN state", async () => {
      mockRedis.get
        .mockResolvedValueOnce("HALF_OPEN")
        .mockResolvedValueOnce("0") // successes
        .mockResolvedValueOnce("0"); // failures

      const canExecute = await healthService.canExecute("groq");

      expect(canExecute).toBe(true);
    });

    it("should not allow more than 1 probe request in HALF_OPEN state", async () => {
      mockRedis.get
        .mockResolvedValueOnce("HALF_OPEN")
        .mockResolvedValueOnce("1") // successes
        .mockResolvedValueOnce("0"); // failures

      const canExecute = await healthService.canExecute("groq");

      expect(canExecute).toBe(false);
    });
  });

  describe("checkCircuitBreakerOrThrow", () => {
    it("should not throw when circuit is CLOSED", async () => {
      mockRedis.get.mockResolvedValue("CLOSED");

      await expect(
        healthService.checkCircuitBreakerOrThrow("groq"),
      ).resolves.not.toThrow();
    });

    it("should throw CircuitBreakerError when circuit is OPEN", async () => {
      mockRedis.get.mockResolvedValue("OPEN");

      await expect(
        healthService.checkCircuitBreakerOrThrow("groq"),
      ).rejects.toThrow(CircuitBreakerError);
    });

    it("should throw CircuitBreakerError when probe limit reached in HALF_OPEN", async () => {
      mockRedis.get
        .mockResolvedValueOnce("HALF_OPEN")
        .mockResolvedValueOnce("1") // successes
        .mockResolvedValueOnce("0"); // failures

      await expect(
        healthService.checkCircuitBreakerOrThrow("groq"),
      ).rejects.toThrow(CircuitBreakerError);
    });

    it("should not throw when probe limit not reached in HALF_OPEN", async () => {
      mockRedis.get
        .mockResolvedValueOnce("HALF_OPEN")
        .mockResolvedValueOnce("0") // successes
        .mockResolvedValueOnce("0"); // failures

      await expect(
        healthService.checkCircuitBreakerOrThrow("groq"),
      ).resolves.not.toThrow();
    });
  });

  describe("recordSuccess", () => {
    it("should reset failure count on success in CLOSED state", async () => {
      mockRedis.get.mockResolvedValue("CLOSED");
      mockRedis.set.mockResolvedValue("OK");
      mockRedis.setex.mockResolvedValue("OK");

      await healthService.recordSuccess("groq", 100);

      expect(mockRedis.set).toHaveBeenCalledWith("circuit:groq:failures", "0");
    });

    it("should transition to CLOSED after success in HALF_OPEN state", async () => {
      mockRedis.get.mockResolvedValue("HALF_OPEN");
      mockRedis.incr.mockResolvedValue(1);
      mockRedis.set.mockResolvedValue("OK");
      mockRedis.setex.mockResolvedValue("OK");

      await healthService.recordSuccess("groq", 100);

      expect(mockRedis.set).toHaveBeenCalledWith(
        "circuit:groq:state",
        "CLOSED",
      );
    });

    it("should record latency with TTL", async () => {
      mockRedis.get.mockResolvedValue("CLOSED");
      mockRedis.set.mockResolvedValue("OK");
      mockRedis.setex.mockResolvedValue("OK");

      await healthService.recordSuccess("groq", 150);

      expect(mockRedis.setex).toHaveBeenCalledWith(
        "health:groq:latency",
        3600,
        "150",
      );
    });
  });

  describe("recordFailure", () => {
    it("should increment failure count", async () => {
      mockRedis.get.mockResolvedValue("CLOSED");
      mockRedis.incr.mockResolvedValue(1);
      mockRedis.set.mockResolvedValue("OK");
      mockRedis.setex.mockResolvedValue("OK");

      await healthService.recordFailure("groq");

      expect(mockRedis.incr).toHaveBeenCalledWith("circuit:groq:failures");
      expect(mockRedis.set).toHaveBeenCalledWith(
        "circuit:groq:last_failure",
        expect.any(String),
      );
    });

    it("should open circuit after 5 failures", async () => {
      mockRedis.get.mockResolvedValue("CLOSED");
      mockRedis.incr.mockResolvedValue(5);
      mockRedis.set.mockResolvedValue("OK");
      mockRedis.setex.mockResolvedValue("OK");

      await healthService.recordFailure("groq");

      expect(mockRedis.set).toHaveBeenCalledWith("circuit:groq:state", "OPEN");
    });

    it("should re-open circuit on failure in HALF_OPEN state", async () => {
      mockRedis.get.mockResolvedValue("HALF_OPEN");
      mockRedis.incr.mockResolvedValue(1);
      mockRedis.set.mockResolvedValue("OK");
      mockRedis.setex.mockResolvedValue("OK");

      await healthService.recordFailure("groq");

      expect(mockRedis.set).toHaveBeenCalledWith("circuit:groq:state", "OPEN");
    });
  });

  describe("getHealthMetrics", () => {
    it("should return complete health metrics", async () => {
      mockRedis.get
        .mockResolvedValueOnce("CLOSED")
        .mockResolvedValueOnce("2")
        .mockResolvedValueOnce("10")
        .mockResolvedValueOnce("1234567890")
        .mockResolvedValueOnce("150")
        .mockResolvedValueOnce("0.9");

      const metrics = await healthService.getHealthMetrics("groq");

      expect(metrics).toEqual({
        providerId: "groq",
        circuitState: "CLOSED",
        failureCount: 2,
        successCount: 10,
        lastFailureTime: 1234567890,
        averageLatency: 150,
        healthScore: 0.9,
      });
    });

    it("should handle null values gracefully", async () => {
      mockRedis.get
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce(null);

      const metrics = await healthService.getHealthMetrics("groq");

      expect(metrics.failureCount).toBe(0);
      expect(metrics.successCount).toBe(0);
      expect(metrics.lastFailureTime).toBeNull();
      expect(metrics.averageLatency).toBeNull();
      expect(metrics.healthScore).toBe(1.0);
    });
  });

  describe("getAllHealthMetrics", () => {
    it("should return metrics for all providers using SCAN", async () => {
      // SCAN returns [cursor, [keys...]] - cursor "0" means done
      mockRedis.scan.mockResolvedValue([
        "0",
        ["circuit:groq:state", "circuit:mistral:state"],
      ]);
      mockRedis.get
        .mockResolvedValueOnce("CLOSED")
        .mockResolvedValueOnce("0")
        .mockResolvedValueOnce("0")
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce("1.0")
        .mockResolvedValueOnce("OPEN")
        .mockResolvedValueOnce("5")
        .mockResolvedValueOnce("0")
        .mockResolvedValueOnce("1234567890")
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce("0.0");

      const metrics = await healthService.getAllHealthMetrics();

      expect(metrics).toHaveLength(2);
      expect(metrics[0].providerId).toBe("groq");
      expect(metrics[1].providerId).toBe("mistral");
      // Verify SCAN was called with correct parameters
      expect(mockRedis.scan).toHaveBeenCalledWith(
        "0",
        "MATCH",
        "circuit:*:state",
        "COUNT",
        100,
      );
    });

    it("should return empty array when no providers", async () => {
      mockRedis.scan.mockResolvedValue(["0", []]);

      const metrics = await healthService.getAllHealthMetrics();

      expect(metrics).toEqual([]);
    });

    it("should handle multiple SCAN iterations", async () => {
      // First SCAN returns partial results with non-zero cursor
      mockRedis.scan
        .mockResolvedValueOnce(["123", ["circuit:groq:state"]])
        .mockResolvedValueOnce(["0", ["circuit:mistral:state"]]);

      mockRedis.get
        .mockResolvedValueOnce("CLOSED")
        .mockResolvedValueOnce("0")
        .mockResolvedValueOnce("0")
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce("1.0")
        .mockResolvedValueOnce("CLOSED")
        .mockResolvedValueOnce("0")
        .mockResolvedValueOnce("0")
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce(null)
        .mockResolvedValueOnce("1.0");

      const metrics = await healthService.getAllHealthMetrics();

      expect(metrics).toHaveLength(2);
      expect(mockRedis.scan).toHaveBeenCalledTimes(2);
    });
  });
});
