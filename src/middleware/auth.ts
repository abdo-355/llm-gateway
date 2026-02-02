import { Request, Response, NextFunction } from "express";
import { getEnv } from "../config/env";
import { GatewayError } from "../types";

// Extend Express Request
declare global {
  namespace Express {
    interface Request {
      requestId: string;
    }
  }
}

export function authMiddleware(
  req: Request,
  res: Response,
  next: NextFunction,
): void {
  const env = getEnv();
  const apiKey = env.GATEWAY_API_KEY;

  if (!apiKey) {
    const error: GatewayError = {
      type: "gateway_error",
      code: "AUTH_NOT_CONFIGURED",
      message: "Gateway API key is not configured",
      request_id: req.requestId,
    };
    res.status(500).json({ error });
    return;
  }

  const authHeader = req.headers.authorization;

  if (!authHeader || !authHeader.startsWith("Bearer ")) {
    const error: GatewayError = {
      type: "authentication_error",
      code: "MISSING_AUTH",
      message:
        "Missing or invalid Authorization header. Expected: Bearer <token>",
      request_id: req.requestId,
    };
    res.status(401).json({ error });
    return;
  }

  const token = authHeader.slice(7);

  if (token !== apiKey) {
    const error: GatewayError = {
      type: "authentication_error",
      code: "INVALID_TOKEN",
      message: "Invalid API key",
      request_id: req.requestId,
    };
    res.status(401).json({ error });
    return;
  }

  next();
}
