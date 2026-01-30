export enum CircuitState {
  CLOSED = 'CLOSED',
  OPEN = 'OPEN',
  HALF_OPEN = 'HALF_OPEN',
}

export interface CircuitBreakerConfig {
  failureThreshold: number;
  recoveryTimeoutMs: number;
  halfOpenMaxCalls: number;
}

export class CircuitBreaker {
  private state: CircuitState;
  private failureCount: number;
  private successCount: number;
  private lastFailureTime: Date | undefined;
  private config: CircuitBreakerConfig;

  constructor(config: Partial<CircuitBreakerConfig> = {}) {
    this.state = CircuitState.CLOSED;
    this.failureCount = 0;
    this.successCount = 0;
    this.config = {
      failureThreshold: config.failureThreshold ?? 5,
      recoveryTimeoutMs: config.recoveryTimeoutMs ?? 30000,
      halfOpenMaxCalls: config.halfOpenMaxCalls ?? 1,
    };
  }

  canExecute(): boolean {
    switch (this.state) {
      case CircuitState.CLOSED:
        return true;
      
      case CircuitState.OPEN:
        // Check if recovery timeout has elapsed
        if (this.lastFailureTime) {
          const elapsed = Date.now() - this.lastFailureTime.getTime();
          if (elapsed >= this.config.recoveryTimeoutMs) {
            this.state = CircuitState.HALF_OPEN;
            this.failureCount = 0;
            this.successCount = 0;
            return true;
          }
        }
        return false;
      
      case CircuitState.HALF_OPEN:
        return this.failureCount + this.successCount < this.config.halfOpenMaxCalls;
    }
  }

  recordSuccess(): void {
    switch (this.state) {
      case CircuitState.HALF_OPEN:
        this.successCount++;
        if (this.successCount >= this.config.halfOpenMaxCalls) {
          this.state = CircuitState.CLOSED;
          this.failureCount = 0;
          this.successCount = 0;
        }
        break;
      
      case CircuitState.CLOSED:
        this.failureCount = 0;
        break;
    }
  }

  recordFailure(): void {
    this.failureCount++;
    this.lastFailureTime = new Date();

    switch (this.state) {
      case CircuitState.HALF_OPEN:
        this.state = CircuitState.OPEN;
        break;
      
      case CircuitState.CLOSED:
        if (this.failureCount >= this.config.failureThreshold) {
          this.state = CircuitState.OPEN;
        }
        break;
    }
  }

  getState(): CircuitState {
    return this.state;
  }

  isOpen(): boolean {
    return this.state === CircuitState.OPEN;
  }
}

export class CircuitBreakerRegistry {
  private breakers: Map<string, CircuitBreaker>;
  private config: CircuitBreakerConfig;

  constructor(config: Partial<CircuitBreakerConfig> = {}) {
    this.breakers = new Map();
    this.config = {
      failureThreshold: config.failureThreshold ?? 5,
      recoveryTimeoutMs: config.recoveryTimeoutMs ?? 30000,
      halfOpenMaxCalls: config.halfOpenMaxCalls ?? 1,
    };
  }

  private getBreaker(providerId: string): CircuitBreaker {
    let breaker = this.breakers.get(providerId);
    if (!breaker) {
      breaker = new CircuitBreaker(this.config);
      this.breakers.set(providerId, breaker);
    }
    return breaker;
  }

  canExecute(providerId: string): boolean {
    return this.getBreaker(providerId).canExecute();
  }

  recordSuccess(providerId: string): void {
    this.getBreaker(providerId).recordSuccess();
  }

  recordFailure(providerId: string): void {
    this.getBreaker(providerId).recordFailure();
  }

  getState(providerId: string): CircuitState {
    return this.getBreaker(providerId).getState();
  }

  isOpen(providerId: string): boolean {
    return this.getBreaker(providerId).isOpen();
  }

  getAllStates(): Map<string, CircuitState> {
    const result = new Map<string, CircuitState>();
    for (const [providerId, breaker] of this.breakers) {
      result.set(providerId, breaker.getState());
    }
    return result;
  }
}
