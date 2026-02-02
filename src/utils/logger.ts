import pino from 'pino';

// Valid log value types
export type LogValue = string | number | boolean | null | undefined | Date | LogValue[] | { [key: string]: LogValue };

export interface LogContext {
  request_id?: string;
  [key: string]: LogValue;
}

export class Logger {
  private logger: pino.Logger;
  private context: LogContext;

  constructor(context: LogContext = {}) {
    this.context = context;
    // Use process.env directly here since logger is created early
    // before getEnv() may be called. These have defaults so it's safe.
    const isDev = process.env.NODE_ENV !== 'production';
    
    this.logger = pino({
      level: process.env.LOG_LEVEL || 'info',
      transport: isDev ? {
        target: 'pino-pretty',
        options: {
          colorize: true,
          translateTime: 'HH:MM:ss Z',
          ignore: 'pid,hostname',
        },
      } : undefined,
      base: undefined,
    });
  }

  child(additionalContext: LogContext): Logger {
    return new Logger({ ...this.context, ...additionalContext });
  }

  info(obj: LogContext, msg?: string): void {
    this.logger.info({ ...this.context, ...obj }, msg);
  }

  error(obj: LogContext, msg?: string): void {
    this.logger.error({ ...this.context, ...obj }, msg);
  }

  warn(obj: LogContext, msg?: string): void {
    this.logger.warn({ ...this.context, ...obj }, msg);
  }

  debug(obj: LogContext, msg?: string): void {
    this.logger.debug({ ...this.context, ...obj }, msg);
  }
}

// Global logger instance
export const logger = new Logger();
