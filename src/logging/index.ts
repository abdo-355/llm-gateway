import pino from 'pino';

export interface LogContext {
  request_id?: string;
  [key: string]: any;
}

export class Logger {
  private logger: pino.Logger;
  private context: LogContext;

  constructor(context: LogContext = {}) {
    this.context = context;
    this.logger = pino({
      level: process.env.LOG_LEVEL || 'info',
      transport: process.env.NODE_ENV !== 'production' ? {
        target: 'pino-pretty',
        options: {
          colorize: true,
          translateTime: 'HH:MM:ss Z',
          ignore: 'pid,hostname',
        },
      } : undefined,
    });
  }

  child(additionalContext: LogContext): Logger {
    return new Logger({ ...this.context, ...additionalContext });
  }

  info(obj: Record<string, any>, msg?: string): void {
    this.logger.info({ ...this.context, ...obj }, msg);
  }

  error(obj: Record<string, any>, msg?: string): void {
    this.logger.error({ ...this.context, ...obj }, msg);
  }

  warn(obj: Record<string, any>, msg?: string): void {
    this.logger.warn({ ...this.context, ...obj }, msg);
  }

  debug(obj: Record<string, any>, msg?: string): void {
    this.logger.debug({ ...this.context, ...obj }, msg);
  }
}
