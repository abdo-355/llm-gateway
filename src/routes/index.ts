import { Router } from "express";
import { completionsHandler } from "./completions";
import { healthHandler } from "./health";
import { metricsHandler } from "./metrics";

export const router = Router();

// Mount routes (health is mounted separately in app.ts to bypass auth)
router.post("/chat/completions", completionsHandler);
router.get("/metrics", metricsHandler);

export { completionsHandler, healthHandler, metricsHandler };
