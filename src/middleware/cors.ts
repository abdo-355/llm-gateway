import { Request, Response, NextFunction } from 'express';

export function corsMiddleware(
  req: Request,
  res: Response,
  next: NextFunction
): void {
  // Read CORS_ORIGINS fresh each time as it can change (not cached like other env vars)
  const originsEnv = process.env.CORS_ORIGINS || '';
  const allowedOrigins = originsEnv.split(',').map(o => o.trim()).filter(o => o);
  
  const origin = req.headers.origin;
  
  // Allow requests with no origin (e.g., curl, mobile apps)
  if (!origin) {
    next();
    return;
  }
  
  // Only set CORS headers if origin is allowed
  if (allowedOrigins.length === 0 || allowedOrigins.includes(origin)) {
    res.setHeader('Access-Control-Allow-Origin', origin);
    res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
    res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Request-ID');
    res.setHeader('Access-Control-Max-Age', '86400');
    
    if (req.method === 'OPTIONS') {
      res.status(204).end();
      return;
    }
  }
  
  next();
}
