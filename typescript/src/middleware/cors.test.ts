import { Request, Response, NextFunction } from "express";
import { corsMiddleware } from "./cors";

describe("corsMiddleware", () => {
  let req: Partial<Request>;
  let res: Partial<Response>;
  let next: NextFunction;

  beforeEach(() => {
    req = {
      method: "GET",
      headers: {},
    };
    res = {
      setHeader: jest.fn(),
      status: jest.fn().mockReturnThis(),
      end: jest.fn(),
    };
    next = jest.fn();
    delete process.env.CORS_ORIGINS;
  });

  afterEach(() => {
    delete process.env.CORS_ORIGINS;
  });

  describe("when no origin header", () => {
    it("should allow request and call next", () => {
      corsMiddleware(req as Request, res as Response, next);

      expect(next).toHaveBeenCalled();
      expect(res.setHeader).not.toHaveBeenCalled();
    });
  });

  describe("when CORS_ORIGINS not set (allow all)", () => {
    beforeEach(() => {
      req.headers = { origin: "https://example.com" };
    });

    it("should set CORS headers for allowed origin", () => {
      corsMiddleware(req as Request, res as Response, next);

      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Allow-Origin",
        "https://example.com",
      );
      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Allow-Methods",
        "GET, POST, OPTIONS",
      );
      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Allow-Headers",
        "Content-Type, Authorization, X-Request-ID",
      );
      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Max-Age",
        "86400",
      );
      expect(next).toHaveBeenCalled();
    });
  });

  describe("when CORS_ORIGINS is set", () => {
    beforeEach(() => {
      process.env.CORS_ORIGINS = "https://app.com,https://admin.com";
    });

    it("should allow request from allowed origin", () => {
      req.headers = { origin: "https://app.com" };

      corsMiddleware(req as Request, res as Response, next);

      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Allow-Origin",
        "https://app.com",
      );
      expect(next).toHaveBeenCalled();
    });

    it("should allow request from another allowed origin", () => {
      req.headers = { origin: "https://admin.com" };

      corsMiddleware(req as Request, res as Response, next);

      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Allow-Origin",
        "https://admin.com",
      );
      expect(next).toHaveBeenCalled();
    });

    it("should not set CORS headers for disallowed origin", () => {
      req.headers = { origin: "https://evil.com" };

      corsMiddleware(req as Request, res as Response, next);

      expect(res.setHeader).not.toHaveBeenCalled();
      expect(next).toHaveBeenCalled();
    });

    it("should handle whitespace in origins list", () => {
      process.env.CORS_ORIGINS = " https://app.com , https://admin.com ";
      req.headers = { origin: "https://app.com" };

      corsMiddleware(req as Request, res as Response, next);

      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Allow-Origin",
        "https://app.com",
      );
    });
  });

  describe("OPTIONS request handling", () => {
    beforeEach(() => {
      req.method = "OPTIONS";
      req.headers = { origin: "https://example.com" };
    });

    it("should return 204 for preflight request", () => {
      corsMiddleware(req as Request, res as Response, next);

      expect(res.status).toHaveBeenCalledWith(204);
      expect(res.end).toHaveBeenCalled();
      expect(next).not.toHaveBeenCalled();
    });

    it("should set CORS headers before returning 204", () => {
      corsMiddleware(req as Request, res as Response, next);

      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Allow-Origin",
        "https://example.com",
      );
      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Allow-Methods",
        "GET, POST, OPTIONS",
      );
    });
  });

  describe("edge cases", () => {
    it("should handle empty CORS_ORIGINS string", () => {
      process.env.CORS_ORIGINS = "";
      req.headers = { origin: "https://example.com" };

      corsMiddleware(req as Request, res as Response, next);

      expect(res.setHeader).toHaveBeenCalled();
    });

    it("should handle multiple commas in CORS_ORIGINS", () => {
      process.env.CORS_ORIGINS = "https://app.com,,https://admin.com";
      req.headers = { origin: "https://app.com" };

      corsMiddleware(req as Request, res as Response, next);

      expect(res.setHeader).toHaveBeenCalledWith(
        "Access-Control-Allow-Origin",
        "https://app.com",
      );
    });
  });
});
