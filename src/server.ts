import { createApp } from "./app";
import { closeRedis } from "./lib/redis";
import { logger } from "./utils/logger";
import { validateAndLoadEnv } from "./config/env";

async function main() {
  // Validate environment variables first - server won't start if any are missing
  let env;
  try {
    env = validateAndLoadEnv();
  } catch (error) {
    console.error(
      error instanceof Error ? error.message : "Environment validation failed",
    );
    process.exit(1);
  }

  const port = env.PORT;
  const app = createApp();

  logger.info({
    event: "startup",
    version: "1.0.0",
    port,
    env: env.NODE_ENV,
  });

  const server = app.listen(port, () => {
    logger.info({
      event: "server_started",
      port,
      env: env.NODE_ENV,
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
