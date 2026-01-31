import {
  ProviderError,
  ValidationError,
  RateLimitError,
  CircuitBreakerError,
  TimeoutError,
} from './index';

describe('Error Classes', () => {
  describe('ProviderError', () => {
    it('should create ProviderError with statusCode and isRetryable', () => {
      const error = new ProviderError('Test error', 500, true);

      expect(error.message).toBe('Test error');
      expect(error.statusCode).toBe(500);
      expect(error.isRetryable).toBe(true);
      expect(error.name).toBe('ProviderError');
    });

    it('should default isRetryable to false', () => {
      const error = new ProviderError('Test error', 400);

      expect(error.isRetryable).toBe(false);
    });

    it('should be an instance of Error', () => {
      const error = new ProviderError('Test', 500);

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(ProviderError);
    });
  });

  describe('ValidationError', () => {
    it('should create ValidationError with details', () => {
      const details = [
        { path: 'model', message: 'Model is required' },
        { path: 'messages', message: 'Messages must be an array' },
      ];
      const error = new ValidationError('Validation failed', details);

      expect(error.message).toBe('Validation failed');
      expect(error.details).toEqual(details);
      expect(error.statusCode).toBe(400);
      expect(error.isRetryable).toBe(false);
      expect(error.name).toBe('ValidationError');
    });

    it('should extend ProviderError', () => {
      const error = new ValidationError('Test', []);

      expect(error).toBeInstanceOf(ProviderError);
    });
  });

  describe('RateLimitError', () => {
    it('should create RateLimitError with retryAfter and limitType', () => {
      const error = new RateLimitError(
        'Rate limit exceeded',
        60,
        'rpm'
      );

      expect(error.message).toBe('Rate limit exceeded');
      expect(error.retryAfter).toBe(60);
      expect(error.limitType).toBe('rpm');
      expect(error.statusCode).toBe(429);
      expect(error.isRetryable).toBe(true);
      expect(error.name).toBe('RateLimitError');
    });

    it('should support different limit types', () => {
      const dailyError = new RateLimitError('Daily limit', 3600, 'daily');
      const tpmError = new RateLimitError('Token limit', 30, 'tpm');

      expect(dailyError.limitType).toBe('daily');
      expect(tpmError.limitType).toBe('tpm');
    });

    it('should extend ProviderError', () => {
      const error = new RateLimitError('Test', 60, 'rpm');

      expect(error).toBeInstanceOf(ProviderError);
    });
  });

  describe('CircuitBreakerError', () => {
    it('should create CircuitBreakerError with providerId and state', () => {
      const error = new CircuitBreakerError(
        'Circuit breaker is open',
        'groq',
        'OPEN'
      );

      expect(error.message).toBe('Circuit breaker is open');
      expect(error.providerId).toBe('groq');
      expect(error.state).toBe('OPEN');
      expect(error.statusCode).toBe(503);
      expect(error.isRetryable).toBe(true);
      expect(error.name).toBe('CircuitBreakerError');
    });

    it('should support HALF_OPEN state', () => {
      const error = new CircuitBreakerError(
        'Circuit breaker is half-open',
        'openrouter',
        'HALF_OPEN'
      );

      expect(error.state).toBe('HALF_OPEN');
    });

    it('should extend ProviderError', () => {
      const error = new CircuitBreakerError('Test', 'provider', 'OPEN');

      expect(error).toBeInstanceOf(ProviderError);
    });
  });

  describe('TimeoutError', () => {
    it('should create TimeoutError with timeoutType', () => {
      const requestError = new TimeoutError('Request timeout', 'request');
      const inactivityError = new TimeoutError(
        'Inactivity timeout',
        'inactivity'
      );

      expect(requestError.timeoutType).toBe('request');
      expect(inactivityError.timeoutType).toBe('inactivity');
      expect(requestError.statusCode).toBe(504);
      expect(requestError.isRetryable).toBe(true);
    });

    it('should extend ProviderError', () => {
      const error = new TimeoutError('Test', 'request');

      expect(error).toBeInstanceOf(ProviderError);
    });
  });
});
