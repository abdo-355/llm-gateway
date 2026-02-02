import { createApp } from "./app";
import { closeRedis } from "./lib/redis";
import { logger } from "./utils/logger";

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
