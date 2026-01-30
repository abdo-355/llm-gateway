import express, { Application, json } from 'express';
import { loadConfig } from './config/loader';
import { QuotaManager } from './core/quota/manager';
import { HealthMonitor } from './core/health/monitor';
import { CircuitBreakerRegistry } from './core/health/breaker';
import { OpenAICompatibleClient } from './providers/openaiCompatible/client';
import { createChatCompletionsRouter } from './http/routes/chatCompletions';
import { createHealthRouter } from './http/routes/health';
import { requestIdMiddleware } from './http/middleware/requestId';
import { authMiddleware } from './http/middleware/auth';
import { Logger } from './logging';
import { Registry, Counter, Histogram, Gauge } from 'prom-client';

async function main() {
  const logger = new Logger();
  
  logger.info({ event: 'startup', version: '1.0.0' });

  // Load configuration
  const configDir = process.env.CONFIG_DIR || './config';
  let config;
  try {
    config = loadConfig(configDir);
    logger.info({ 
      event: 'config_loaded', 
      providers: config.providers.map(p => p.id),
      certifications: config.certifications.length,
    });
  } catch (error) {
    logger.error({ 
      event: 'config_load_failed', 
      error: error instanceof Error ? error.message : String(error),
    });
    process.exit(1);
  }

  // Initialize core components
  const quotaManager = new QuotaManager();
  const healthMonitor = new HealthMonitor();
  const circuitBreakers = new CircuitBreakerRegistry({
    failureThreshold: 5,
    recoveryTimeoutMs: 30000,
  });
  const providerClient = new OpenAICompatibleClient(logger);

  // Setup periodic quota window updates
  setInterval(() => {
    quotaManager.tick();
  }, 1000);

  // Daily quota reset
  setInterval(() => {
    quotaManager.resetDaily();
    logger.info({ event: 'quota_reset_daily' });
  }, 24 * 60 * 60 * 1000);

  // Setup Prometheus metrics
  const register = new Registry();
  
  const gatewayRequestsTotal = new Counter({
    name: 'gateway_requests_total',
    help: 'Total number of requests',
    labelNames: ['status'],
    registers: [register],
  });

  const gatewayLatencyHistogram = new Histogram({
    name: 'gateway_latency_ms',
    help: 'Request latency in milliseconds',
    labelNames: ['provider', 'model'],
    buckets: [50, 100, 250, 500, 1000, 2500, 5000, 10000],
    registers: [register],
  });

  const providerCircuitState = new Gauge({
    name: 'provider_circuit_state',
    help: 'Circuit breaker state (0=closed, 1=half-open, 2=open)',
    labelNames: ['provider'],
    registers: [register],
  });

  const quotaRemaining = new Gauge({
    name: 'quota_remaining',
    help: 'Remaining quota percentage',
    labelNames: ['provider'],
    registers: [register],
  });

  // Update metrics periodically
  setInterval(() => {
    for (const provider of config.providers) {
      const state = circuitBreakers.getState(provider.id);
      const stateValue = state === 'CLOSED' ? 0 : state === 'HALF_OPEN' ? 1 : 2;
      providerCircuitState.set({ provider: provider.id }, stateValue);

      const quota = quotaManager.checkQuota(provider.id, provider.limits);
      quotaRemaining.set({ provider: provider.id }, quota.headroomScore * 100);
    }
  }, 5000);

  // Create Express app
  const app: Application = express();
  
  // Middleware
  app.use(json({ limit: '10mb' }));
  app.use(requestIdMiddleware);
  app.use(authMiddleware(process.env.INTERNAL_API_KEY));

  // Routes
  app.use('/v1', createChatCompletionsRouter(
    config,
    providerClient,
    quotaManager,
    healthMonitor,
    circuitBreakers,
    logger
  ));

  app.use('/', createHealthRouter(
    config,
    quotaManager,
    healthMonitor,
    circuitBreakers
  ));

  // Metrics endpoint
  app.get('/metrics', async (req, res) => {
    res.set('Content-Type', register.contentType);
    res.end(await register.metrics());
  });

  // Error handling
  app.use((err: Error, req: express.Request, res: express.Response, _next: express.NextFunction) => {
    logger.error({
      event: 'unhandled_error',
      error: err.message,
      stack: err.stack,
    });
    res.status(500).json({
      error: {
        type: 'internal_error',
        code: 'internal_error',
        message: 'An unexpected error occurred',
      },
    });
  });

  // Start server
  const port = process.env.PORT || 3000;
  app.listen(port, () => {
    logger.info({ 
      event: 'server_started', 
      port,
      internal_api_key_set: !!process.env.INTERNAL_API_KEY,
    });
  });

  // Graceful shutdown
  process.on('SIGTERM', () => {
    logger.info({ event: 'shutdown', signal: 'SIGTERM' });
    process.exit(0);
  });

  process.on('SIGINT', () => {
    logger.info({ event: 'shutdown', signal: 'SIGINT' });
    process.exit(0);
  });
}

main().catch((error) => {
  console.error('Failed to start server:', error);
  process.exit(1);
});
