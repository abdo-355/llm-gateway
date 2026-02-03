import { Request, Response, NextFunction } from "express";
import { v4 as uuidv4 } from "uuid";

export function requestIdMiddleware(
  req: Request,
  res: Response,
  next: NextFunction,
): void {
  // Use provided request ID or generate new one
  const requestId = (req.headers["x-request-id"] as string) || uuidv4();

  req.requestId = requestId;
  res.setHeader("X-Request-ID", requestId);

  next();
}
