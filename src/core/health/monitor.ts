export class EWMA {
  private alpha: number;
  private value: number | undefined;

  constructor(halfLifeMs: number) {
    // Convert half-life to alpha: alpha = 1 - e^(-ln(2) * dt / halfLife)
    // For discrete updates, we use a simplified version
    this.alpha = 0.1; // Can be tuned based on update frequency
  }

  update(newValue: number): void {
    if (this.value === undefined) {
      this.value = newValue;
    } else {
      this.value = this.alpha * newValue + (1 - this.alpha) * this.value;
    }
  }

  getValue(): number | undefined {
    return this.value;
  }
}

export interface HealthMetrics {
  successCount: number;
  errorCount: number;
  timeoutCount: number;
  lastErrorTime: Date | undefined;
  consecutiveErrors: number;
  averageLatency: number | undefined;
}

export class HealthMonitor {
  private metrics: Map<string, HealthMetrics>;
  private latencyEWMA: Map<string, EWMA>;

  constructor() {
    this.metrics = new Map();
    this.latencyEWMA = new Map();
  }

  private getMetrics(providerId: string): HealthMetrics {
    let metrics = this.metrics.get(providerId);
    if (!metrics) {
      metrics = {
        successCount: 0,
        errorCount: 0,
        timeoutCount: 0,
        lastErrorTime: undefined,
        consecutiveErrors: 0,
        averageLatency: undefined,
      };
      this.metrics.set(providerId, metrics);
    }
    return metrics;
  }

  recordSuccess(providerId: string, latencyMs: number): void {
    const metrics = this.getMetrics(providerId);
    metrics.successCount++;
    metrics.consecutiveErrors = 0;

    let ewma = this.latencyEWMA.get(providerId);
    if (!ewma) {
      ewma = new EWMA(60000); // 1 minute half-life
      this.latencyEWMA.set(providerId, ewma);
    }
    ewma.update(latencyMs);
    metrics.averageLatency = ewma.getValue();
  }

  recordError(providerId: string, isTimeout: boolean = false): void {
    const metrics = this.getMetrics(providerId);
    metrics.errorCount++;
    metrics.consecutiveErrors++;
    metrics.lastErrorTime = new Date();

    if (isTimeout) {
      metrics.timeoutCount++;
    }
  }

  getHealthScore(providerId: string): number {
    const metrics = this.getMetrics(providerId);
    
    // Health score from 0 to 1
    // Based on error rate and consecutive errors
    const total = metrics.successCount + metrics.errorCount;
    if (total === 0) {
      return 1.0; // Assume healthy if no data
    }

    const errorRate = metrics.errorCount / total;
    let score = 1 - errorRate;

    // Penalize consecutive errors heavily
    if (metrics.consecutiveErrors > 3) {
      score *= 0.5;
    }
    if (metrics.consecutiveErrors > 5) {
      score = 0;
    }

    return Math.max(0, score);
  }

  getLatency(providerId: string): number | undefined {
    const ewma = this.latencyEWMA.get(providerId);
    return ewma?.getValue();
  }

  getMetricsAll(): Map<string, number> {
    const result = new Map<string, number>();
    for (const [providerId] of this.metrics) {
      result.set(providerId, this.getHealthScore(providerId));
    }
    return result;
  }

  getLatencyAll(): Map<string, number> {
    const result = new Map<string, number>();
    for (const [providerId, ewma] of this.latencyEWMA) {
      const value = ewma.getValue();
      if (value !== undefined) {
        result.set(providerId, value);
      }
    }
    return result;
  }
}
