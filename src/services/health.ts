import { getRedisClient } from "../lib/redis";
import { CircuitBreakerError } from "../errors";

const CIRCUIT_PREFIX = "circuit:";
const HEALTH_PREFIX = "health:";

export type CircuitState = "CLOSED" | "OPEN" | "HALF_OPEN";

export interface HealthMetrics {
  providerId: string;
  circuitState: CircuitState;
  failureCount: number;
  successCount: number;
  lastFailureTime: number | null;
  averageLatency: number | null;
  healthScore: number;
}

export class HealthService {
  private redis = getRedisClient();
  private failureThreshold = 5;
  private recoveryTimeoutMs = 30000;

  async getCircuitState(providerId: string): Promise<CircuitState> {
    const stateKey = `${CIRCUIT_PREFIX}${providerId}:state`;
    const state = await this.redis.get(stateKey);
    return (state as CircuitState) || "CLOSED";
  }

  async canExecute(providerId: string): Promise<boolean> {
    const state = await this.getCircuitState(providerId);

    if (state === "CLOSED") {
      return true;
    }

    if (state === "OPEN") {
      // Check if recovery timeout has passed
      const lastFailureKey = `${CIRCUIT_PREFIX}${providerId}:last_failure`;
      const lastFailure = await this.redis.get(lastFailureKey);

      if (lastFailure) {
        const elapsed = Date.now() - parseInt(lastFailure, 10);
        if (elapsed >= this.recoveryTimeoutMs) {
          // Transition to HALF_OPEN
          await this.setCircuitState(providerId, "HALF_OPEN");
          await this.redis.set(`${CIRCUIT_PREFIX}${providerId}:failures`, "0");
          await this.redis.set(`${CIRCUIT_PREFIX}${providerId}:successes`, "0");
          return true;
        }
      }
      return false;
    }

    if (state === "HALF_OPEN") {
      // Allow limited requests in half-open state
      const successes = parseInt(
        (await this.redis.get(`${CIRCUIT_PREFIX}${providerId}:successes`)) ||
          "0",
        10,
      );
      const failures = parseInt(
        (await this.redis.get(`${CIRCUIT_PREFIX}${providerId}:failures`)) ||
          "0",
        10,
      );
      return successes + failures < 1; // Allow 1 probe request
    }

    return false;
  }

  /**
   * Check circuit breaker and throw CircuitBreakerError if circuit is open
   * Use this when you want to fail fast with a clear error
   */
  async checkCircuitBreakerOrThrow(providerId: string): Promise<void> {
    const state = await this.getCircuitState(providerId);

    if (state === "OPEN") {
      throw new CircuitBreakerError(
        `Circuit breaker is OPEN for provider ${providerId}`,
        providerId,
        "OPEN",
      );
    }

    if (state === "HALF_OPEN") {
      // Check if probe limit reached
      const successes = parseInt(
        (await this.redis.get(`${CIRCUIT_PREFIX}${providerId}:successes`)) ||
          "0",
        10,
      );
      const failures = parseInt(
        (await this.redis.get(`${CIRCUIT_PREFIX}${providerId}:failures`)) ||
          "0",
        10,
      );

      if (successes + failures >= 1) {
        throw new CircuitBreakerError(
          `Circuit breaker is HALF_OPEN for provider ${providerId}, probe limit reached`,
          providerId,
          "HALF_OPEN",
        );
      }
    }
  }

  async recordSuccess(providerId: string, latencyMs: number): Promise<void> {
    const state = await this.getCircuitState(providerId);
    const successesKey = `${CIRCUIT_PREFIX}${providerId}:successes`;
    const failuresKey = `${CIRCUIT_PREFIX}${providerId}:failures`;

    if (state === "HALF_OPEN") {
      const successes = await this.redis.incr(successesKey);
      if (successes >= 1) {
        // Close the circuit
        await this.setCircuitState(providerId, "CLOSED");
        await this.redis.set(failuresKey, "0");
        await this.redis.set(successesKey, "0");
      }
    } else if (state === "CLOSED") {
      // Reset failure count on success
      await this.redis.set(failuresKey, "0");
    }

    // Record latency with 1 hour TTL
    const latencyKey = `${HEALTH_PREFIX}${providerId}:latency`;
    await this.redis.setex(latencyKey, 3600, latencyMs.toString());

    // Update health score
    await this.updateHealthScore(providerId);
  }

