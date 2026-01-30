import { RollingWindow, QuotaManager } from '../core/quota/manager';

describe('RollingWindow', () => {
  it('should track values in rolling window', () => {
    const window = new RollingWindow(3);
    
    window.add(1);
    expect(window.getSum()).toBe(1);
    
    window.add(2);
    expect(window.getSum()).toBe(3);
    
    window.add(3);
    expect(window.getSum()).toBe(6);
    
    // Should roll over
    window.add(4);
    expect(window.getSum()).toBe(9); // 2 + 3 + 4
  });

  it('should calculate average', () => {
    const window = new RollingWindow(3);
    
    window.add(10);
    window.add(20);
    window.add(30);
    
    expect(window.getAverage()).toBe(20);
  });
});

describe('QuotaManager', () => {
  it('should initialize quota state', () => {
    const manager = new QuotaManager();
    const state = manager.getQuotaState('provider-a');
    
    expect(state.dailyRequests).toBe(0);
    expect(state.rollingRpm).toBe(0);
  });

  it('should check quota availability', () => {
    const manager = new QuotaManager();
    
    const result = manager.checkQuota('provider-a', {
      daily_requests: 100,
      rpm: 60,
    });
    
    expect(result.ok).toBe(true);
    expect(result.remaining).toBe(100);
    expect(result.headroomScore).toBe(1);
  });

  it('should reject when daily limit exceeded', () => {
    const manager = new QuotaManager();
    
    // Exhaust quota
    for (let i = 0; i < 100; i++) {
      manager.recordRequest('provider-a');
    }
    
    const result = manager.checkQuota('provider-a', {
      daily_requests: 100,
    });
    
    expect(result.ok).toBe(false);
    expect(result.remaining).toBe(0);
  });

  it('should reject when RPM exceeded', () => {
    const manager = new QuotaManager();
    
    // Record 60 requests (RPM limit)
    for (let i = 0; i < 60; i++) {
      manager.recordRequest('provider-a');
    }
    
    const result = manager.checkQuota('provider-a', {
      rpm: 60,
    });
    
    expect(result.ok).toBe(false);
  });

  it('should update rolling windows on tick', () => {
    const manager = new QuotaManager();
    
    // Record some requests
    manager.recordRequest('provider-a');
    manager.recordRequest('provider-a');
    
    const stateBefore = manager.getQuotaState('provider-a');
    expect(stateBefore.rollingRpm).toBe(2);
    
    // Tick shifts windows
    manager.tick();
    
    const stateAfter = manager.getQuotaState('provider-a');
    expect(stateAfter.rollingRpm).toBeLessThan(stateBefore.rollingRpm);
  });

  it('should reset daily quotas', () => {
    const manager = new QuotaManager();
    
    // Use up some quota
    manager.recordRequest('provider-a');
    manager.recordRequest('provider-a');
    
    const stateBefore = manager.getQuotaState('provider-a');
    expect(stateBefore.dailyRequests).toBe(2);
    
    // Reset
    manager.resetDaily();
    
    const stateAfter = manager.getQuotaState('provider-a');
    expect(stateAfter.dailyRequests).toBe(0);
  });

  it('should track tokens when provided', () => {
    const manager = new QuotaManager();
    
    manager.recordRequest('provider-a', 100);
    manager.recordRequest('provider-a', 200);
    
    const state = manager.getQuotaState('provider-a');
    expect(state.rollingTpm).toBe(300);
  });

  it('should calculate headroom score correctly', () => {
    const manager = new QuotaManager();
    
    // Use 25 out of 100 daily requests
    for (let i = 0; i < 25; i++) {
      manager.recordRequest('provider-a');
    }
    
    const result = manager.checkQuota('provider-a', {
      daily_requests: 100,
    });
    
    expect(result.ok).toBe(true);
    expect(result.headroomScore).toBe(0.75); // 75 remaining
  });
});
