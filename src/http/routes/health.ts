import { Router, Request, Response } from 'express';
import { QuotaManager } from '../../core/quota/manager';
import { HealthMonitor } from '../../core/health/monitor';
import { CircuitBreakerRegistry, CircuitState } from '../../core/health/breaker';
import { LoadedConfig } from '../../config/loader';

export function createHealthRouter(
  config: LoadedConfig,
  quotaManager: QuotaManager,
  healthMonitor: HealthMonitor,
  circuitBreakers: CircuitBreakerRegistry
): Router {
  const router = Router();

  router.get('/health', (req: Request, res: Response) => {
    const providerStatus = config.providers.map(provider => {
      const circuitState = circuitBreakers.getState(provider.id);
      const quota = quotaManager.checkQuota(provider.id, provider.limits);
      const healthScore = healthMonitor.getHealthScore(provider.id);
      const avgLatency = healthMonitor.getLatency(provider.id);

      return {
        id: provider.id,
        circuit_state: circuitState,
        quota_available: quota.ok,
        quota_headroom: quota.headroomScore,
        health_score: healthScore,
        avg_latency_ms: avgLatency,
      };
    });

    const allHealthy = providerStatus.every(p => 
      p.circuit_state !== CircuitState.OPEN && p.quota_available
    );

    res.json({
      status: allHealthy ? 'healthy' : 'degraded',
      providers: providerStatus,
    });
  });

  router.get('/health/providers', (req: Request, res: Response) => {
    res.json({
      providers: config.providers.map(p => ({
        id: p.id,
        base_url: p.base_url,
        capabilities: p.capabilities,
        models: p.models,
      })),
    });
  });

  return router;
}
