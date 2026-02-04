import { Request, Response, NextFunction } from "express";

export function helmetMiddleware(
  _req: Request,
  res: Response,
  next: NextFunction,
): void {
  // Security headers
  res.setHeader("X-Content-Type-Options", "nosniff");
  res.setHeader("X-Frame-Options", "DENY");
  res.setHeader("X-XSS-Protection", "1; mode=block");
  res.setHeader(
    "Strict-Transport-Security",
    "max-age=31536000; includeSubDomains",
  );
  res.setHeader("Content-Security-Policy", "default-src 'self'");
  res.removeHeader("X-Powered-By");

  next();
}
