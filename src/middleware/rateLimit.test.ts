import { Request, Response, NextFunction } from "express";
import { rateLimitMiddleware } from "./rateLimit";
import { getEnv } from "../config/env";
import { getRedisClient } from "../lib/redis";

jest.mock("../config/env", () => ({
  getEnv: jest.fn(),
}));

jest.mock("../lib/redis", () => ({
  getRedisClient: jest.fn(),
}));

describe("Rate Limit Middleware", () => {
  let mockReq: Partial<Request>;
  let mockRes: Partial<Response>;
  let mockNext: NextFunction;
  let jsonMock: jest.Mock;
  let statusMock: jest.Mock;
  let setHeaderMock: jest.Mock;
  let mockRedis: {
    zremrangebyscore: jest.Mock;
    zcard: jest.Mock;
    zadd: jest.Mock;
    expire: jest.Mock;
  };

  beforeEach(() => {
    jsonMock = jest.fn();
    statusMock = jest.fn().mockReturnThis();
    setHeaderMock = jest.fn();

    mockReq = {
      ip: "192.168.1.1",
      requestId: "test-request-id",
      headers: {},
    };

    mockRes = {
      status: statusMock,
      json: jsonMock,
      setHeader: setHeaderMock,
    };

    mockNext = jest.fn();

    mockRedis = {
      zremrangebyscore: jest.fn().mockResolvedValue(0),
      zcard: jest.fn().mockResolvedValue(0),
      zadd: jest.fn().mockResolvedValue(1),
      expire: jest.fn().mockResolvedValue(1),
    };

    (getRedisClient as jest.Mock).mockReturnValue(mockRedis);
    jest.clearAllMocks();
  });

  describe("rateLimitMiddleware", () => {
    it("should allow request when under rate limit", async () => {
      (getEnv as jest.Mock).mockReturnValue({
        RATE_LIMIT_PER_IP: 100,
        RATE_LIMIT_WINDOW_MS: 60000,
      });

      mockRedis.zcard.mockResolvedValue(50);

      await rateLimitMiddleware(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).toHaveBeenCalled();
      expect(statusMock).not.toHaveBeenCalled();
      expect(jsonMock).not.toHaveBeenCalled();
      expect(setHeaderMock).toHaveBeenCalledWith("X-RateLimit-Limit", "100");
      expect(setHeaderMock).toHaveBeenCalledWith("X-RateLimit-Remaining", "49");
    });

    it("should block request when rate limit exceeded", async () => {
      (getEnv as jest.Mock).mockReturnValue({
        RATE_LIMIT_PER_IP: 100,
        RATE_LIMIT_WINDOW_MS: 60000,
      });

      mockRedis.zcard.mockResolvedValue(100);

      await rateLimitMiddleware(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).not.toHaveBeenCalled();
      expect(statusMock).toHaveBeenCalledWith(429);
      expect(jsonMock).toHaveBeenCalledWith({
        error: {
          type: "rate_limit_error",
          code: "RATE_LIMIT_EXCEEDED",
          message: "Rate limit exceeded. Maximum 100 requests per 60 seconds.",
          request_id: "test-request-id",
        },
      });
    });

    it("should use correct Redis key with IP", async () => {
      (getEnv as jest.Mock).mockReturnValue({
        RATE_LIMIT_PER_IP: 100,
        RATE_LIMIT_WINDOW_MS: 60000,
      });

      mockReq = {
        ...mockReq,
        ip: "10.0.0.1",
      };

      await rateLimitMiddleware(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockRedis.zremrangebyscore).toHaveBeenCalledWith(
        "rate_limit:10.0.0.1",
        expect.any(Number),
        expect.any(Number),
      );
      expect(mockRedis.zcard).toHaveBeenCalledWith("rate_limit:10.0.0.1");
      expect(mockRedis.zadd).toHaveBeenCalledWith(
        "rate_limit:10.0.0.1",
        expect.any(Number),
        expect.stringContaining("-"),
      );
    });

    it("should set correct remaining header", async () => {
      (getEnv as jest.Mock).mockReturnValue({
        RATE_LIMIT_PER_IP: 10,
        RATE_LIMIT_WINDOW_MS: 60000,
      });

      mockRedis.zcard.mockResolvedValue(5);

      await rateLimitMiddleware(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(setHeaderMock).toHaveBeenCalledWith("X-RateLimit-Remaining", "4");
    });

    it("should set expiry on Redis key", async () => {
      (getEnv as jest.Mock).mockReturnValue({
        RATE_LIMIT_PER_IP: 100,
        RATE_LIMIT_WINDOW_MS: 120000,
      });

      await rateLimitMiddleware(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockRedis.expire).toHaveBeenCalledWith(
        "rate_limit:192.168.1.1",
        120,
      );
    });

    it("should handle missing IP gracefully", async () => {
      (getEnv as jest.Mock).mockReturnValue({
        RATE_LIMIT_PER_IP: 100,
        RATE_LIMIT_WINDOW_MS: 60000,
      });

      mockReq = {
        ...mockReq,
        ip: undefined,
        socket: { remoteAddress: "127.0.0.1" } as any,
      };

      await rateLimitMiddleware(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).toHaveBeenCalled();
      expect(mockRedis.zcard).toHaveBeenCalledWith("rate_limit:127.0.0.1");
    });

    it("should allow request when Redis fails", async () => {
      (getEnv as jest.Mock).mockReturnValue({
        RATE_LIMIT_PER_IP: 100,
        RATE_LIMIT_WINDOW_MS: 60000,
      });

      mockRedis.zremrangebyscore.mockRejectedValue(
        new Error("Redis connection failed"),
      );

      await rateLimitMiddleware(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).toHaveBeenCalled();
      expect(statusMock).not.toHaveBeenCalled();
      expect(jsonMock).not.toHaveBeenCalled();
    });

    it("should handle zero count correctly", async () => {
      (getEnv as jest.Mock).mockReturnValue({
        RATE_LIMIT_PER_IP: 100,
        RATE_LIMIT_WINDOW_MS: 60000,
      });

      mockRedis.zcard.mockResolvedValue(0);

      await rateLimitMiddleware(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(setHeaderMock).toHaveBeenCalledWith("X-RateLimit-Remaining", "99");
    });
  });
});
