import { Request, Response, NextFunction } from 'express';
import { helmetMiddleware } from './helmet';

describe('helmetMiddleware', () => {
  let req: Partial<Request>;
  let res: Partial<Response>;
  let next: NextFunction;

  beforeEach(() => {
    req = {};
    res = {
      setHeader: jest.fn(),
      removeHeader: jest.fn(),
    };
    next = jest.fn();
  });

  it('should set X-Content-Type-Options to nosniff', () => {
    helmetMiddleware(req as Request, res as Response, next);

    expect(res.setHeader).toHaveBeenCalledWith('X-Content-Type-Options', 'nosniff');
  });

  it('should set X-Frame-Options to DENY', () => {
    helmetMiddleware(req as Request, res as Response, next);

    expect(res.setHeader).toHaveBeenCalledWith('X-Frame-Options', 'DENY');
  });

  it('should set X-XSS-Protection', () => {
    helmetMiddleware(req as Request, res as Response, next);

    expect(res.setHeader).toHaveBeenCalledWith('X-XSS-Protection', '1; mode=block');
  });

  it('should set Strict-Transport-Security with max-age', () => {
    helmetMiddleware(req as Request, res as Response, next);

    expect(res.setHeader).toHaveBeenCalledWith(
      'Strict-Transport-Security',
      'max-age=31536000; includeSubDomains'
    );
  });

  it('should set Content-Security-Policy', () => {
    helmetMiddleware(req as Request, res as Response, next);

    expect(res.setHeader).toHaveBeenCalledWith(
      'Content-Security-Policy',
      "default-src 'self'"
    );
  });

  it('should remove X-Powered-By header', () => {
    helmetMiddleware(req as Request, res as Response, next);

    expect(res.removeHeader).toHaveBeenCalledWith('X-Powered-By');
  });

  it('should call next() to continue to next middleware', () => {
    helmetMiddleware(req as Request, res as Response, next);

    expect(next).toHaveBeenCalledTimes(1);
    expect(next).toHaveBeenCalledWith();
  });

  it('should set all security headers in one call', () => {
    helmetMiddleware(req as Request, res as Response, next);

    expect(res.setHeader).toHaveBeenCalledTimes(5);
    expect(res.removeHeader).toHaveBeenCalledTimes(1);
  });

  it('should set correct max-age value (1 year in seconds)', () => {
    helmetMiddleware(req as Request, res as Response, next);

    const hstsCall = (res.setHeader as jest.Mock).mock.calls.find(
      call => call[0] === 'Strict-Transport-Security'
    );
    expect(hstsCall[1]).toContain('max-age=31536000');
  });

  it('should include includeSubDomains in HSTS header', () => {
    helmetMiddleware(req as Request, res as Response, next);

    const hstsCall = (res.setHeader as jest.Mock).mock.calls.find(
      call => call[0] === 'Strict-Transport-Security'
    );
    expect(hstsCall[1]).toContain('includeSubDomains');
  });
});
