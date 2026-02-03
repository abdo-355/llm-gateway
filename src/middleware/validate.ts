import { Request, Response, NextFunction } from "express";
import { ZodSchema } from "zod";
import { ValidationError } from "../errors";

export function validateMiddleware(schema: ZodSchema) {
  return (req: Request, _res: Response, next: NextFunction): void => {
    const result = schema.safeParse(req.body);

    if (!result.success) {
      const validationDetails = result.error.errors.map((e) => ({
        path: e.path.join("."),
        message: e.message,
      }));

      const error = new ValidationError(
        "Request validation failed",
        validationDetails,
      );

      next(error);
      return;
    }

    // Store validated data
    (req as any).validatedBody = result.data;
    next();
  };
}
