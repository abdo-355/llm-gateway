import { Request, Response } from "express";
import { loadConfig } from "../config";
import { healthService } from "../services/health";

import { logger } from "../utils/logger";

export async function healthHandler(
  _req: Request,
  res: Response,
): Promise<void> {
  try {
    const config = loadConfig();
    const healthMetrics = await healthService.getAllHealthMetrics();

    const providers = config.providers.map((provider) => {
      const metrics = healthMetrics.find((m) => m.providerId === provider.id);
      const quota = provider.limits;

      return {
        id: provider.id,
        circuit_state: metrics?.circuitState || "CLOSED",
        quota: quota
          ? {
              rpm: quota.rpm,
              daily_requests: quota.dailyRequests,
            }
          : null,
        health_score: metrics?.healthScore ?? 1.0,
        avg_latency_ms: metrics?.averageLatency,
      };
    });

    const allHealthy = providers.every((p) => p.circuit_state !== "OPEN");

    res.json({
      status: allHealthy ? "healthy" : "degraded",
      timestamp: new Date().toISOString(),
      providers,
    });
  } catch (error) {
    logger.error({
      event: "health_check_failed",
      error: error instanceof Error ? error.message : String(error),
    });

    res.status(500).json({
      status: "unhealthy",
      error: "Failed to check health",
    });
  }
}
