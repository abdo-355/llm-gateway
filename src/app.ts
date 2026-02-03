import express, { Application } from "express";
import { middleware } from "./middleware";
import { router, healthHandler } from "./routes";

export function createApp(): Application {
  const app = express();

  // Security middleware
  app.use(middleware.helmet);
  app.use(middleware.cors);

  // Request handling
  app.use(middleware.requestId);
  app.use(express.json({ limit: "10mb" }));

  // Health endpoint - no auth required
  app.get("/health", healthHandler);

  // Mandatory auth (no bypass)
  app.use(middleware.auth);

  // Rate limiting (async middleware needs wrapper)
  app.use((req, res, next) => {
    middleware.rateLimit(req, res, next).catch(next);
  });

  // Routes
  app.use("/v1", router);
  app.use("/", router);

  // Error handling
  app.use(middleware.errorHandler);

  return app;
}
