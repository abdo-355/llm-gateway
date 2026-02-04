import { Request, Response } from "express";
import { z } from "zod";
import { validateMiddleware } from "./validate";
import { ValidationError } from "../errors";

describe("Validate Middleware", () => {
  let mockReq: Partial<Request>;
  let mockRes: Partial<Response>;
  let mockNext: jest.Mock<any, any>;

  beforeEach(() => {
    mockReq = {
      body: {},
      requestId: "test-request-id",
    };
    mockRes = {};
    mockNext = jest.fn<any, any>();
  });

  describe("validateMiddleware", () => {
    it("should call next() with valid body", () => {
      const schema = z.object({
        model: z.string(),
        messages: z.array(
          z.object({
            role: z.string(),
            content: z.string(),
          }),
        ),
      });

      mockReq.body = {
        model: "gpt-4",
        messages: [{ role: "user", content: "Hello" }],
      };

      validateMiddleware(schema)(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).toHaveBeenCalled();
      expect(mockNext).toHaveBeenCalledWith();
      expect(mockNext).not.toHaveBeenCalledWith(expect.any(Error));
    });

    it("should attach validated data to request", () => {
      const schema = z.object({
        model: z.string(),
        max_tokens: z.number().optional(),
      });

      mockReq.body = {
        model: "gpt-4",
        max_tokens: 100,
      };

      validateMiddleware(schema)(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect((mockReq as any).validatedBody).toEqual({
        model: "gpt-4",
        max_tokens: 100,
      });
    });

    it("should call next with ValidationError for missing required field", () => {
      const schema = z.object({
        model: z.string(),
        messages: z.array(
          z.object({
            role: z.string(),
            content: z.string(),
          }),
        ),
      });

      mockReq.body = {
        model: "gpt-4",
        messages: [{ role: "user" }],
      };

      validateMiddleware(schema)(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).toHaveBeenCalled();
      const error = mockNext.mock.calls[0][0];
      expect(error).toBeInstanceOf(ValidationError);
      expect(error.message).toBe("Request validation failed");
      expect(error.details).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            path: "messages.0.content",
            message: expect.any(String),
          }),
        ]),
      );
    });

    it("should call next with ValidationError for wrong type", () => {
      const schema = z.object({
        model: z.string(),
        max_tokens: z.number(),
      });

      mockReq.body = {
        model: "gpt-4",
        max_tokens: "not-a-number",
      };

      validateMiddleware(schema)(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).toHaveBeenCalled();
      const error = mockNext.mock.calls[0][0];
      expect(error).toBeInstanceOf(ValidationError);
    });

    it("should handle nested validation errors", () => {
      const schema = z.object({
        messages: z.array(
          z.object({
            role: z.string(),
            content: z.string(),
          }),
        ),
      });

      mockReq.body = {
        messages: [
          { role: "user", content: "Hello" },
          { role: "assistant", content: "Hi there" },
          { role: "user" },
        ],
      };

      validateMiddleware(schema)(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).toHaveBeenCalled();
      const error = mockNext.mock.calls[0][0];
      expect(error).toBeInstanceOf(ValidationError);
      expect(error.details).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            path: "messages.2.content",
          }),
        ]),
      );
    });

    it("should handle empty body with required fields", () => {
      const schema = z.object({
        model: z.string(),
        messages: z.array(
          z.object({
            role: z.string(),
            content: z.string(),
          }),
        ),
      });

      mockReq.body = {};

      validateMiddleware(schema)(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).toHaveBeenCalled();
      const error = mockNext.mock.calls[0][0];
      expect(error).toBeInstanceOf(ValidationError);
    });

    it("should handle invalid array structure", () => {
      const schema = z.object({
        messages: z.array(
          z.object({
            role: z.string(),
            content: z.string(),
          }),
        ),
      });

      mockReq.body = {
        messages: "not-an-array",
      };

      validateMiddleware(schema)(
        mockReq as Request,
        mockRes as Response,
        mockNext,
      );

      expect(mockNext).toHaveBeenCalled();
      const error = mockNext.mock.calls[0][0];
      expect(error).toBeInstanceOf(ValidationError);
    });
  });
});
