import { Router } from 'express';
import { completionsHandler } from './completions';
import { healthHandler } from './health';
import { metricsHandler } from './metrics';

export const router = Router();

// Mount routes
router.post('/chat/completions', completionsHandler);
router.get('/health', healthHandler);
router.get('/metrics', metricsHandler);

export { completionsHandler, healthHandler, metricsHandler };
