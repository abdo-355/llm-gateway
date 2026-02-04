import { Request, Response } from "express";
import { healthHandler } from "./health";
import { loadConfig } from "../config";
import { healthService } from "../services/health";

jest.mock("../config", () => ({
  loadConfig: jest.fn(),
}));

jest.mock("../services/health", () => ({
  healthService: {
    getAllHealthMetrics: jest.fn(),
  },
}));

jest.mock("../utils/logger", () => ({
  logger: {
    error: jest.fn(),
  },
}));

describe("Health Route", () => {
  let mockReq: Partial<Request>;
  let mockRes: Partial<Response>;
  let jsonMock: jest.Mock;
  let statusMock: jest.Mock;

  beforeEach(() => {
    jsonMock = jest.fn();
    statusMock = jest.fn().mockReturnThis();

    mockReq = {};

    mockRes = {
      json: jsonMock,
      status: statusMock,
    };

    jest.clearAllMocks();
  });

  describe("healthHandler", () => {
    it("should return healthy status when all circuits are closed", async () => {
      (loadConfig as jest.Mock).mockReturnValue({
        providers: [
          {
            id: "groq",
            limits: { rpm: 1000, dailyRequests: 10000 },
          },
          {
            id: "cerebras",
            limits: { rpm: 500, dailyRequests: 5000 },
          },
        ],
      });

      (healthService.getAllHealthMetrics as jest.Mock).mockResolvedValue([
        {
          providerId: "groq",
          circuitState: "CLOSED",
          healthScore: 1.0,
          averageLatency: 150,
        },
        {
          providerId: "cerebras",
          circuitState: "CLOSED",
          healthScore: 0.95,
          averageLatency: 200,
        },
      ]);

      await healthHandler(mockReq as Request, mockRes as Response);

      expect(jsonMock).toHaveBeenCalledWith(
        expect.objectContaining({
          status: "healthy",
          providers: expect.arrayContaining([
            expect.objectContaining({
              id: "groq",
              circuit_state: "CLOSED",
              health_score: 1.0,
            }),
            expect.objectContaining({
              id: "cerebras",
              circuit_state: "CLOSED",
              health_score: 0.95,
            }),
          ]),
        }),
      );
      expect(statusMock).not.toHaveBeenCalled();
    });

    it("should return degraded status when any circuit is open", async () => {
      (loadConfig as jest.Mock).mockReturnValue({
        providers: [
          {
            id: "groq",
            limits: { rpm: 1000, dailyRequests: 10000 },
          },
          {
            id: "cerebras",
            limits: { rpm: 500, dailyRequests: 5000 },
          },
        ],
      });

      (healthService.getAllHealthMetrics as jest.Mock).mockResolvedValue([
        {
          providerId: "groq",
          circuitState: "OPEN",
          healthScore: 0,
          averageLatency: null,
        },
        {
          providerId: "cerebras",
          circuitState: "CLOSED",
          healthScore: 1.0,
          averageLatency: 150,
        },
      ]);

      await healthHandler(mockReq as Request, mockRes as Response);

      expect(jsonMock).toHaveBeenCalledWith(
        expect.objectContaining({
          status: "degraded",
          providers: expect.arrayContaining([
            expect.objectContaining({
              id: "groq",
              circuit_state: "OPEN",
            }),
          ]),
        }),
      );
    });

    it("should return healthy for provider with no metrics", async () => {
      (loadConfig as jest.Mock).mockReturnValue({
        providers: [
          {
            id: "groq",
            limits: { rpm: 1000 },
          },
        ],
      });

      (healthService.getAllHealthMetrics as jest.Mock).mockResolvedValue([]);

      await healthHandler(mockReq as Request, mockRes as Response);

      expect(jsonMock).toHaveBeenCalledWith(
        expect.objectContaining({
          status: "healthy",
          providers: expect.arrayContaining([
            expect.objectContaining({
              id: "groq",
              circuit_state: "CLOSED",
              health_score: 1.0,
            }),
          ]),
        }),
      );
    });

    it("should include quota limits in response", async () => {
      (loadConfig as jest.Mock).mockReturnValue({
        providers: [
          {
            id: "groq",
            limits: { rpm: 1000, dailyRequests: 5000 },
          },
        ],
      });

      (healthService.getAllHealthMetrics as jest.Mock).mockResolvedValue([
        {
          providerId: "groq",
          circuitState: "CLOSED",
          healthScore: 1.0,
          averageLatency: 100,
        },
      ]);

      await healthHandler(mockReq as Request, mockRes as Response);

      expect(jsonMock).toHaveBeenCalledWith(
        expect.objectContaining({
          providers: expect.arrayContaining([
            expect.objectContaining({
              id: "groq",
              quota: {
                rpm: 1000,
                daily_requests: 5000,
              },
            }),
          ]),
        }),
      );
    });

    it("should return null quota when no limits configured", async () => {
      (loadConfig as jest.Mock).mockReturnValue({
        providers: [
          {
            id: "groq",
            limits: undefined,
          },
        ],
      });

      (healthService.getAllHealthMetrics as jest.Mock).mockResolvedValue([
        {
          providerId: "groq",
          circuitState: "CLOSED",
          healthScore: 1.0,
          averageLatency: 100,
        },
      ]);

      await healthHandler(mockReq as Request, mockRes as Response);

      expect(jsonMock).toHaveBeenCalledWith(
        expect.objectContaining({
          providers: expect.arrayContaining([
            expect.objectContaining({
              id: "groq",
              quota: null,
            }),
          ]),
        }),
      );
    });

    it("should return 500 when health service throws", async () => {
      (loadConfig as jest.Mock).mockReturnValue({
        providers: [{ id: "groq" }],
      });

      (healthService.getAllHealthMetrics as jest.Mock).mockRejectedValue(
        new Error("Redis connection failed"),
      );

      await healthHandler(mockReq as Request, mockRes as Response);

      expect(statusMock).toHaveBeenCalledWith(500);
      expect(jsonMock).toHaveBeenCalledWith({
        status: "unhealthy",
        error: "Failed to check health",
      });
    });

    it("should include timestamp in response", async () => {
      (loadConfig as jest.Mock).mockReturnValue({
        providers: [],
      });

      (healthService.getAllHealthMetrics as jest.Mock).mockResolvedValue([]);

      await healthHandler(mockReq as Request, mockRes as Response);

      expect(jsonMock).toHaveBeenCalledWith(
        expect.objectContaining({
          timestamp: expect.any(String),
        }),
      );
    });

    it("should handle HALF_OPEN circuit state as healthy", async () => {
      (loadConfig as jest.Mock).mockReturnValue({
        providers: [
          {
            id: "groq",
            limits: {},
          },
        ],
      });

      (healthService.getAllHealthMetrics as jest.Mock).mockResolvedValue([
        {
          providerId: "groq",
          circuitState: "HALF_OPEN",
          healthScore: 0.5,
          averageLatency: 500,
        },
      ]);

      await healthHandler(mockReq as Request, mockRes as Response);

      expect(jsonMock).toHaveBeenCalledWith(
        expect.objectContaining({
          status: "healthy",
        }),
      );
    });
  });
});
