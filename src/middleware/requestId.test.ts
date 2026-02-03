import { Request, Response, NextFunction } from "express";
import { requestIdMiddleware } from "./requestId";

// Mock uuid
jest.mock("uuid", () => ({
  v4: jest.fn().mockReturnValue("mock-uuid-1234"),
}));

describe("requestIdMiddleware", () => {
  let req: Partial<Request>;
  let res: Partial<Response>;
  let next: NextFunction;

  beforeEach(() => {
    req = {
      headers: {},
    };
    res = {
      setHeader: jest.fn(),
    };
    next = jest.fn();
  });

  it("should generate a new request ID when none provided", () => {
    requestIdMiddleware(req as Request, res as Response, next);

    expect(req.requestId).toBe("mock-uuid-1234");
    expect(res.setHeader).toHaveBeenCalledWith(
      "X-Request-ID",
      "mock-uuid-1234",
    );
    expect(next).toHaveBeenCalled();
  });

  it("should use provided request ID from header", () => {
    req.headers = { "x-request-id": "provided-id-123" };

    requestIdMiddleware(req as Request, res as Response, next);

    expect(req.requestId).toBe("provided-id-123");
    expect(res.setHeader).toHaveBeenCalledWith(
      "X-Request-ID",
      "provided-id-123",
    );
    expect(next).toHaveBeenCalled();
  });

  it("should call next() to continue to next middleware", () => {
    requestIdMiddleware(req as Request, res as Response, next);

    expect(next).toHaveBeenCalledTimes(1);
    expect(next).toHaveBeenCalledWith();
  });

  it("should handle case-insensitive header key", () => {
    req.headers = { "X-Request-ID": "uppercase-id" };

    requestIdMiddleware(req as Request, res as Response, next);

    // Express lowercases header keys, but we access via 'x-request-id'
    expect(req.requestId).toBe("mock-uuid-1234");
  });

  it("should set requestId as string type", () => {
    requestIdMiddleware(req as Request, res as Response, next);

    expect(typeof req.requestId).toBe("string");
    expect(req.requestId).toHaveLength(14); // 'mock-uuid-1234'
  });
});
