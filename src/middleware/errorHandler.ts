import { Request, Response, NextFunction } from 'express';
import { GatewayError } from '../types';
import { logger } from '../utils/logger';

export function errorHandler(
  err: Error,
  req: Request,
  res: Response,
  _next: NextFunction
): void {
  logger.error({
    event: 'unhandled_error',
    error: err.message,
    stack: err.stack,
    request_id: req.requestId,
  });

  const error: GatewayError = {
    type: 'gateway_error',
    code: 'INTERNAL_ERROR',
    message: process.env.NODE_ENV === 'production' 
      ? 'An internal error occurred' 
      : err.message,
    request_id: req.requestId,
  };

  res.status(500).json({ error });
}
