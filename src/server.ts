import { createApp } from "./app";
import { closeRedis } from "./lib/redis";
import { logger } from "./utils/logger";
import { Registry, Counter, Histogram } from "prom-client";

// Create Prometheus registry
export const register = new Registry();

// Define metrics
export const gatewayRequestsTotal = new Counter({
  name: "gateway_requests_total",
  help: "Total number of requests",
  labelNames: ["status"],
  registers: [register],
});

export const gatewayLatencyHistogram = new Histogram({
  name: "gateway_latency_ms",
  help: "Request latency in milliseconds",
  labelNames: ["provider", "model"],
  buckets: [50, 100, 250, 500, 1000, 2500, 5000, 10000],
  registers: [register],
});

async function main() {
  const port = process.env.PORT || "8080";
  const app = createApp();

  logger.info({
    event: "startup",
    version: "1.0.0",
    port,
  });

  const server = app.listen(parseInt(port, 10), () => {
    logger.info({
      event: "server_started",
      port,
      env: process.env.NODE_ENV || "development",
    });
  });

  // Graceful shutdown
  const shutdown = async (signal: string) => {
    logger.info({ event: "shutdown", signal });

    server.close(async () => {
      logger.info({ event: "http_server_closed" });

      // Close Redis connection
      await closeRedis();
      logger.info({ event: "redis_closed" });

      process.exit(0);
    });
  };

  process.on("SIGTERM", () => shutdown("SIGTERM"));
  process.on("SIGINT", () => shutdown("SIGINT"));
}

main().catch((error) => {
  console.error("Failed to start server:", error);
  process.exit(1);
});
