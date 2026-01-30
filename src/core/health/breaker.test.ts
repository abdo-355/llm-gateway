import { CircuitBreaker, CircuitBreakerRegistry, CircuitState } from '../core/health/breaker';

describe('CircuitBreaker', () => {
  it('should start in CLOSED state', () => {
    const breaker = new CircuitBreaker();
    expect(breaker.getState()).toBe(CircuitState.CLOSED);
    expect(breaker.canExecute()).toBe(true);
  });

  it('should open after threshold failures', () => {
    const breaker = new CircuitBreaker({ failureThreshold: 3 });
    
    breaker.recordFailure();
    expect(breaker.getState()).toBe(CircuitState.CLOSED);
    
    breaker.recordFailure();
    expect(breaker.getState()).toBe(CircuitState.CLOSED);
    
    breaker.recordFailure();
    expect(breaker.getState()).toBe(CircuitState.OPEN);
    expect(breaker.canExecute()).toBe(false);
  });

  it('should reset failure count on success', () => {
    const breaker = new CircuitBreaker({ failureThreshold: 3 });
    
    breaker.recordFailure();
    breaker.recordFailure();
    breaker.recordSuccess();
    
    expect(breaker.getState()).toBe(CircuitState.CLOSED);
    
    // Need 3 more failures to open
    breaker.recordFailure();
    breaker.recordFailure();
    breaker.recordFailure();
    
    expect(breaker.getState()).toBe(CircuitState.OPEN);
  });

  it('should transition to HALF_OPEN after recovery timeout', () => {
    jest.useFakeTimers();
    
    const breaker = new CircuitBreaker({ 
      failureThreshold: 3,
      recoveryTimeoutMs: 30000 
    });
    
    // Open the circuit
    breaker.recordFailure();
    breaker.recordFailure();
    breaker.recordFailure();
    
    expect(breaker.getState()).toBe(CircuitState.OPEN);
    expect(breaker.canExecute()).toBe(false);
    
    // Advance time past recovery timeout
    jest.advanceTimersByTime(30001);
    
    expect(breaker.canExecute()).toBe(true);
    expect(breaker.getState()).toBe(CircuitState.HALF_OPEN);
    
    jest.useRealTimers();
  });

  it('should close on success in HALF_OPEN', () => {
    jest.useFakeTimers();
    
    const breaker = new CircuitBreaker({ 
      failureThreshold: 3,
      recoveryTimeoutMs: 30000,
      halfOpenMaxCalls: 1 
    });
    
    // Open and recover
    breaker.recordFailure();
    breaker.recordFailure();
    breaker.recordFailure();
    jest.advanceTimersByTime(30001);
    
    expect(breaker.getState()).toBe(CircuitState.HALF_OPEN);
    
    // Success should close it
    breaker.recordSuccess();
    expect(breaker.getState()).toBe(CircuitState.CLOSED);
    
    jest.useRealTimers();
  });

  it('should re-open on failure in HALF_OPEN', () => {
    jest.useFakeTimers();
    
    const breaker = new CircuitBreaker({ 
      failureThreshold: 3,
      recoveryTimeoutMs: 30000 
    });
    
    // Open and recover
    breaker.recordFailure();
    breaker.recordFailure();
    breaker.recordFailure();
    jest.advanceTimersByTime(30001);
    
    expect(breaker.getState()).toBe(CircuitState.HALF_OPEN);
    
    // Failure should re-open
    breaker.recordFailure();
    expect(breaker.getState()).toBe(CircuitState.OPEN);
    
    jest.useRealTimers();
  });
});

describe('CircuitBreakerRegistry', () => {
  it('should manage multiple breakers', () => {
    const registry = new CircuitBreakerRegistry({ failureThreshold: 3 });
    
    expect(registry.canExecute('provider-a')).toBe(true);
    expect(registry.canExecute('provider-b')).toBe(true);
    
    // Trip provider-a
    registry.recordFailure('provider-a');
    registry.recordFailure('provider-a');
    registry.recordFailure('provider-a');
    
    expect(registry.isOpen('provider-a')).toBe(true);
    expect(registry.isOpen('provider-b')).toBe(false);
    expect(registry.canExecute('provider-a')).toBe(false);
    expect(registry.canExecute('provider-b')).toBe(true);
  });

  it('should return all states', () => {
    const registry = new CircuitBreakerRegistry({ failureThreshold: 2 });
    
    registry.recordFailure('provider-a');
    registry.recordFailure('provider-a');
    
    const states = registry.getAllStates();
    
    expect(states.get('provider-a')).toBe(CircuitState.OPEN);
    expect(states.has('provider-b')).toBe(false);
  });
});
