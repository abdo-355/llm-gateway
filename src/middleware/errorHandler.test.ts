import { Request, Response, NextFunction } from "express";
import { errorHandler } from "./errorHandler";
import { logger } from "../utils/logger";

// Mock logger
jest.mock("../utils/logger", () => ({
  logger: {
    error: jest.fn(),
  },
}));

describe("errorHandler", () => {
  let req: Partial<Request>;
  let res: Partial<Response>;
  let next: NextFunction;
  const originalEnv = process.env.NODE_ENV;

  beforeEach(() => {
    req = {
      requestId: "req-123",
    };
    res = {
      status: jest.fn().mockReturnThis(),
      json: jest.fn().mockReturnThis(),
    };
    next = jest.fn();
    jest.clearAllMocks();
    delete process.env.NODE_ENV;
  });

  afterEach(() => {
    process.env.NODE_ENV = originalEnv;
  });

  describe("response structure", () => {
    it("should return HTTP 500 status", () => {
      const error = new Error("Test error");

      errorHandler(error, req as Request, res as Response, next);

      expect(res.status).toHaveBeenCalledWith(500);
      expect(res.status).toHaveBeenCalledTimes(1);
    });

    it("should return JSON response with error property", () => {
      const error = new Error("Test error");

      errorHandler(error, req as Request, res as Response, next);

      expect(res.json).toHaveBeenCalledTimes(1);
      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response).toHaveProperty("error");
      expect(response.error).toHaveProperty("type", "gateway_error");
      expect(response.error).toHaveProperty("code", "INTERNAL_ERROR");
      expect(response.error).toHaveProperty("message");
      expect(response.error).toHaveProperty("request_id");
    });

    it("should include request_id from request object", () => {
      const error = new Error("Test error");

      errorHandler(error, req as Request, res as Response, next);

      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response.error.request_id).toBe("req-123");
    });
  });

  describe("environment-specific messages", () => {
    it("should show actual error message in development", () => {
      process.env.NODE_ENV = "development";
      const error = new Error("Detailed error message");

      errorHandler(error, req as Request, res as Response, next);

      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response.error.message).toBe("Detailed error message");
    });

    it("should show generic message in production", () => {
      process.env.NODE_ENV = "production";
      const error = new Error("Sensitive internal details");

      errorHandler(error, req as Request, res as Response, next);

      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response.error.message).toBe("An internal error occurred");
    });

    it("should show error message when NODE_ENV is undefined", () => {
      const error = new Error("Test message");

      errorHandler(error, req as Request, res as Response, next);

      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response.error.message).toBe("Test message");
    });
  });

  describe("logging", () => {
    it("should log error with unhandled_error event", () => {
      const error = new Error("Test error");

      errorHandler(error, req as Request, res as Response, next);

      expect(logger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          event: "unhandled_error",
        }),
      );
    });

    it("should include error message in log", () => {
      const error = new Error("Specific error message");

      errorHandler(error, req as Request, res as Response, next);

      expect(logger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          error: "Specific error message",
        }),
      );
    });

    it("should include stack trace in log", () => {
      const error = new Error("Test error");

      errorHandler(error, req as Request, res as Response, next);

      expect(logger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          stack: expect.stringContaining("Error: Test error"),
        }),
      );
    });

    it("should include request_id in log", () => {
      const error = new Error("Test error");

      errorHandler(error, req as Request, res as Response, next);

      expect(logger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          request_id: "req-123",
        }),
      );
    });

    it("should handle undefined requestId", () => {
      req.requestId = undefined;
      const error = new Error("Test error");

      errorHandler(error, req as Request, res as Response, next);

      expect(logger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          request_id: undefined,
        }),
      );

      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response.error.request_id).toBeUndefined();
    });
  });

  describe("error object handling", () => {
    it("should handle standard Error object", () => {
      const error = new Error("Standard error");

      errorHandler(error, req as Request, res as Response, next);

      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response.error.message).toBe("Standard error");
    });

    it("should handle custom error subclasses", () => {
      class CustomError extends Error {
        constructor(message: string) {
          super(message);
          this.name = "CustomError";
        }
      }
      const error = new CustomError("Custom error message");

      errorHandler(error, req as Request, res as Response, next);

      expect(logger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          error: "Custom error message",
        }),
      );
    });

    it("should handle error with empty message", () => {
      const error = new Error("");

      errorHandler(error, req as Request, res as Response, next);

      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response.error.message).toBe("");
      expect(logger.error).toHaveBeenCalledWith(
        expect.objectContaining({
          error: "",
        }),
      );
    });

    it("should handle error with undefined message gracefully", () => {
      const error = new Error();

      errorHandler(error, req as Request, res as Response, next);

      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response.error.message).toBe("");
    });
  });

  describe("Express middleware contract", () => {
    it("should accept 4 parameters", () => {
      const error = new Error("Test");

      // Should not throw
      expect(() => {
        errorHandler(error, req as Request, res as Response, next);
      }).not.toThrow();
    });

    it("should call res.status exactly once", () => {
      const error = new Error("Test");

      errorHandler(error, req as Request, res as Response, next);

      expect(res.status).toHaveBeenCalledTimes(1);
    });

    it("should call res.json exactly once", () => {
      const error = new Error("Test");

      errorHandler(error, req as Request, res as Response, next);

      expect(res.json).toHaveBeenCalledTimes(1);
    });

    it("should NOT call next function (error handlers terminate)", () => {
      const error = new Error("Test");

      errorHandler(error, req as Request, res as Response, next);

      expect(next).not.toHaveBeenCalled();
    });

    it("should pass correct error structure to res.json", () => {
      const error = new Error("Test message");

      errorHandler(error, req as Request, res as Response, next);

      expect(res.json).toHaveBeenCalledWith({
        error: {
          type: "gateway_error",
          code: "INTERNAL_ERROR",
          message: "Test message",
          request_id: "req-123",
        },
      });
    });
  });

  describe("error type fields", () => {
    it("should always set type to gateway_error", () => {
      const errors = [
        new Error("Error 1"),
        new TypeError("Error 2"),
        new RangeError("Error 3"),
      ];

      errors.forEach((error) => {
        const mockRes = {
          status: jest.fn().mockReturnThis(),
          json: jest.fn().mockReturnThis(),
        };
        errorHandler(
          error,
          req as Request,
          mockRes as unknown as Response,
          next,
        );

        const response = mockRes.json.mock.calls[0][0];
        expect(response.error.type).toBe("gateway_error");
      });
    });

    it("should always set code to INTERNAL_ERROR", () => {
      const error = new Error("Test");

      errorHandler(error, req as Request, res as Response, next);

      const response = (res.json as jest.Mock).mock.calls[0][0];
      expect(response.error.code).toBe("INTERNAL_ERROR");
    });
  });
});
