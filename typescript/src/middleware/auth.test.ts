import { Request, Response, NextFunction } from "express";
import { authMiddleware } from "./auth";
import { getEnv } from "../config/env";

// Mock config
jest.mock("../config/env", () => ({
  getEnv: jest.fn(),
}));

describe("Auth Middleware", () => {
  let mockReq: Partial<Request>;
  let mockRes: Partial<Response>;
  let mockNext: NextFunction;
  let jsonMock: jest.Mock;
  let statusMock: jest.Mock;

  beforeEach(() => {
    jsonMock = jest.fn();
    statusMock = jest.fn().mockReturnThis();

    mockReq = {
      headers: {},
      requestId: "test-request-id",
    };

    mockRes = {
      status: statusMock,
      json: jsonMock,
    };

    mockNext = jest.fn();

    jest.clearAllMocks();
  });

  describe("authMiddleware", () => {
    it("should call next() with valid Bearer token", () => {
      (getEnv as jest.Mock).mockReturnValue({
        GATEWAY_API_KEY: "valid-api-key",
      });

      mockReq.headers = {
        authorization: "Bearer valid-api-key",
      };

      authMiddleware(mockReq as Request, mockRes as Response, mockNext);

      expect(mockNext).toHaveBeenCalled();
      expect(statusMock).not.toHaveBeenCalled();
      expect(jsonMock).not.toHaveBeenCalled();
    });

    it("should return 401 with missing Authorization header", () => {
      (getEnv as jest.Mock).mockReturnValue({
        GATEWAY_API_KEY: "valid-api-key",
      });

      mockReq.headers = {};

      authMiddleware(mockReq as Request, mockRes as Response, mockNext);

      expect(mockNext).not.toHaveBeenCalled();
      expect(statusMock).toHaveBeenCalledWith(401);
      expect(jsonMock).toHaveBeenCalledWith({
        error: {
          type: "authentication_error",
          code: "MISSING_AUTH",
          message:
            "Missing or invalid Authorization header. Expected: Bearer <token>",
          request_id: "test-request-id",
        },
      });
    });

    it("should return 401 with non-Bearer authorization", () => {
      (getEnv as jest.Mock).mockReturnValue({
        GATEWAY_API_KEY: "valid-api-key",
      });

      mockReq.headers = {
        authorization: "Basic dXNlcjpwYXNz",
      };

      authMiddleware(mockReq as Request, mockRes as Response, mockNext);

      expect(mockNext).not.toHaveBeenCalled();
      expect(statusMock).toHaveBeenCalledWith(401);
      expect(jsonMock).toHaveBeenCalledWith({
        error: {
          type: "authentication_error",
          code: "MISSING_AUTH",
          message:
            "Missing or invalid Authorization header. Expected: Bearer <token>",
          request_id: "test-request-id",
        },
      });
    });

    it("should return 401 with invalid API key", () => {
      (getEnv as jest.Mock).mockReturnValue({
        GATEWAY_API_KEY: "valid-api-key",
      });

      mockReq.headers = {
        authorization: "Bearer invalid-key",
      };

      authMiddleware(mockReq as Request, mockRes as Response, mockNext);

      expect(mockNext).not.toHaveBeenCalled();
      expect(statusMock).toHaveBeenCalledWith(401);
      expect(jsonMock).toHaveBeenCalledWith({
        error: {
          type: "authentication_error",
          code: "INVALID_TOKEN",
          message: "Invalid API key",
          request_id: "test-request-id",
        },
      });
    });

    it("should return 500 when API key is not configured", () => {
      (getEnv as jest.Mock).mockReturnValue({
        GATEWAY_API_KEY: "",
      });

      mockReq.headers = {
        authorization: "Bearer some-key",
      };

      authMiddleware(mockReq as Request, mockRes as Response, mockNext);

      expect(mockNext).not.toHaveBeenCalled();
      expect(statusMock).toHaveBeenCalledWith(500);
      expect(jsonMock).toHaveBeenCalledWith({
        error: {
          type: "gateway_error",
          code: "AUTH_NOT_CONFIGURED",
          message: "Gateway API key is not configured",
          request_id: "test-request-id",
        },
      });
    });

    it("should return 401 with Bearer token containing extra spaces", () => {
      (getEnv as jest.Mock).mockReturnValue({
        GATEWAY_API_KEY: "valid-api-key",
      });

      mockReq.headers = {
        authorization: "Bearer valid-api-key  ",
      };

      authMiddleware(mockReq as Request, mockRes as Response, mockNext);

      expect(mockNext).not.toHaveBeenCalled();
      expect(statusMock).toHaveBeenCalledWith(401);
      expect(jsonMock).toHaveBeenCalledWith({
        error: {
          type: "authentication_error",
          code: "INVALID_TOKEN",
          message: "Invalid API key",
          request_id: "test-request-id",
        },
      });
    });

    it("should preserve requestId in error response", () => {
      (getEnv as jest.Mock).mockReturnValue({
        GATEWAY_API_KEY: "valid-api-key",
      });

      mockReq.headers = {};
      mockReq.requestId = "custom-request-id";

      authMiddleware(mockReq as Request, mockRes as Response, mockNext);

      expect(jsonMock).toHaveBeenCalledWith(
        expect.objectContaining({
          error: expect.objectContaining({
            request_id: "custom-request-id",
          }),
        }),
      );
    });
  });
});