  async recordFailure(providerId: string): Promise<void> {
    const state = await this.getCircuitState(providerId);
    const failuresKey = `${CIRCUIT_PREFIX}${providerId}:failures`;
    const lastFailureKey = `${CIRCUIT_PREFIX}${providerId}:last_failure`;

    const failures = await this.redis.incr(failuresKey);
    await this.redis.set(lastFailureKey, Date.now().toString());

    if (state === "HALF_OPEN") {
      // Re-open the circuit
      await this.setCircuitState(providerId, "OPEN");
    } else if (state === "CLOSED" && failures >= this.failureThreshold) {
      // Open the circuit
      await this.setCircuitState(providerId, "OPEN");
    }

    // Update health score
    await this.updateHealthScore(providerId);
  }

  async getHealthMetrics(providerId: string): Promise<HealthMetrics> {
    const [
      circuitState,
      failureCount,
      successCount,
      lastFailureTime,
      averageLatency,
      healthScore,
    ] = await Promise.all([
      this.getCircuitState(providerId),
      this.redis
        .get(`${CIRCUIT_PREFIX}${providerId}:failures`)
        .then((v) => parseInt(v || "0", 10)),
      this.redis
        .get(`${CIRCUIT_PREFIX}${providerId}:successes`)
        .then((v) => parseInt(v || "0", 10)),
      this.redis
        .get(`${CIRCUIT_PREFIX}${providerId}:last_failure`)
        .then((v) => (v ? parseInt(v, 10) : null)),
      this.redis
        .get(`${HEALTH_PREFIX}${providerId}:latency`)
        .then((v) => (v ? parseInt(v, 10) : null)),
      this.redis
        .get(`${HEALTH_PREFIX}${providerId}:score`)
        .then((v) => (v ? parseFloat(v) : 1.0)),
    ]);

    return {
      providerId,
      circuitState,
      failureCount,
      successCount,
      lastFailureTime,
      averageLatency,
      healthScore,
    };
  }

  async getAllHealthMetrics(): Promise<HealthMetrics[]> {
    // Get all provider IDs from circuit keys
    const keys = await this.redis.keys(`${CIRCUIT_PREFIX}*:state`);
    const providerIds = keys.map((k) =>
      k.replace(`${CIRCUIT_PREFIX}`, "").replace(":state", ""),
    );

    const metrics = await Promise.all(
      providerIds.map((id) => this.getHealthMetrics(id)),
    );

    return metrics;
  }

  private async setCircuitState(
    providerId: string,
    state: CircuitState,
  ): Promise<void> {
    const stateKey = `${CIRCUIT_PREFIX}${providerId}:state`;
    await this.redis.set(stateKey, state);
  }

  private async updateHealthScore(providerId: string): Promise<void> {
    const failures = parseInt(
      (await this.redis.get(`${CIRCUIT_PREFIX}${providerId}:failures`)) || "0",
      10,
    );
    const state = await this.getCircuitState(providerId);

    // Simple health score calculation
    let score = 1.0;

    if (state === "OPEN") {
      score = 0;
    } else if (state === "HALF_OPEN") {
      score = 0.5;
    } else {
      // Penalize consecutive failures
      if (failures > 3) {
        score = 0.5;
      } else if (failures > 0) {
        score = 1 - failures * 0.1;
      }
    }

    const scoreKey = `${HEALTH_PREFIX}${providerId}:score`;
    await this.redis.setex(scoreKey, 3600, score.toString());
  }
}

export const healthService = new HealthService();
