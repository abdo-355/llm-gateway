import { Request, Response, NextFunction } from 'express';
import { RequestWithId } from './requestId';

export function authMiddleware(
  internalApiKey: string | undefined
): (req: Request, res: Response, next: NextFunction) => void {
  return (req: Request, res: Response, next: NextFunction): void => {
    if (!internalApiKey) {
      // No auth required
      next();
      return;
    }

    const authHeader = req.headers.authorization;
    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      res.status(401).json({
        error: {
          type: 'authentication_error',
          code: 'missing_auth',
          message: 'Missing or invalid Authorization header',
          request_id: (req as RequestWithId).requestId,
        },
      });
      return;
    }

    const token = authHeader.slice(7);
    if (token !== internalApiKey) {
      res.status(401).json({
        error: {
          type: 'authentication_error',
          code: 'invalid_token',
          message: 'Invalid API key',
          request_id: (req as RequestWithId).requestId,
        },
      });
      return;
    }

    next();
  };
}
